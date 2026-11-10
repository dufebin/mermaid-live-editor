# Canvas Multi-Select + Space-to-Pan Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Figma-style canvas interaction to the diagram view: marquee multi-select with group drag, and Space-held mouse panning.

**Architecture:** All interaction changes live in the mouse-event layer of `View.svelte`; a capture-phase `mousedown` gate on the container decides between svg-pan-zoom's built-in pan (Space held), node select/drag (node target), and marquee (empty space, editable). Multi-select state is a new rune module (`selectedNodes.svelte.ts`) that stays the visual source of truth and drives the existing single-selection module for Monaco cursor sync. Group drag reuses the per-node dx/dy offset store (`nodePositions.svelte.ts`); the backend `positions` payload is unchanged.

**Tech Stack:** SvelteKit 2 + Svelte 5 runes, svg-pan-zoom (unchanged), Tailwind 4, vitest (jsdom), Playwright.

**Spec:** `specs/2026-11-10-canvas-multiselect-space-pan-design.md`

## Global Constraints

- Package manager is pnpm; Node >= 24.
- Svelte 5 runes only (`$state`, `$derived`, `$effect`); no legacy stores.
- ESLint enforces alphabetically sorted keys for objects with 5+ keys in `src/` — run `pnpm lint:fix` before committing.
- `docs/` is build output — never edit or commit anything under `docs/`.
- Unit tests are colocated (`*.test.ts` next to source); run a single file with `pnpm vitest run src/lib/util/<file>.test.ts`.
- If `.claude/worktrees/` contains repo copies, scope vitest with `--dir src`.
- Playwright can be flaky under parallel workers — run e2e with `--workers=1` before assuming a real failure.
- HMR is disabled by design; every dev-server change is a full reload.
- Every commit message ends with:

  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  ```

---

### Task 1: Marquee geometry helpers (pure functions)

**Files:**

- Create: `src/lib/util/marqueeSelection.ts`
- Test: `src/lib/util/marqueeSelection.test.ts`

**Interfaces:**

- Consumes: nothing (pure module).
- Produces (used by Task 4 and Task 5 in `View.svelte`):
  - `interface Rect { height: number; left: number; top: number; width: number }`
  - `MARQUEE_CLICK_THRESHOLD_PX = 3`
  - `normalizeRect(x1: number, y1: number, x2: number, y2: number): Rect` — axis-independent rectangle from two corners
  - `rectsIntersect(a: Rect, b: Rect): boolean` — closed-interval overlap (touching edges count)
  - `isMarqueeClick(x1: number, y1: number, x2: number, y2: number): boolean` — both axis deltas below threshold
  - `namesInRect(nodes: { name: string; rect: Rect }[], marquee: Rect): string[]` — unique names (DOM order) of entries whose rect intersects

- [ ] **Step 1: Write the failing tests**

Create `src/lib/util/marqueeSelection.test.ts`:

```ts
import { describe, expect, it } from 'vitest';
import { isMarqueeClick, namesInRect, normalizeRect, rectsIntersect } from './marqueeSelection';

describe('normalizeRect', () => {
  it('normalizes dragged-in-any-direction corners', () => {
    expect(normalizeRect(100, 50, 40, 10)).toEqual({
      height: 40,
      left: 40,
      top: 10,
      width: 60
    });
  });
});

describe('rectsIntersect', () => {
  it('detects overlap', () => {
    expect(
      rectsIntersect(
        { height: 10, left: 0, top: 0, width: 10 },
        {
          height: 10,
          left: 5,
          top: 5,
          width: 10
        }
      )
    ).toBe(true);
  });

  it('rejects disjoint rects', () => {
    expect(
      rectsIntersect(
        { height: 10, left: 0, top: 0, width: 10 },
        {
          height: 10,
          left: 11,
          top: 0,
          width: 10
        }
      )
    ).toBe(false);
  });

  it('treats touching edges as intersecting', () => {
    expect(
      rectsIntersect(
        { height: 10, left: 0, top: 0, width: 10 },
        {
          height: 10,
          left: 10,
          top: 0,
          width: 10
        }
      )
    ).toBe(true);
  });
});

