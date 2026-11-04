<script lang="ts">
  import type { NodeOffset } from '$/util/nodePositions.svelte';
  import type { State, ValidatedState } from '$/types';
  import { updateDiagram } from '$/util/api';
  import { recordRenderTime, shouldRefreshView } from '$/util/autoSync';
  import { findDefinitionLine, parseNodeId, replaceNodeLabel } from '$/util/diagramNodes';
  import { currentDiagram } from '$/util/currentDiagram.svelte';
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
  import { TID } from '$/constants';
  import FontAwesome, { mayContainFontAwesome } from '$lib/components/FontAwesome.svelte';
  import uniqueID from 'lodash-es/uniqueId';
  import type { MermaidConfig } from 'mermaid';
  import { mode } from 'mode-watcher';
  import { onMount } from 'svelte';

  let {
    panZoomState = new PanZoomState(),
    shouldShowGrid = true
  }: { panZoomState?: PanZoomState; shouldShowGrid?: boolean } = $props();
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
  let selectedSvg: SVGElement | undefined = $state();
  let editingLabel:
    | { height: number; left: number; name: string; top: number; value: string; width: number }
    | undefined = $state();

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

  const highlight = (element: SVGElement | undefined) => {
    selectedSvg?.classList.remove('visual-selected');
    element?.classList.add('visual-selected');
    selectedSvg = element;
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
        baseDx: number;
        baseDy: number;
        moved: boolean;
        name: string;
        startX: number;
        startY: number;
      }
    | undefined;

  const onContainerMouseDown = (event: MouseEvent) => {
    const name = findNodeName(event.target);
    if (!name) {
      clearSelectedNode();
      highlight(undefined);
      return;
    }
    const element = (event.target as Element).closest(NODE_SELECTOR);
    if (!(element instanceof SVGElement)) {
      return;
    }
    const line = findDefinitionLine(code, name);
    setSelectedNode({
      lineText: (code.split('\n')[Math.max((line ?? 1) - 1, 0)] ?? '').trim(),
      name
    });
    highlight(element);
    const base = nodePositions.value[name] ?? { dx: 0, dy: 0 };
    dragState = {
      baseDx: base.dx,
      baseDy: base.dy,
      moved: false,
      name,
      startX: event.clientX,
      startY: event.clientY
    };
  };

  const onContainerMouseMove = (event: MouseEvent) => {
    if (!dragState) {
      return;
    }
    // Defensive fallback in case a stale NaN zoom survived another path.
    const zoom = Number.isFinite(nodePositions.zoom) ? nodePositions.zoom : 1;
    const dx = dragState.baseDx + (event.clientX - dragState.startX) / zoom;
    const dy = dragState.baseDy + (event.clientY - dragState.startY) / zoom;
    if (dx === dragState.baseDx && dy === dragState.baseDy) {
      return;
    }
    panZoomState.isNodeDragging = true;
    dragState.moved = true;
    for (const element of findNodeElements(dragState.name)) {
      setElementOffset(element, dx, dy);
    }
    setNodePosition(dragState.name, dx, dy);
  };

  const onContainerMouseUp = () => {
    if (!dragState) {
      return;
    }
    const { moved, name } = dragState;
    dragState = undefined;
    panZoomState.isNodeDragging = false;
    if (moved) {
      persistPositions(name);
    }
  };

  const persistPositions = (changedName: string) => {
    const diagram = currentDiagram.diagram;
    if (!diagram) {
      return; // unsaved diagram: positions stay in memory only
    }
    void updateDiagram(diagram.id, { positions: nodePositions.value }).catch((error) => {
      notify(error instanceof Error ? error.message : `Failed to save position of ${changedName}`);
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
          // visual-selected class — restore it for the still-selected node.
          const selected = selectedNode.current;
          if (selected) {
            highlight(findNodeElements(selected.name)[0]);
          }
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

  // Reload saved node offsets when the active diagram changes.
  $effect(() => {
    const diagram = currentDiagram.diagram;
    resetNodePositions(sanitizePositions(diagram?.positions));
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
    error && 'opacity-50'
  ]}>
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    id="container"
    bind:this={container}
    class="h-full overflow-auto"
    ondblclick={onContainerDblClick}
    onmousedown={onContainerMouseDown}
    onmousemove={onContainerMouseMove}
    onmouseup={onContainerMouseUp}
    onmouseleave={onContainerMouseUp}>
  </div>
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

<svelte:window
  onkeydown={(event) => {
    if (event.key !== 'Escape') {
      return;
    }
    if (editingLabel) {
      editingLabel = undefined;
      return;
    }
    clearSelectedNode();
    highlight(undefined);
  }} />

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
