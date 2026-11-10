<script lang="ts">
  import type { NodeRect } from '$/util/marqueeSelection';
  import type { NodeOffset } from '$/util/nodePositions.svelte';
  import type { State, ValidatedState } from '$/types';
  import { updateDiagram } from '$/util/api';
  import { recordRenderTime, shouldRefreshView } from '$/util/autoSync';
  import { findDefinitionLine, parseNodeId, replaceNodeLabel } from '$/util/diagramNodes';
  import { currentDiagram } from '$/util/currentDiagram.svelte';
  import { isMarqueeClick, namesInRect, normalizeRect } from '$/util/marqueeSelection';
  import { notify } from '$/util/notify';
  import {
    nodePositions,
    resetNodePositions,
    setNodePosition,
    setZoom
  } from '$/util/nodePositions.svelte';
  import { PanZoomState } from '$/util/panZoom';
  import { renderAndPlaceDiagram } from '$/util/renderView';
  import { updateCode, updateCodeStore, validatedState } from '$/util/state.svelte';
  import { saveStatistics } from '$/util/stats';
  import { clearSelectedNode, selectedNode, setSelectedNode } from '$/util/selectedNode.svelte';
  import {
    clearSelectedNodes,
    selectedNodes,
    setSelectedNodes,
    toggleSelectedNode
  } from '$/util/selectedNodes.svelte';
  import Marquee from '$/components/Marquee.svelte';
  import { TID } from '$/constants';
  import FontAwesome, { mayContainFontAwesome } from '$lib/components/FontAwesome.svelte';
  import uniqueID from 'lodash-es/uniqueId';
  import type { MermaidConfig } from 'mermaid';
  import { mode } from 'mode-watcher';
  import { onMount } from 'svelte';

  // `editable` gates the node interaction layer (select/drag/label-edit).
  // The read-only /view page mounts this component too and must never write
  // code, positions, or selection state.
  let {
    editable = false,
    panZoomState = new PanZoomState(),
    shouldShowGrid = true
  }: { editable?: boolean; panZoomState?: PanZoomState; shouldShowGrid?: boolean } = $props();
  let code = '';
  let config = '';
  let container: HTMLDivElement | undefined = $state();
  let rough: boolean;
  let view: HTMLDivElement | undefined = $state();
  let error = $state(false);
  let panZoom = true;
  let manualUpdate = true;
  let waitForFontAwesomeToLoad: FontAwesome['waitForFontAwesomeToLoad'] | undefined = $state();

  const NODE_SELECTOR = '.node, .actor';
  const viewId = uniqueID('graph-');
  let diagramType: string | undefined;
  let editingLabel:
    | { height: number; left: number; name: string; top: number; value: string; width: number }
    | undefined = $state();

  let isSpacePressed = $state(false);
  let isPanning = $state(false);

  // Space must keep typing normally inside editors and inputs.
  const isEditableTarget = (target: EventTarget | null): boolean => {
    if (!(target instanceof Element)) {
      return false;
    }
    return (
      target.closest('input, textarea, [contenteditable="true"], .monaco-editor, .cm-editor') !==
      null
    );
  };

  const onWindowKeyDown = (event: KeyboardEvent) => {
    if (event.code === 'Space') {
      if (event.repeat || isEditableTarget(event.target)) {
        return;
      }
      // Prevent page scroll / button activation while space-panning.
      event.preventDefault();
      isSpacePressed = true;
      cancelMarquee();
      return;
    }
    if (event.key !== 'Escape') {
      return;
    }
    if (editingLabel) {
      editingLabel = undefined;
      return;
    }
    clearSelectedNode();
    clearSelectedNodes();
    highlightNames([]);
  };

  const onWindowKeyUp = (event: KeyboardEvent) => {
    if (event.code === 'Space') {
      isSpacePressed = false;
      isPanning = false;
    }
  };

  const onWindowBlur = () => {
    // Never get stuck in pan mode after alt-tab.
    isSpacePressed = false;
    isPanning = false;
  };

  // Set up panZoom state observer to update the store when pan/zoom changes
  const setupPanZoomObserver = () => {
    panZoomState.onPanZoomChange = (pan, zoomValue) => {
      // A failed panzoom init (non-invertible SVGMatrix) fires callbacks with a
      // NaN zoom: skip both the zoom sync and the state write — `??` would not
      // catch it and NaN would leak into localStorage and the URL hash.
      if (zoomValue != null && Number.isFinite(zoomValue)) {
        setZoom(zoomValue);
        updateCodeStore({ pan, zoom: zoomValue });
      }
    };
  };

  const handlePanZoom = (state: State, graphDiv: SVGSVGElement) => {
    try {
      panZoomState.updateElement(graphDiv, state);
    } catch (error) {
      console.error('PanZoom error:', error);
    }
  };

  const isNodeOffset = (value: unknown): value is NodeOffset =>
    typeof value === 'object' &&
    value !== null &&
    Number.isFinite((value as NodeOffset).dx) &&
    Number.isFinite((value as NodeOffset).dy);

  // Persisted positions come from the API payload: keep only well-formed offsets.
  const sanitizePositions = (
    raw: Record<string, NodeOffset> | undefined
  ): Record<string, NodeOffset> => {
    const clean: Record<string, NodeOffset> = {};
    for (const [name, offset] of Object.entries(raw ?? {})) {
      if (isNodeOffset(offset)) {
        clean[name] = { dx: offset.dx, dy: offset.dy };
      }
    }
    return clean;
  };

  const findNodeNameFromElement = (element: Element): string | null => {
    const actorName = element.getAttribute('name');
    if (actorName) {
      return actorName;
    }
    if (diagramType) {
      // The detector reports versioned types (e.g. flowchart-v2) while rendered
      // node ids use the unversioned name (flowchart) — try both prefixes.
      const stripped = diagramType.replace(/-v\d+$/, '');
      return (
        parseNodeId(element.id, viewId, diagramType) ??
        (stripped !== diagramType ? parseNodeId(element.id, viewId, stripped) : null)
      );
    }
    return parseNodeId(element.id, viewId, diagramType ?? '');
  };

  const findNodeName = (target: EventTarget | null): string | null => {
    if (!(target instanceof Element)) {
      return null;
    }
    const element = target.closest(NODE_SELECTOR);
    if (!element) {
      return null;
    }
    return findNodeNameFromElement(element);
  };

  // Sequence diagrams render two rects (actor-top/actor-bottom) with the same
  // name per actor, so every element resolving to `name` must be moved. The
  // actor-box text carries no name attribute and is not addressable: clicking
  // it counts as empty space.
  const findNodeElements = (name: string): SVGElement[] => {
    const matches: SVGElement[] = [];
    for (const element of container?.querySelectorAll<SVGElement>(NODE_SELECTOR) ?? []) {
      if (findNodeNameFromElement(element) === name) {
        matches.push(element);
      }
    }
    return matches;
  };

  // Set-based highlight: every rendered element resolving to a selected name
  // gets .visual-selected (sequence actors render two boxes per name).
  const highlightNames = (names: Iterable<string>): void => {
    for (const element of container?.querySelectorAll('.visual-selected') ?? []) {
      element.classList.remove('visual-selected');
    }
    const set = new Set(names);
    for (const element of container?.querySelectorAll<SVGElement>(NODE_SELECTOR) ?? []) {
      const name = findNodeNameFromElement(element);
      if (name && set.has(name)) {
        element.classList.add('visual-selected');
      }
    }
  };

  // Single writer for selection changes: the multi store is the visual source
  // of truth; size 1 also drives the single-selection module (Monaco sync +
  // AI chip).
  const applySelection = (names: string[]): void => {
    setSelectedNodes(names);
    if (names.length === 1) {
      const line = findDefinitionLine(code, names[0]);
      setSelectedNode({
        lineText: (code.split('\n')[Math.max((line ?? 1) - 1, 0)] ?? '').trim(),
        name: names[0]
      });
    } else {
      clearSelectedNode();
    }
    highlightNames(names);
  };

  // Marquee state. `marquee` is view-relative while active; `nodeRects` is a
  // once-per-drag snapshot in page coordinates.
  let marquee = $state<{ curX: number; curY: number; startX: number; startY: number }>();
  let nodeRects: NodeRect[] = [];

  const cancelMarquee = (): void => {
    marquee = undefined;
    highlightNames(selectedNodes.current);
  };

  const startMarquee = (event: MouseEvent): void => {
    // Snapshot node rects once: nodes do not move during a marquee.
    nodeRects = [];
    for (const element of container?.querySelectorAll<SVGElement>(NODE_SELECTOR) ?? []) {
      const name = findNodeNameFromElement(element);
      if (name) {
        const { height, left, top, width } = element.getBoundingClientRect();
        nodeRects.push({ name, rect: { height, left, top, width } });
      }
    }
    marquee = {
      curX: event.clientX,
      curY: event.clientY,
      startX: event.clientX,
      startY: event.clientY
    };
  };

  const finishMarquee = (event: MouseEvent): void => {
    if (!marquee) {
      return;
    }
    const { startX, startY } = marquee;
    const rect = normalizeRect(startX, startY, event.clientX, event.clientY);
    marquee = undefined;
    if (isMarqueeClick(startX, startY, event.clientX, event.clientY)) {
      // A click, not a drag: plain click clears, shift+click keeps the set.
      if (!event.shiftKey) {
        applySelection([]);
      } else {
        highlightNames(selectedNodes.current);
      }
      return;
    }
    const hits = namesInRect(nodeRects, rect);
    applySelection(event.shiftKey ? [...new Set([...selectedNodes.current, ...hits])] : hits);
  };

  // Offsets use the CSS `translate` property, which composes with mermaid's
  // `transform` presentation attribute; an inline `transform` would override
  // the attribute and teleport the node near the SVG origin.
  const setElementOffset = (element: SVGElement, dx: number, dy: number) => {
    // The CSS `translate` property takes space-separated values (commas belong
    // to the `transform` function syntax and are rejected by the CSSOM).
    element.style.setProperty('translate', `${dx}px ${dy}px`);
  };

  // Apply saved offsets to freshly rendered nodes.
  const applyPositions = () => {
    for (const [name, offset] of Object.entries(nodePositions.value)) {
      if (!isNodeOffset(offset)) {
        continue;
      }
      for (const element of findNodeElements(name)) {
        setElementOffset(element, offset.dx, offset.dy);
      }
    }
  };

  let dragState:
    | {
        baseOffsets: Record<string, NodeOffset>;
        lastDx: number;
        lastDy: number;
        moved: boolean;
        names: string[];
        startX: number;
        startY: number;
      }
    | undefined;

  // Capture phase: decide who owns this mousedown BEFORE svg-pan-zoom's own
  // (bubble-phase, svg-element) listener sees it. stopPropagation() here keeps
  // the event from ever reaching the SVG, so empty-space drags cannot pan.
  // Note: this also suppresses our own bubble-phase onmousedown for that event.
  const onContainerMouseDownCapture = (event: MouseEvent) => {
    if (event.button !== 0) {
      return;
    }
    if (isSpacePressed) {
      isPanning = true;
      return; // let svg-pan-zoom pan (anywhere, including over nodes)
    }
    if (!editable) {
      return; // /view keeps plain-drag pan
    }
    if (findNodeName(event.target)) {
      return; // node path — handled by onContainerMouseDown
    }
    event.stopPropagation();
    startMarquee(event);
  };

  // Chrome also fires a pointerdown for every mousedown, and Hammerjs listens
  // for pointerdown on the SVG (panZoom.ts's customEventsHandler) — gating
  // mousedown alone still lets Hammer pan on plain empty-canvas drags. Gate
  // mouse pointerdown through the same path; touch pointerdown must pass so
  // the Hammer touch pinch/pan path keeps working.
  const onContainerPointerDownCapture = (event: PointerEvent) => {
    if (event.pointerType !== 'mouse') {
      return;
    }
    onContainerMouseDownCapture(event);
  };

  const onContainerMouseDown = (event: MouseEvent) => {
    // Only the primary button selects/drags; right/middle clicks must not
    // arm dragState or steal the selection.
    if (event.button !== 0 || isSpacePressed) {
      return;
    }
    const name = findNodeName(event.target);
    if (!name) {
      // Unreachable for plain left-clicks (capture gate stops the event) but
      // kept as a safe no-op for any path that slips through.
      return;
    }
    const element = (event.target as Element).closest(NODE_SELECTOR);
    if (!(element instanceof SVGElement)) {
      return;
    }
    if (event.shiftKey) {
      // Toggle membership; shift+click never arms a drag.
      toggleSelectedNode(name);
      applySelection(selectedNodes.current);
      return;
    }
    const current = selectedNodes.current;
    // Dragging a member of a multi-selection drags the whole set; anything
    // else collapses the selection to that node first.
    const names = current.includes(name) && current.length > 1 ? current : [name];
    if (!current.includes(name)) {
      applySelection([name]);
    }
    const baseOffsets: Record<string, NodeOffset> = {};
    for (const dragName of names) {
      baseOffsets[dragName] = nodePositions.value[dragName] ?? { dx: 0, dy: 0 };
    }
    dragState = {
      baseOffsets,
      lastDx: 0,
      lastDy: 0,
      moved: false,
      names,
      startX: event.clientX,
      startY: event.clientY
    };
  };

  const onContainerMouseMove = (event: MouseEvent) => {
    if (marquee) {
      marquee = { ...marquee, curX: event.clientX, curY: event.clientY };
      const rect = normalizeRect(marquee.startX, marquee.startY, event.clientX, event.clientY);
      const hits = namesInRect(nodeRects, rect);
      // Live preview only — the set itself is written on mouseup.
      highlightNames(event.shiftKey ? [...new Set([...selectedNodes.current, ...hits])] : hits);
      return;
    }
    if (!dragState) {
      return;
    }
    // Defensive fallback in case a stale NaN zoom survived another path.
    const zoom = Number.isFinite(nodePositions.zoom) ? nodePositions.zoom : 1;
    const dx = (event.clientX - dragState.startX) / zoom;
    const dy = (event.clientY - dragState.startY) / zoom;
    if (dx === dragState.lastDx && dy === dragState.lastDy) {
      return;
    }
    dragState.lastDx = dx;
    dragState.lastDy = dy;
    panZoomState.isNodeDragging = true;
    dragState.moved = true;
    for (const name of dragState.names) {
      const base = dragState.baseOffsets[name];
      const nextDx = base.dx + dx;
      const nextDy = base.dy + dy;
      for (const element of findNodeElements(name)) {
        setElementOffset(element, nextDx, nextDy);
      }
      setNodePosition(name, nextDx, nextDy);
    }
  };

  const onContainerMouseUp = (event?: MouseEvent) => {
    isPanning = false;
    if (marquee) {
      if (event) {
        finishMarquee(event);
      } else {
        cancelMarquee(); // mouseleave: abort without changing the selection
      }
    }
    if (!dragState) {
      return;
    }
    const { moved, names } = dragState;
    dragState = undefined;
    panZoomState.isNodeDragging = false;
    if (moved) {
      persistPositions(names);
    }
  };

  const persistPositions = (changedNames: string[]) => {
    const diagram = currentDiagram.diagram;
    if (!diagram) {
      return; // unsaved diagram: positions stay in memory only
    }
    void updateDiagram(diagram.id, { positions: nodePositions.value }).catch((error) => {
      notify(
        error instanceof Error
          ? error.message
          : `Failed to save positions of ${changedNames.join(', ')}`
      );
    });
  };

  const onContainerDblClick = (event: MouseEvent) => {
    const name = findNodeName(event.target);
    if (!name) {
      return;
    }
    const element = (event.target as Element).closest(NODE_SELECTOR);
    if (!(element instanceof SVGElement)) {
      return;
    }
    const rect = element.getBoundingClientRect();
    const viewRect = view?.getBoundingClientRect();
    if (!viewRect) {
      return;
    }
    editingLabel = {
      height: Math.max(rect.height, 28),
      left: rect.left - viewRect.left,
      name,
      top: rect.top - viewRect.top,
      value: element.textContent?.trim() ?? '',
      width: Math.max(rect.width, 80)
    };
  };

  const submitLabel = () => {
    if (!editingLabel) {
      return;
    }
    const { name, value } = editingLabel;
    editingLabel = undefined;
    const nextCode = replaceNodeLabel(code, name, value.trim(), diagramType);
    if (nextCode === null) {
      notify('Label editing not supported here, try the AI bar');
      return;
    }
    updateCode(nextCode);
  };

  const onLabelKeydown = (event: KeyboardEvent) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      submitLabel();
    } else if (event.key === 'Escape') {
      event.preventDefault();
      editingLabel = undefined;
    }
  };

  // Marquee rectangle translated into view-relative coordinates for the
  // overlay component (the overlay lives inside #view, not #container).
  const marqueeRect = $derived.by(() => {
    if (!marquee || !view) {
      return undefined;
    }
    const viewRect = view.getBoundingClientRect();
    return normalizeRect(
      marquee.startX - viewRect.left,
      marquee.startY - viewRect.top,
      marquee.curX - viewRect.left,
      marquee.curY - viewRect.top
    );
  });

  const handleStateChange = async (state: ValidatedState) => {
    const startTime = Date.now();
    if (state.error !== undefined) {
      error = true;
      return;
    }
    error = false;
    try {
      if (container) {
        manualUpdate = true;
        // Do not render if there is no change in Code/Config/PanZoom
        if (
          code === state.code &&
          config === state.mermaid &&
          rough === state.rough &&
          panZoom === state.panZoom
        ) {
          return;
        }

        if (!shouldRefreshView()) {
          return;
        }

        code = state.code;
        config = state.mermaid;
        rough = state.rough;
        panZoom = state.panZoom ?? true;

        if (mayContainFontAwesome(code)) {
          await waitForFontAwesomeToLoad?.();
        }

        const scroll = view?.parentElement?.scrollTop;
        const { diagramType: detectedDiagramType, graphDiv } = await renderAndPlaceDiagram({
          code,
          config: JSON.parse(state.mermaid) as MermaidConfig,
          container,
          rough: state.rough,
          viewId
        });
        diagramType = detectedDiagramType;
        if (graphDiv && state.panZoom) {
          handlePanZoom(state, graphDiv);
        }
        if (graphDiv) {
          applyPositions();
          // Re-rendering replaces the container contents, dropping the
          // visual-selected class — restore it for the selected set.
          highlightNames(selectedNodes.current);
        }
        if (view?.parentElement && scroll) {
          view.parentElement.scrollTop = scroll;
        }
        error = false;
      } else if (manualUpdate) {
        manualUpdate = false;
      }
    } catch (error_) {
      console.error('view fail', error_);
      error = true;
    }
    const renderTime = Date.now() - startTime;
    saveStatistics({ code, diagramType, isRough: state.rough, renderTime });
    recordRenderTime(renderTime, () => {
      updateCodeStore({ updateDiagram: true });
    });
  };

  onMount(() => {
    setupPanZoomObserver();
  });

  // Reload saved node offsets when the active diagram changes. A different
  // diagram invalidates the current selection — its node names are stale.
  // (Visual un-highlighting happens via the chip-clear effect below.)
  $effect(() => {
    const diagram = currentDiagram.diagram;
    resetNodePositions(sanitizePositions(diagram?.positions));
    clearSelectedNode();
    clearSelectedNodes();
  });

  // The AI-bar chip and other external clearSelectedNode() callers must drop
  // the whole selection. When the multi-set is larger than 1 the single
  // selection is intentionally absent (applySelection cleared it), so only
  // clear for sets of size <= 1.
  $effect(() => {
    if (selectedNode.current || editingLabel || selectedNodes.current.length > 1) {
      return;
    }
    if (selectedNodes.current.length === 1) {
      clearSelectedNodes();
    }
    highlightNames([]);
  });

  // Queue state changes to avoid race condition
  let pendingStateChange = Promise.resolve();
  $effect(() => {
    const state = validatedState.current;
    // eslint-disable-next-line @typescript-eslint/no-empty-function
    pendingStateChange = pendingStateChange.then(() => handleStateChange(state).catch(() => {}));
  });
