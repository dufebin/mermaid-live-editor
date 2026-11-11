# mermaid-server Rename + install.sh Design

Date: 2026-11-11
Status: Approved (user directive after two clarification rounds)

## Goal

Unify naming around "mermaid-server" (the one complete service), remove the
dev-mode storage unit, and add a one-command install/update script.

## Changes

1. **Binary rename**: `bin/mermaid-storage` → `bin/mermaid-server`
   (`server-go/Makefile` BIN, root `.gitignore` entry, assemble script log,
   README references).
2. **Production unit rename**: `systemd/mermaid-frontend.service` →
   `systemd/mermaid-server.service` (git mv); ExecStart points at
   `bin/mermaid-server`; Description updated to "mermaid-live-editor server
   (static + API, 0.0.0.0:3000)".
3. **Delete `systemd/mermaid-storage.service`** — dev mode no longer uses a
   unit. Dev runs the storage API ad-hoc:
   - terminal 1: `make -C server-go run` (repo-root cwd, defaults
     HOST=127.0.0.1 PORT=18790, no STATIC_DIR = API-only)
   - terminal 2: `pnpm dev` (:3000, /api proxied to 18790)
   - hard rule preserved: dev's 18790 instance and the production unit must
     never run at the same time (double-write SQLite); stop the unit first.
4. **`install.sh`** (repo root, run as the normal user, sudo only for
   systemd steps):
   1. `pnpm build:prod` (build is safe while production runs)
   2. disable+stop obsolete units (`mermaid-frontend`, `mermaid-storage`,
      `mermaid-live`), remove their `/etc` entries (file and symlinks)
   3. symlink `systemd/mermaid-server.service` into `/etc`, `daemon-reload`
   4. `enable --now mermaid-server` + `restart` (idempotent for fresh
      install and update)
   5. verify: `curl :3000/` 200, `curl :3000/api/auth/me` 401 JSON
   6. cleanup stale `server-go/bin/` if present
5. **README-LOCAL**: production section rewritten — install/update =
   `bash install.sh`; startup methods under the `mermaid-server` name; a new
   开发模式（调试） section with the two-terminal commands and the
   stop-production-first warning; all `mermaid-storage`/`mermaid-frontend`/
   unit-disable dance references removed.
6. **Cutover**: run `bash install.sh` live.

## Out of Scope

- Docker image internals (its own binary name and CMD stay as-is).
- The `/ai` production gap (separate follow-up).
