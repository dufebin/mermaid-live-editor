# Centralized bin/ Output Directory Design

Date: 2026-11-11
Status: Approved (user OK'd design incl. inline execution; startup methods to be added to README)

## Goal

Collect all compiled artifacts under `/data/opensource/mermaid-live-editor/bin/`
and point systemd at that directory, replacing the split layout
(`docs/` frontend + `server-go/bin/mermaid-storage` backend).

## Layout

```
bin/
  mermaid-storage   # Go binary (Makefile output moved here)
  static/           # frontend static files (assembled from docs/)
  static.prev/      # previous frontend, kept one generation for rollback
```

## Build flow (`pnpm build:prod`)

1. `pnpm build` → still writes `docs/` (upstream convention, Docker
   `mermaid` stage, and GitHub Pages workflow keep working unchanged; the
   live server keeps serving the old `bin/static`, eliminating the previous
   deploy-window white screen).
2. `make -C server-go build` → `go build -o ../bin/mermaid-storage` (rename
   replacement is safe over a running binary).
3. `node scripts/assemble-bin.mjs` → rotates `bin/static` → `bin/static.prev`,
   copies `docs/` → `bin/static`.

## Changes

- `server-go/Makefile`: build output `../bin/mermaid-storage`; `run` target
  executes from the repo root (cwd-sensitive DATA_DIR/STATIC_DIR defaults).
- Root `.gitignore`: add `/bin/`.
- `scripts/assemble-bin.mjs` (~15 lines, node:fs).
- `package.json`: `build:prod` = the three steps above.
- `systemd/mermaid-frontend.service`:
  `ExecStart=/data/opensource/mermaid-live-editor/bin/mermaid-storage`,
  `Environment=STATIC_DIR=bin/static` (WorkingDirectory unchanged).
- `README-LOCAL.md` 生产部署 section: new paths; remove the build-window
  white-screen warning (design eliminates it — note zero-downtime frontend,
  restart applies the new binary); document 启动方式 (systemd start/restart
  commands AND manual one-off run with env vars, with the stop-systemd-first
  warning against double-writing SQLite); note `bin/static.prev` and `docs/`
  are reclaimable space.

## Cutover

`pnpm build:prod` → unit update (symlink layout, daemon-reload) →
`systemctl restart mermaid-frontend` → verify :3000 SPA + 401 API →
remove stale `server-go/bin/`.

## Out of Scope

- Changing `pnpm build`'s output dir (upstream/Docker/Pages convention).
- Absolute env paths in the unit (relative + WorkingDirectory retained).
