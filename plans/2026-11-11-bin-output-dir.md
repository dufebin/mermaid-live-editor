# Centralized bin/ Output Directory Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collect all compiled artifacts under the repo-root `bin/` directory and point systemd at it.

**Architecture:** `pnpm build` keeps writing `docs/` (upstream/Docker/Pages convention intact); `build:prod` then builds the Go binary into `bin/mermaid-storage` and assembles `bin/static/` from `docs/` via a rotation script (`static` → `static.prev`), which also eliminates the deploy-window white screen. The systemd unit starts the binary from `bin/` with `STATIC_DIR=bin/static`.

**Tech Stack:** node:fs, make, systemd.

**Spec:** `specs/2026-11-11-bin-output-dir-design.md`

## Global Constraints

- `pnpm build`'s output dir stays `docs/` — never change it (Docker mermaid stage + GitHub Pages workflow read it).
- `go build -o` rename-replacement over a running binary is safe; do not stop production for builds.
- Working tree has unrelated uncommitted user WIP — stage only named files; never repo-wide `pnpm lint:fix`.
- Every commit ends with `Co-Authored-By: Claude Code <noreply@anthropic.com>`.
- Do not run default/auth e2e against production.

---

### Task 1: Build pipeline + unit + runbook + cutover (single small task)

**Files:**

- Create: `scripts/assemble-bin.mjs`
- Modify: `server-go/Makefile`, `.gitignore`, `package.json`, `systemd/mermaid-frontend.service`, `README-LOCAL.md`
- Delete after cutover: `server-go/bin/` (stale binary location)

- [ ] **Step 1: Makefile** — `server-go/Makefile`:

```make
BIN := ../bin/mermaid-storage

.PHONY: build run test vet

build:
	go build -o $(BIN) .

# Run from the repo root so relative DATA_DIR/STATIC_DIR resolve like systemd.
run: build
	cd .. && ./bin/mermaid-storage

test:
	go test ./... -count=1

vet:
	go vet ./...
```

- [ ] **Step 2: Root `.gitignore`** — add a `/bin/` line next to the existing `/docs` entry.

- [ ] **Step 3: `scripts/assemble-bin.mjs`**:

```js
import { cpSync, existsSync, mkdirSync, renameSync, rmSync } from 'node:fs';

const ROOT = new URL('..', import.meta.url).pathname;
const bin = `${ROOT}bin`;

mkdirSync(bin, { recursive: true });
rmSync(`${bin}/static.prev`, { recursive: true, force: true });
if (existsSync(`${bin}/static`)) {
  renameSync(`${bin}/static`, `${bin}/static.prev`);
}
cpSync(`${ROOT}docs`, `${bin}/static`, { recursive: true });
console.log('bin/ assembled: mermaid-storage + static/ (previous generation at static.prev)');
```

- [ ] **Step 4: `package.json`** — `"build:prod": "pnpm build && make -C server-go build && node scripts/assemble-bin.mjs"`.

- [ ] **Step 5: `systemd/mermaid-frontend.service`** — ExecStart → `/data/opensource/mermaid-live-editor/bin/mermaid-storage`; `Environment=STATIC_DIR=bin/static` (WorkingDirectory and other lines unchanged).

- [ ] **Step 6: README-LOCAL 生产部署 section** — update to:
  - 产物位置:`bin/mermaid-storage` + `bin/static/`
  - **启动方式**(user-requested):systemd(`sudo systemctl restart mermaid-frontend` 日常;`start`/`status`/`is-active`)与手动一次性运行(`cd /data/opensource/mermaid-live-editor && HOST=0.0.0.0 PORT=3000 STATIC_DIR=bin/static DATA_DIR=server-data ./bin/mermaid-storage`,附"必须先停 systemd,否则双写 SQLite"警告)
  - 更新流程改为零停机描述:构建写 `docs/`、线上仍服务旧 `bin/static`,组装换新后 restart 生效;**删除原"构建期间白屏"警告**
  - `bin/static.prev` 与 `docs/` 可随时删除回收空间(回滚 = 把 static.prev 换回 static)

- [ ] **Step 7: Build + smoke** — `pnpm build:prod`;确认 `bin/mermaid-storage` + `bin/static/`(含 index.html、404.html、_app/);scratch 端口验证:`DATA_DIR=$(mktemp -d) PORT=3101 STATIC_DIR=bin/static ./bin/mermaid-storage &` → curl `/edit`(SPA)、`/api/auth/me`(401)、`/_app/` 资源(带 immutable Cache-Control)→ kill。

- [ ] **Step 8: Cutover** — `sudo systemctl daemon-reload && sudo systemctl restart mermaid-frontend`;验证 `systemctl is-active` + curl :3000 SPA/401;`rm -rf server-go/bin`(旧位置)。

- [ ] **Step 9: Verify + commit** — `make -C server-go test`;`pnpm exec prettier --check README-LOCAL.md scripts/assemble-bin.mjs`;stage: `scripts/assemble-bin.mjs server-go/Makefile .gitignore package.json systemd/mermaid-frontend.service README-LOCAL.md`;commit `feat: centralize compiled artifacts in bin/ with zero-downtime assembly`。

## Self-Review Notes

- Spec coverage: layout/build flow (Steps 1-4), unit (Step 5), README incl. startup methods + white-screen warning removal (Step 6), cutover + stale cleanup (Step 8). Out-of-scope honored (docs/ output dir untouched).
- Type consistency: `BIN := ../bin/mermaid-storage` matches unit ExecStart path and assemble script's `bin/` target; `STATIC_DIR=bin/static` matches assemble output.
