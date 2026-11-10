# Canvas Multi-Select + Space-to-Pan Design

Date: 2026-11-10
Status: Approved (pending implementation)

## Goal

Bring Figma/mermaid.ai-style canvas interaction to the editor view:

1. **Multi-select** — marquee (rubber-band) selection on empty-canvas drag, Shift+click to toggle membership, group drag of the selected set.
2. **Space-to-pan** — holding Space turns the canvas into pan mode (grab cursor), dragging anywhere (including over nodes) pans instead of selecting.

Out of scope for v1: batch delete, alignment/distribution tools, multi-select persistence
(selection stays ephemeral, like the existing single selection).

## Interaction Model

| Gesture                          | Behavior                                                                                 |
| -------------------------------- | ---------------------------------------------------------------------------------------- |
| Click node                       | Single-select (current behavior): highlight + sync Monaco cursor                         |
| Drag on empty canvas             | **Marquee select** — semi-transparent rect; intersecting nodes join the selection        |
| Shift + click node               | Toggle node membership in the selection set                                              |
| Shift + drag on empty canvas     | Marquee select, union with the existing set                                              |
| Drag a node in the set (set > 1) | **Group drag**: all selected nodes move by the same delta, preserving relative positions |
| Drag a node not in the set       | Selection collapses to that node (current behavior), single drag                         |
| Space held + drag                | Pan the canvas (anywhere, including over nodes); cursor becomes grab/grabbing            |
| Wheel zoom / touch pinch         | Unchanged                                                                                |
| `/view` read-only page           | Unchanged — empty-canvas drag still pans, no marquee                                     |

Space is only intercepted when focus is not in an input, Monaco, or CodeMirror.
Window blur clears the space state (prevents a stuck pan mode).

## Implementation

### Space-to-pan (capture-phase event gating)

`panZoom.ts` stays as-is. Instead of toggling `enablePan()/disablePan()` (which risks
breaking the Hammer touch path), we gate the mousedown event itself:

- A capture-phase `mousedown` listener on the container:
  - Space held → let the event through; svg-pan-zoom's built-in mouse pan handles it.
  - Space not held, empty-space target, `editable` → `stopPropagation()` (svg-pan-zoom
    never sees the mousedown, so no pan) and start the marquee.
  - Node target → pass through to the existing node select/drag logic
    (`beforePan: () => !isNodeDragging` already prevents pan-vs-node-drag races).
  - `!editable` (`/view`) → always let through; empty-drag keeps panning.

`onContainerMouseDown` returns early while space is held (pan never selects or arms a drag).

Cursor: container gains `cursor-grab` while space is down, `cursor-grabbing` while panning.

Space state lives in `View.svelte` via `svelte:window` keydown/keyup on `event.code === 'Space'`,
guarded against repeat events and against targets inside inputs/editors; window `blur` resets it.

### Multi-select state

New module `src/lib/util/selectedNodes.svelte.ts` (modeled on `selectedNode.svelte.ts`):

- `$state` set of node names + `setSelectedNodes` / `toggleSelectedNode` / `clearSelectedNodes`.
- Set size 1 → drive the existing single-selection module (Monaco cursor sync).
- Set size ≠ 1 → clear the single selection.
- Not persisted to URL/localStorage.

### Group drag

Reuses the per-node dx/dy offset mechanism in `nodePositions.svelte.ts`:

- mousedown on a set member (set > 1) snapshots every member's base offset + pointer origin.
- mousemove applies `delta / nodePositions.zoom` to every member:
  `setElementOffset` + `setNodePosition` per node.
- mouseup persists **once**: `persistPositions` changes from a single `changedName` to a
  list — the API payload is already the whole positions map, so one `updateDiagram` call.

### Marquee

- New `Marquee.svelte` overlay: absolutely-positioned rect inside `#view`, updated on mousemove.
- Intersection test runs against node rects snapshotted at marquee start
  (`getBoundingClientRect` per top-level `.node`/`.actor`).
- During the drag the intersecting nodes get a live highlight preview (visual only);
  the selection set itself is written once, on mouseup.
- Pure helpers (`rectsIntersect`, node-rect collection + name dedupe) live in a util module
  for unit testing.
- Sequence diagrams: same-name actor boxes (top/bottom) dedupe by node name — hitting
  either box selects the actor.

### Re-render restoration

The existing "restore single selection highlight after re-render" effect generalizes to
restore the highlight for the entire multi-select set.

## Data Flow

```
mousedown (capture, empty, editable, no space)
  → start marquee → mousemove updates rect + live intersection preview → mouseup finalizes set
mousedown (node)
  → not in set: collapse to single-select (current) / in set (set>1): arm group drag
  → mousemove: all members' offsets += delta/zoom → mouseup: one persist call
Space keydown/keyup (window)
  → maintain isSpacePressed → capture gate lets through / blocks + cursor switch
```

## Edge Cases

- Marquee displacement < 3px counts as a click → clear selection (current semantics).
- Space + click on a node → pan only; no selection.
- Space typed in the label-edit input / Monaco / CodeMirror → normal input, never hijacked.
- Unknown diagram types / no nodes → marquee selects nothing, no side effects.
- Persist failure → existing `notify` toast.
- Mobile: untouched — touch pan/pinch via Hammer still works; marquee/space are desktop-only.

## Testing

- **Unit (vitest, colocated):** rect-intersection helpers; multi-select store
  set/toggle/clear semantics; group-drag delta math (zoom conversion).
- **E2E (Playwright, `TID` constants):** marquee two nodes → group drag → both moved by the
  same delta; space+drag → pan state changed; Shift+click toggles membership.
- All existing unit/e2e tests stay green.
