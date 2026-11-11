# Single-Binary Production Deployment Design

Date: 2026-11-11
Status: Approved (pending implementation)

## Goal

Serve the compiled frontend (adapter-static build in `docs/`) and the storage
API from the single Go binary, replacing the current production setup that
runs `pnpm dev` (a Vite dev server) via systemd.

## Static File Serving (`server-go/static.go`)

- New env var `STATIC_DIR` (default: empty). Empty = API-only mode — the
  current behavior; the dev workflow (vite dev on :3000 proxying `/api` to
  the storage server) is completely unchanged.
- When set, the binary serves the directory on the same port as the API:
  - `/api/*` routes take priority (existing handlers, auth gating unchanged).
  - Other GET/HEAD requests: if the file exists under `STATIC_DIR`, serve it
    (`http.FileServer` semantics, correct Content-Type via extension).
  - If not, serve `STATIC_DIR/404.html` (the adapter-static SPA fallback)
    with status 200 so client-side routing takes over.
  - Directory listing disabled (unknown dir path falls through to the SPA
    fallback).
- Production listens on `HOST=0.0.0.0 PORT=3000` — one port serves both the
  app and the API; the frontend's relative `/api` paths are same-origin by
  construction, no proxy needed.

## systemd Layout

- `systemd/mermaid-frontend.service` changes from `pnpm dev` to:
  `ExecStart=/data/opensource/mermaid-live-editor/server-go/bin/mermaid-storage`
  with `Environment=HOST=0.0.0.0 PORT=3000 STATIC_DIR=docs`.
- **Hard constraint documented and enforced in the runbook: while the
  production unit runs, `mermaid-storage.service` (port 18790) must be
  stopped** — two Go processes writing the same SQLite file break the
  single-writer assumption. The dev mode is the inverse: vite dev + storage
  on 18790, production unit stopped.

## Build Script

- `package.json` gains `"build:prod": "pnpm build && make -C server-go build"`
  producing both artifacts (`docs/` and `server-go/bin/mermaid-storage`).
- Deploy = `pnpm build:prod && sudo systemctl restart mermaid-frontend`.

## Documentation

`README-LOCAL.md` gains a "生产部署(编译版)" section: the one-command
build, unit installation/switching between dev and production modes, the
two-writers hazard, and the update flow (change code → `pnpm build:prod` →
restart).

## Testing

- Go unit tests (httptest): file hit, SPA fallback (status 200, 404.html
  content), `/api` never intercepted by static, directory paths fall back,
  empty `STATIC_DIR` = API-only behavior unchanged.
- Smoke test: run the binary with `STATIC_DIR=docs` on a scratch port (3100),
  curl `/edit` returns the SPA HTML and `/api/auth/me` returns 401.
- All existing tests (Go 22, vitest 210, isolated auth e2e) run against the
  dev-mode stack and must stay green.

## Out of Scope

- `go:embed` of the frontend into the binary (keeps frontend/backend builds
  decoupled — rebuild either independently).
- HTTPS (LAN deployment is plain HTTP).
- nginx/caddy or any other reverse proxy.
