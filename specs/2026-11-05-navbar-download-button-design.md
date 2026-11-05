# Download Button in Navbar — Design

Date: 2026-11-05
Status: Approved (approach A)

## Goal

Add a **Download** button next to the Share button in the editor navbar, offering one-click PNG / SVG download via a small popover menu. Remove the ~100 lines of download logic duplication by extracting it from `Actions.svelte` into a shared util.

## Approach

### 1. Shared util: `src/lib/util/download.ts`

Extract from `Actions.svelte` (behavior unchanged):

- `getFileName(extension)` — `mermaid-diagram-YYYY-MM-DD-HHmmss.<ext>`
- `simulateDownload(download, href)`
- `fixForeignObjectClipping(svg)` — rough-mode foreignObject height fix
- `getSvgElement()` — clone of `#container svg` with xlink namespace
- `getBase64SVG(svg?, width?, height?)` — serialized + base64 SVG string (reads `validatedState.current.rough`)
- `downloadSVG()` — data-URI download, plus `logEvent('download', { type: 'svg' })`
- `downloadPNG(options)` — the canvas export flow: disable panZoom → wait → build canvas (size mode `auto` default at 2x, or explicit width/height) → draw → download → re-enable panZoom, plus `logEvent('download', { type: 'png' })`

PNG size options are parameters (`{ sizeMode, size }`) so `Actions.svelte` keeps its UI state and passes it through; the navbar button uses the auto default.

Details preserved from current implementation:

- `svg` missing → throw `Error('svg not found')`; the navbar button catches and surfaces it via `notify()`.
- panZoom restore fallback timer (2 s) stays inside `downloadPNG`.
- rough-mode clipping fix and background fill stay inside the shared functions.

### 2. New component: `src/lib/components/DownloadButton.svelte`

- Popover-based menu (same pattern as `DropdownNavMenu.svelte`, using `$/components/ui/popover`).
- Trigger button matches Share's style: `buttonVariants({ size: 'sm' })`, download icon + "Download" label.
- Menu items: **PNG** → `downloadPNG()` (auto size), **SVG** → `downloadSVG()`.
- `data-testid` values added to `TID` in `src/lib/constants.ts`.

### 3. Wiring: `src/routes/(app)/edit/+page.svelte`

Place `<DownloadButton />` between `<Share />` and the Save button (navbar line ~138).

### 4. Refactor: `src/lib/components/Actions.svelte`

Replace local copies of the extracted functions with imports from `$/util/download`. PNG/SVG buttons, copy-image, and Markdown-copy behavior unchanged. Clipboard copy keeps its own canvas flow (it needs the blob, not a download) — it may reuse shared `getBase64SVG`/`getSvgElement`.

## Error handling

- SVG element not found: navbar button shows `notify()` message; Actions keeps current throw-on-missing behavior internally through the same shared function.
- No new user-facing strings beyond the notify message.

## Analytics

Reuse `logEvent('download', { type })` — no new event names.

## Testing

- No e2e (user preference from prior sessions — user verifies manually).
- No new unit tests for DOM-heavy export code; existing colocated tests unaffected.
- Verification: `pnpm check`, `pnpm lint`, `pnpm vitest run --dir src`.