</script>

<FontAwesome bind:waitForFontAwesomeToLoad />

<div
  id="view"
  bind:this={view}
  class={[
    'relative h-full w-full',
    shouldShowGrid && `grid-bg-${mode.current}`,
    error && 'opacity-50',
    isSpacePressed && !isPanning && 'cursor-grab',
    isPanning && 'cursor-grabbing'
  ]}>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    id="container"
    bind:this={container}
    class="h-full overflow-auto"
    ondblclick={editable ? onContainerDblClick : undefined}
    onmousedowncapture={onContainerMouseDownCapture}
    onpointerdowncapture={onContainerPointerDownCapture}
    onmousedown={editable ? onContainerMouseDown : undefined}
    onmousemove={editable ? onContainerMouseMove : undefined}
    onmouseup={editable ? onContainerMouseUp : undefined}
    onmouseleave={editable ? onContainerMouseUp : undefined}>
  </div>
  {#if marqueeRect}
    <Marquee rect={marqueeRect} />
  {/if}
  {#if editingLabel}
    <input
      bind:value={editingLabel.value}
      class="absolute z-10 rounded-sm border-2 border-accent bg-background px-1 text-sm text-foreground shadow-lg focus:outline-none"
      data-testid={TID.nodeLabelInput}
      style="height: {editingLabel.height}px; left: {editingLabel.left}px; top: {editingLabel.top}px; width: {editingLabel.width}px"
      onblur={submitLabel}
      onkeydown={onLabelKeydown} />
  {/if}
</div>

<svelte:window onblur={onWindowBlur} onkeydown={onWindowKeyDown} onkeyup={onWindowKeyUp} />

<style>
  .grid-bg-light {
    background-size: 30px 30px;
    background-image: radial-gradient(circle, #e4e4e48c 2px, #0000 2px);
  }

  .grid-bg-dark {
    background-size: 30px 30px;
    background-image: radial-gradient(circle, #46464646 2px, #0000 2px);
  }

  :global(.visual-selected) {
    filter: drop-shadow(0 0 4px var(--color-accent, #7c3aed));
  }
</style>