describe('isMarqueeClick', () => {
  it('treats sub-threshold movement as a click', () => {
    expect(isMarqueeClick(10, 10, 12, 12)).toBe(true);
  });

  it('treats real drags as a marquee', () => {
    expect(isMarqueeClick(10, 10, 40, 12)).toBe(false);
  });
});

describe('namesInRect', () => {
  const nodes = [
    { name: 'A', rect: { height: 10, left: 0, top: 0, width: 10 } },
    { name: 'B', rect: { height: 10, left: 100, top: 0, width: 10 } },
    // sequence-actor case: two boxes, same name — hitting either selects it
    { name: 'C', rect: { height: 10, left: 200, top: 0, width: 10 } },
    { name: 'C', rect: { height: 10, left: 0, top: 500, width: 10 } }
  ];

  it('returns intersecting names in DOM order', () => {
    expect(namesInRect(nodes, { height: 100, left: 0, top: 0, width: 300 })).toEqual([
      'A',
      'B',
      'C'
    ]);
  });

  it('dedupes names with multiple boxes', () => {
    expect(namesInRect(nodes, { height: 20, left: 0, top: 490, width: 20 })).toEqual(['C']);
  });

  it('returns empty for no hits', () => {
    expect(namesInRect(nodes, { height: 5, left: 400, top: 400, width: 5 })).toEqual([]);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm vitest run src/lib/util/marqueeSelection.test.ts`
Expected: FAIL — cannot resolve `./marqueeSelection`.

- [ ] **Step 3: Write the implementation**

Create `src/lib/util/marqueeSelection.ts`:

```ts
export interface Rect {
  height: number;
  left: number;
  top: number;
  width: number;
}

export interface NodeRect {
  name: string;
  rect: Rect;
}

export const MARQUEE_CLICK_THRESHOLD_PX = 3;

export const normalizeRect = (x1: number, y1: number, x2: number, y2: number): Rect => ({
  height: Math.abs(y2 - y1),
  left: Math.min(x1, x2),
  top: Math.min(y1, y2),
  width: Math.abs(x2 - x1)
});

export const rectsIntersect = (a: Rect, b: Rect): boolean =>
  a.left <= b.left + b.width &&
  b.left <= a.left + a.width &&
  a.top <= b.top + b.height &&
  b.top <= a.top + a.height;

export const isMarqueeClick = (x1: number, y1: number, x2: number, y2: number): boolean =>
  Math.abs(x2 - x1) < MARQUEE_CLICK_THRESHOLD_PX && Math.abs(y2 - y1) < MARQUEE_CLICK_THRESHOLD_PX;

export const namesInRect = (nodes: NodeRect[], marquee: Rect): string[] => {
  const hits: string[] = [];
  for (const node of nodes) {
    if (rectsIntersect(node.rect, marquee) && !hits.includes(node.name)) {
      hits.push(node.name);
    }
  }
  return hits;
};
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm vitest run src/lib/util/marqueeSelection.test.ts`
Expected: PASS (all 8 tests).

- [ ] **Step 5: Lint and commit**

```bash
pnpm lint:fix
git add src/lib/util/marqueeSelection.ts src/lib/util/marqueeSelection.test.ts
git commit -m "feat: add marquee selection geometry helpers

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 2: Multi-select state store

**Files:**

- Create: `src/lib/util/selectedNodes.svelte.ts`
- Test: `src/lib/util/selectedNodes.test.ts`

**Interfaces:**

- Consumes: nothing.
- Produces (used by Tasks 4–5 in `View.svelte`):
  - `selectedNodes.current: string[]` — reactive getter for the selected node-name set (DOM order, unique)
  - `setSelectedNodes(names: string[]): void` — replaces the set, dedupes
  - `toggleSelectedNode(name: string): void` — adds or removes one name
  - `clearSelectedNodes(): void` — empties the set
- Modeled on `src/lib/util/selectedNode.svelte.ts` (single-selection module, unchanged).

- [ ] **Step 1: Write the failing tests**

Create `src/lib/util/selectedNodes.test.ts`:

```ts
import { describe, expect, it } from 'vitest';
import {
  clearSelectedNodes,
  selectedNodes,
  setSelectedNodes,
  toggleSelectedNode
} from './selectedNodes.svelte';

describe('selectedNodes store', () => {
  it('starts empty', () => {
    expect(selectedNodes.current).toEqual([]);
  });

  it('sets and dedupes', () => {
    setSelectedNodes(['A', 'B', 'A']);
    expect(selectedNodes.current).toEqual(['A', 'B']);
  });

  it('toggles membership', () => {
    setSelectedNodes(['A', 'B']);
    toggleSelectedNode('C');
    expect(selectedNodes.current).toEqual(['A', 'B', 'C']);
    toggleSelectedNode('A');
    expect(selectedNodes.current).toEqual(['B', 'C']);
  });

  it('clears', () => {
    setSelectedNodes(['A']);
    clearSelectedNodes();
    expect(selectedNodes.current).toEqual([]);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `pnpm vitest run src/lib/util/selectedNodes.test.ts`
Expected: FAIL — cannot resolve `./selectedNodes.svelte`.

- [ ] **Step 3: Write the implementation**

Create `src/lib/util/selectedNodes.svelte.ts`:

```ts
// Ephemeral multi-selection state for the canvas. Never persisted to
// localStorage or the URL hash. View.svelte is the sole writer and keeps
// the single-selection module (selectedNode.svelte.ts) in sync with it.
let names: string[] = $state.raw([]);

export const selectedNodes = {
  get current(): string[] {
    return names;
  }
};

export const setSelectedNodes = (next: string[]): void => {
  names = [...new Set(next)];
};

export const toggleSelectedNode = (name: string): void => {
  names = names.includes(name) ? names.filter((current) => current !== name) : [...names, name];
};

export const clearSelectedNodes = (): void => {
  names = [];
};
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `pnpm vitest run src/lib/util/selectedNodes.test.ts`
Expected: PASS (4 tests). If the runner reports `$state` is not defined, confirm `vite.config.js` still has `resolve: process.env.VITEST ? { conditions: ['browser'] } : undefined` — that line is what makes rune modules testable; do not remove it.

- [ ] **Step 5: Lint and commit**

```bash
pnpm lint:fix
git add src/lib/util/selectedNodes.svelte.ts src/lib/util/selectedNodes.test.ts
git commit -m "feat: add multi-select node state store

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 3: Space-to-pan gating

Empty-canvas mouse drag must stop panning on the editable page (it becomes marquee in Task 4); Space+drag pans everywhere on the canvas. The `/view` read-only page keeps plain-drag panning.

Key mechanism: a **capture-phase** `mousedown` listener on the container calls `event.stopPropagation()` for empty-space clicks when Space is not held. svg-pan-zoom binds its own mousedown on the inner SVG element — it never sees the stopped event, so it cannot start a pan. When Space IS held the event passes through and svg-pan-zoom's built-in mouse pan does the work. `panZoom.ts` is not modified; the Hammer touch path is untouched.

> **As-built correction (verified during Task 3 implementation and review):** on pointer-event browsers the desktop pan is driven by Hammer's PointerEventInput bound to the SVG (via `panZoom.ts` `customEventsHandler`), so a mousedown-only gate does NOT stop it. The implemented gate is reached from TWO capture listeners on `#container`: `onmousedowncapture` and `onContainerPointerDownCapture` (wired via `onpointerdowncapture`, which delegates to the same gate for `pointerType === 'mouse'` only — touch/pen pointerdown always passes through). Both fire per mouse press with identical coordinates, so the gate is idempotent by design. Tasks 4–5 must hook this shared gate, never gate touch events.

Important Svelte/DOM detail: `stopPropagation()` during the capture phase on the container also prevents the container's own **bubble-phase** listeners (`onmousedown` etc.) from firing for that event — the old "click empty space clears selection" branch in `onContainerMouseDown` becomes unreachable and is removed here (click-to-clear returns in Task 4 via the marquee click threshold).

**Files:**

- Modify: `src/lib/components/View.svelte`
- Test: `tests/canvasInteraction.spec.ts` (new e2e spec)

**Interfaces:**

- Consumes: nothing from Tasks 1–2.
- Produces (used by Tasks 4–5):
  - `let isSpacePressed = $state(false)` in `View.svelte`
  - `const isEditableTarget = (target: EventTarget | null): boolean` in `View.svelte`
  - shared capture gate `onContainerMouseDownCapture`, wired via BOTH `onmousedowncapture` and `onContainerPointerDownCapture` (`onpointerdowncapture`, `pointerType === 'mouse'` only) on `#container`

- [ ] **Step 1: Write the failing e2e tests**

Create `tests/canvasInteraction.spec.ts`:

```ts
import { expect, test } from './test';

const viewportTransform = async (page: import('@playwright/test').Page): Promise<string | null> =>
  page.locator('.svg-pan-zoom_viewport').getAttribute('transform');

test.describe('Canvas interaction', () => {
  test('space+drag pans the canvas', async ({ editPage }) => {
    const { page } = editPage;
    await editPage.checkTextInView('Christmas');
    const view = await editPage.view.boundingBox();
    expect(view).not.toBeNull();
    const before = await viewportTransform(page);
    await page.keyboard.down('Space');
    await page.mouse.move(view!.x + 100, view!.y + 100);
    await page.mouse.down();
    await page.mouse.move(view!.x + 200, view!.y + 180, { steps: 5 });
    await page.mouse.up();
    await page.keyboard.up('Space');
    const after = await viewportTransform(page);
    expect(after).not.toBeNull();
    expect(after).not.toBe(before);
  });

  test('plain empty-canvas drag does not pan', async ({ editPage }) => {
    const { page } = editPage;
    await editPage.checkTextInView('Christmas');
    const view = await editPage.view.boundingBox();
    expect(view).not.toBeNull();
    const before = await viewportTransform(page);
    await page.mouse.move(view!.x + 100, view!.y + 100);
    await page.mouse.down();
    await page.mouse.move(view!.x + 200, view!.y + 180, { steps: 5 });
    await page.mouse.up();
    const after = await viewportTransform(page);
    expect(after).toBe(before);
  });
});
```

- [ ] **Step 2: Run e2e to verify they fail**

Run: `pnpm test:e2e tests/canvasInteraction.spec.ts --workers=1`
Expected: both FAIL — the first because Space currently does nothing (no pan → transform unchanged), the second because plain drag still pans (transform changes).

- [ ] **Step 3: Implement in View.svelte**

3a. Add state and helpers after the `let editingLabel` declaration (around line 51):

```ts
let isSpacePressed = $state(false);
let isPanning = $state(false);

// Space must keep typing normally inside editors and inputs.
const isEditableTarget = (target: EventTarget | null): boolean => {
  if (!(target instanceof Element)) {
    return false;
  }
  return (
    target.closest('input, textarea, [contenteditable="true"], .monaco-editor, .cm-editor') !== null
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
  highlight(undefined);
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
```

(The Escape branch is the body of the existing `<svelte:window>` handler, moved into `onWindowKeyDown`.)

3b. Add the capture-phase gate (place next to `onContainerMouseDown`):

```ts
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
  // Task 4 starts the marquee here.
};
```

3c. Early-return in `onContainerMouseDown` — replace its opening:

```ts
  const onContainerMouseDown = (event: MouseEvent) => {
    if (event.button !== 0 || isSpacePressed) {
      return;
    }
    const name = findNodeName(event.target);
    if (!name) {
      // Unreachable for plain left-clicks (capture gate stops the event) but
      // kept as a safe no-op for any path that slips through.
      return;
    }
```

(Delete the old `clearSelectedNode(); highlight(undefined);` lines in that branch — click-to-clear comes back with the marquee click threshold in Task 4.)

3d. Clear `isPanning` in `onContainerMouseUp` (add as its first statement):

```ts
isPanning = false;
```

3e. Wire the template — on the `#container` div add the capture handler:

```svelte
onmousedowncapture={onContainerMouseDownCapture}
```

Replace the `<svelte:window>` block at the bottom with:

```svelte
<svelte:window onblur={onWindowBlur} onkeydown={onWindowKeyDown} onkeyup={onWindowKeyUp} />
```

3f. Cursor classes — on the outer `#view` div, extend the `class` array:

```svelte
class={[
  'relative h-full w-full',
  shouldShowGrid && `grid-bg-${mode.current}`,
  error && 'opacity-50',
  isSpacePressed && !isPanning && 'cursor-grab',
  isPanning && 'cursor-grabbing'
]}
```

- [ ] **Step 4: Verify types and lint**

Run: `pnpm check && pnpm lint:fix`
Expected: no errors.

- [ ] **Step 5: Run e2e to verify they pass**

Run: `pnpm test:e2e tests/canvasInteraction.spec.ts --workers=1`
Expected: both PASS. If flaky, retry once before investigating.

- [ ] **Step 6: Commit**

```bash
git add src/lib/components/View.svelte tests/canvasInteraction.spec.ts
git commit -m "feat: pan canvas with space+drag, stop plain empty-drag panning

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 4: Marquee selection

Empty-canvas drag (editable, no Space) draws a selection rectangle; intersecting nodes get a live highlight preview; on mouseup the selection set is written. Sub-threshold drags are clicks: plain click clears the selection, Shift+click keeps it. Shift+drag unions with the existing set. Highlighting generalizes from one element to a name set, and the selection-clearing effect is reworked so the AI-bar chip (`clearSelectedNode` from `AIEditBar.svelte`) also drops the multi-selection.

**Files:**

- Modify: `src/lib/components/View.svelte`
- Create: `src/lib/components/Marquee.svelte`
- Modify: `src/lib/constants.ts` (add one TID entry)
- Test: `tests/canvasInteraction.spec.ts` (extend)

**Interfaces:**

- Consumes from Task 1: `NodeRect`, `isMarqueeClick`, `namesInRect`, `normalizeRect`.
- Consumes from Task 2: `clearSelectedNodes`, `selectedNodes`, `setSelectedNodes`.
- Consumes from Task 3: `isSpacePressed`, capture handler structure.
- Produces (used by Task 5):
  - `const highlightNames = (names: Iterable<string>): void` — applies `.visual-selected` to every element of the given node names (replaces `highlight`/`selectedSvg`)
  - `const applySelection = (names: string[]): void` — writes the multi store, syncs/clears the single selection (Monaco + AI chip), re-highlights
  - marquee state (`marquee`, `nodeRects`, `startMarquee`, `cancelMarquee`, `finishMarquee`)

- [ ] **Step 1: Write the failing e2e test**

Append to the `test.describe('Canvas interaction', ...)` block in `tests/canvasInteraction.spec.ts`:

```ts
test('marquee selects multiple nodes and click clears', async ({ editPage }) => {
  const { page } = editPage;
  await editPage.clearEditor();
  await editPage.typeInEditor('flowchart LR\n A[Alpha] --> B[Beta]');
  await editPage.checkTextInView('Beta');
  const view = await editPage.view.boundingBox();
  expect(view).not.toBeNull();
  // Drag from the far-left edge across both nodes.
  await page.mouse.move(view!.x + 5, view!.y + view!.height / 2);
  await page.mouse.down();
  await page.mouse.move(view!.x + view!.width - 5, view!.y + view!.height / 2, { steps: 5 });
  await page.mouse.up();
  await expect(page.locator('.visual-selected')).toHaveCount(2);

  // A plain click on empty space clears the selection.
  await page.mouse.click(view!.x + 5, view!.y + 5);
  await expect(page.locator('.visual-selected')).toHaveCount(0);
});
```

- [ ] **Step 2: Run e2e to verify it fails**

Run: `pnpm test:e2e tests/canvasInteraction.spec.ts --workers=1 -g "marquee"`
Expected: FAIL — `.visual-selected` count stays 0 (no marquee exists yet).

- [ ] **Step 3: Add the TID constant**

In `src/lib/constants.ts`, inside `TID` (keys are alphabetically sorted; insert between `importInput` and `navbarDownloadPNG`):

```ts
  marquee: 'marquee-selection',
```

- [ ] **Step 4: Create the Marquee overlay component**

Create `src/lib/components/Marquee.svelte`:

```svelte
<script lang="ts">
  import { TID } from '$/constants';
  let { rect }: { rect: { height: number; left: number; top: number; width: number } } = $props();
</script>

<!-- pointer-events-none so the overlay never steals mouse events -->
<div
  class="pointer-events-none absolute z-10 rounded-sm border-2 border-accent bg-accent/10"
  data-testid={TID.marquee}
  style="height: {rect.height}px; left: {rect.left}px; top: {rect.top}px; width: {rect.width}px">
</div>
```

- [ ] **Step 5: Implement marquee + set-based highlight in View.svelte**

5a. Extend imports:

```ts
import { Marquee } from '$/components/Marquee.svelte';
import { isMarqueeClick, namesInRect, normalizeRect } from '$/util/marqueeSelection';
import type { NodeRect } from '$/util/marqueeSelection';
import { clearSelectedNodes, selectedNodes, setSelectedNodes } from '$/util/selectedNodes.svelte';
```

(Match the existing import style: `import type { ... }` on its own line, then value imports, as in the current header of `View.svelte`.)

5b. Replace `highlight`/`selectedSvg` (delete `let selectedSvg ...` and the old `highlight` function) with:

```ts
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
```

5c. Marquee state and handlers (place after `applySelection`):

```ts
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
```

5d. Hook the marquee into the handlers:

- Shared capture gate (`onContainerMouseDownCapture`): replace the trailing comment `// Task 4 starts the marquee here.` with `startMarquee(event);` (keep the `event.stopPropagation();` line above it). The gate is invoked from BOTH `onmousedowncapture` and the mouse-only `onpointerdowncapture` path, so `startMarquee` runs twice per press with identical coordinates — it must stay idempotent (it is: same start point, re-snapshot of node rects). Never gate touch pointer events.
- Space keydown in `onWindowKeyDown`: after `isSpacePressed = true;` add `cancelMarquee();`.
- `onContainerMouseMove`: add a marquee branch at the top:

```ts
if (marquee) {
  marquee = { ...marquee, curX: event.clientX, curY: event.clientY };
  const rect = normalizeRect(marquee.startX, marquee.startY, event.clientX, event.clientY);
  const hits = namesInRect(nodeRects, rect);
  // Live preview only — the set itself is written on mouseup.
  highlightNames(event.shiftKey ? [...new Set([...selectedNodes.current, ...hits])] : hits);
  return;
}
```

- `onContainerMouseUp`: change signature to accept the event and finalize the marquee:

```ts
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
  // ... existing dragState finalization unchanged ...
};
```

(The `onmouseleave` wiring stays `onmouseleave={onContainerMouseUp}` — Svelte passes the event, which is fine: leaving the view mid-marquee then finalizes with the leave coordinates. If you prefer abort-on-leave, wire `onmouseleave={() => onContainerMouseUp()}` instead — either is acceptable; pick one and keep the drag path consistent.)

5e. Marquee overlay rect (view-relative, for the component). Add after the handlers:

```ts
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
```

Render it inside the `#view` div, next to the `{#if editingLabel}` block:

```svelte
{#if marqueeRect}
  <Marquee rect={marqueeRect} />
{/if}
```

5f. Rework the selection-clear effect. Replace the existing effect:

```ts
$effect(() => {
  if (!selectedNode.current && !editingLabel) {
    highlight(undefined);
  }
});
```

with:

```ts
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
```

5g. Re-render restore. In `handleStateChange`, replace the block:

```ts
const selected = selectedNode.current;
if (selected) {
  highlight(findNodeElements(selected.name)[0]);
}
```

with:

```ts
highlightNames(selectedNodes.current);
```

5h. Escape in `onWindowKeyDown` — extend the final branch to also clear the multi set (replace `clearSelectedNode(); highlight(undefined);`):

```ts
clearSelectedNode();
clearSelectedNodes();
highlightNames([]);
```

Also update the diagram-change effect (it calls `clearSelectedNode()`): add `clearSelectedNodes();` next to it — switching diagrams invalidates node names.

- [ ] **Step 6: Verify types and lint**

Run: `pnpm check && pnpm lint:fix`
Expected: no errors (`highlight`, `selectedSvg` fully removed; no unused imports).

- [ ] **Step 7: Run all e2e in the spec**

Run: `pnpm test:e2e tests/canvasInteraction.spec.ts --workers=1`
Expected: 3 PASS (space-pan, no-plain-pan, marquee).

- [ ] **Step 8: Commit**

```bash
git add src/lib/components/View.svelte src/lib/components/Marquee.svelte src/lib/constants.ts tests/canvasInteraction.spec.ts
git commit -m "feat: marquee multi-select on empty-canvas drag

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 5: Shift+click toggle and group drag

Shift+click on a node toggles its membership. Mousedown on a node that is part of a multi-selection (size > 1) arms a group drag: every selected node moves by the same delta/zoom, and one API call persists all positions. Mousedown on a node outside the set collapses the selection to that node (existing single behavior).

**Files:**

- Modify: `src/lib/components/View.svelte`
- Test: `tests/canvasInteraction.spec.ts` (extend)

**Interfaces:**

- Consumes from Task 2: `toggleSelectedNode`, `selectedNodes`.
- Consumes from Task 4: `applySelection`, `highlightNames`.
- Consumes: `NodeOffset` from `$/util/nodePositions.svelte` (already imported in View.svelte as a type — verify; add if missing).
- Produces: none (end of feature).

- [ ] **Step 1: Write the failing e2e tests**

Append to `tests/canvasInteraction.spec.ts` inside the describe block:

```ts
test('shift+click toggles selection membership', async ({ editPage }) => {
  const { page } = editPage;
  await editPage.clearEditor();
  await editPage.typeInEditor('flowchart LR\n A[Alpha] --> B[Beta]');
  await editPage.checkTextInView('Beta');
  const alpha = page.locator('#view .node').filter({ hasText: 'Alpha' });
  const beta = page.locator('#view .node').filter({ hasText: 'Beta' });
  await alpha.click();
  await expect(page.locator('.visual-selected')).toHaveCount(1);
  await beta.click({ modifiers: ['Shift'] });
  await expect(page.locator('.visual-selected')).toHaveCount(2);
  await beta.click({ modifiers: ['Shift'] });
  await expect(page.locator('.visual-selected')).toHaveCount(1);
});

test('group drag moves selected nodes together', async ({ editPage }) => {
  const { page } = editPage;
  await editPage.clearEditor();
  await editPage.typeInEditor('flowchart LR\n A[Alpha] --> B[Beta]');
  await editPage.checkTextInView('Beta');
  const view = await editPage.view.boundingBox();
  expect(view).not.toBeNull();
  // Marquee-select both nodes.
  await page.mouse.move(view!.x + 5, view!.y + view!.height / 2);
  await page.mouse.down();
  await page.mouse.move(view!.x + view!.width - 5, view!.y + view!.height / 2, { steps: 5 });
  await page.mouse.up();
  await expect(page.locator('.visual-selected')).toHaveCount(2);

  // Drag one of them; both must move by the same delta.
  const alpha = page.locator('#view .node').filter({ hasText: 'Alpha' });
  const beta = page.locator('#view .node').filter({ hasText: 'Beta' });
  const box = await alpha.boundingBox();
  expect(box).not.toBeNull();
  await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2);
  await page.mouse.down();
  await page.mouse.move(box!.x + box!.width / 2 + 60, box!.y + box!.height / 2 + 40, {
    steps: 5
  });
  await page.mouse.up();

  const translateOf = (locator: import('@playwright/test').Locator) =>
    locator.evaluate((element) => (element as SVGElement).style.translate);
  const t1 = await translateOf(alpha);
  const t2 = await translateOf(beta);
  expect(t1).not.toBe('');
  expect(t1).not.toBe('0px 0px');
  expect(t1).toBe(t2);
});
```

- [ ] **Step 2: Run e2e to verify they fail**

Run: `pnpm test:e2e tests/canvasInteraction.spec.ts --workers=1 -g "shift|group drag"`
Expected: both FAIL — shift+click currently replaces the selection with one node; group drag moves only the dragged node.

- [ ] **Step 3: Implement in View.svelte**

3a. Extend imports: add `toggleSelectedNode` to the `selectedNodes.svelte` import.

3b. Replace `dragState` (single-node shape) and `onContainerMouseDown`'s node path. New drag state and handler:

```ts
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

const onContainerMouseDown = (event: MouseEvent) => {
  if (event.button !== 0 || isSpacePressed) {
    return;
  }
  const name = findNodeName(event.target);
  if (!name) {
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
```

3c. Replace `onContainerMouseMove`'s drag branch (keep the marquee branch from Task 4 at the top):

```ts
const onContainerMouseMove = (event: MouseEvent) => {
  if (marquee) {
    // ... Task 4 marquee branch unchanged ...
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
```

3d. `onContainerMouseUp` — update the drag finalization to pass the whole list:

```ts
if (!dragState) {
  return;
}
const { moved, names } = dragState;
dragState = undefined;
panZoomState.isNodeDragging = false;
if (moved) {
  persistPositions(names);
}
```

3e. `persistPositions` — change the signature and message:

```ts
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
```

- [ ] **Step 4: Verify types and lint**

Run: `pnpm check && pnpm lint:fix`
Expected: no errors.

- [ ] **Step 5: Run the full e2e spec**

Run: `pnpm test:e2e tests/canvasInteraction.spec.ts --workers=1`
Expected: 5 PASS.

- [ ] **Step 6: Run the full test suites**

Run: `pnpm vitest run --dir src && pnpm test:e2e --workers=1`
Expected: all unit tests and all e2e specs green. Single-node drag is now the `names: [name]` case of the same code path — no existing test should regress.

- [ ] **Step 7: Commit**

```bash
git add src/lib/components/View.svelte tests/canvasInteraction.spec.ts
git commit -m "feat: shift+click toggle and group drag for multi-selected nodes

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

## Self-Review Notes

- Spec coverage: marquee (Task 4), Shift+click toggle (Task 5), group drag (Task 5), space+drag pan (Task 3), `/view` unchanged (Task 3 capture gate `!editable` early return), touch untouched (no Hammer changes), re-render restore (Task 4 step 5g), single persist call (Task 5 step 3e), click threshold (Task 1 + Task 4), window blur reset (Task 3), editor/input space guard (Task 3 `isEditableTarget`).
- Type consistency: `NodeOffset` comes from `$/util/nodePositions.svelte` (already imported in View.svelte); `NodeRect`/`Rect` from Task 1; `selectedNodes.*` signatures match between Task 2 and Tasks 4–5.
- Known intentional deviation: none.
