# 图表存储 + Dashboard 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为本地 mermaid-live-editor 提供图表保存与管理：零依赖 Node SQLite 存储服务、`/dashboard` 管理页（文件夹/缩略图/搜索/收藏/导入导出）、编辑器本地 Save 按钮，并经配置隐藏商业引流 UI。

**Architecture:** 仓库内 `server/` 目录跑一个 `node:http` + `node:sqlite` 零依赖 REST 服务（:18781，WAL，`server-data/diagrams.db`）；前端经 vite `/api` 代理访问（同 `/ai` 模式）。图表内容存与 URL hash 相同的序列化 State（`validatedState.current.serialized`），加载走 `replaceInputState(deserializeState(...))` 复用全部既有管线。`currentDiagramId` 为内存态模块，`history.replaceState` 不触发 hashchange，故仅真实 URL 变化（layout 的 hashchange 监听）会清空它。

**Tech Stack:** Node 24（node:sqlite、node:http、node:crypto）、SvelteKit 2 + Svelte 5 runes、vitest（jsdom）、Playwright（E2E）。规格：`specs/2026-11-03-diagram-storage-dashboard-design.md`。

## Global Constraints

- Node ≥ 24.16.0（node:sqlite 需要它）、pnpm 10.34.9
- `server/` 不引入任何 npm 依赖、不用 TypeScript，纯 ESM `.mjs`
- `docs/` 是构建输出目录，绝不编辑；文档放 `specs/`
- 密钥/本地配置只进 `.env.local`（gitignored）；`server-data/` 必须 gitignore
- ESLint 强制 `src/` 下 5+ 键对象按字母序排 key；本功能不新增客户端 env——api 走相对路径 `/api`，无需配置
- 时间戳 ISO 8601 UTC；id 用 `randomUUID()`
- 改 `vite.config.js`/`.env.local` 后需重启 dev server；dev server 由 controller 管理，子代理不得 kill
- 提交信息以 `Co-Authored-By: Claude Code <noreply@anthropic.com>` 结尾
- 参考 API 风格：`src/lib/util/ai.ts`（fetch 封装）、`src/lib/components/Share.svelte`（Dialog 用法）

---

### Task 1: SQLite 存储服务

**Files:**

- Create: `server/db.mjs`
- Create: `server/index.mjs`
- Modify: `.gitignore`（追加 `server-data/`）

**Interfaces:**

- Consumes: 无
- Produces: REST API（全部返回 camelCase JSON）；后续任务依赖的端点签名：
  - `GET /api/folders` → `Folder[]`（`{id, name, parentId, createdAt, updatedAt}`，parentId 可 null）
  - `POST /api/folders {name, parentId?}` → `Folder`（201）
  - `PATCH /api/folders/:id {name?, parentId?}` → `Folder`（parentId=null 表示移到根级；环 → 400）
  - `DELETE /api/folders/:id` → `{ok:true}`（子文件夹级联删，图表 SET NULL 回未分类）
  - `GET /api/diagrams?folderId=&search=&favorite=1` → `Diagram[]`（`{id, folderId, name, state, favorite, createdAt, updatedAt}`，favorite 为 boolean，按 updated_at 倒序；folderId=root 表示未分类）
  - `POST /api/diagrams {name, folderId?, state}` → `Diagram`（201）
  - `GET/PATCH/DELETE /api/diagrams/:id`（PATCH 接受 `{name?, folderId?, state?, favorite?}` 任意子集）
  - `GET /api/export` → `{folders, diagrams, exportedAt}`
  - `POST /api/import {folders, diagrams}` → `{imported, skipped}`（按 id INSERT OR IGNORE 去重，不覆盖）

- [ ] **Step 1: 创建 `server/db.mjs`**

```js
import { randomUUID } from 'node:crypto';
import { mkdirSync } from 'node:fs';
import { DatabaseSync } from 'node:sqlite';
import path from 'node:path';

const DATA_DIR = process.env.DATA_DIR ?? 'server-data';
const now = () => new Date().toISOString();

mkdirSync(DATA_DIR, { recursive: true });

export const db = new DatabaseSync(path.join(DATA_DIR, 'diagrams.db'));

db.exec(`
  PRAGMA journal_mode = WAL;
  PRAGMA foreign_keys = ON;
  CREATE TABLE IF NOT EXISTS folders (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    parent_id TEXT REFERENCES folders(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
  );
  CREATE TABLE IF NOT EXISTS diagrams (
    id TEXT PRIMARY KEY,
    folder_id TEXT REFERENCES folders(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    state TEXT NOT NULL,
    favorite INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
  );
  CREATE INDEX IF NOT EXISTS idx_diagrams_folder ON diagrams(folder_id);
  CREATE INDEX IF NOT EXISTS idx_diagrams_favorite ON diagrams(favorite);
`);

export const listFolders = () => db.prepare('SELECT * FROM folders ORDER BY name').all();

export const listAllDiagrams = () => db.prepare('SELECT * FROM diagrams').all();

export const getFolder = (id) => db.prepare('SELECT * FROM folders WHERE id = ?').get(id) ?? null;

// True when ancestorId is id itself or one of id's ancestors (cycle guard for moves).
export const isSelfOrAncestor = (id, ancestorId) => {
  let current = getFolder(ancestorId);
  while (current) {
    if (current.id === id) {
      return true;
    }
    current = current.parent_id ? getFolder(current.parent_id) : null;
  }
  return false;
};

export const createFolder = ({ name, parentId = null }) => {
  const id = randomUUID();
  const ts = now();
  db.prepare(
    'INSERT INTO folders (id, name, parent_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)'
  ).run(id, name, parentId, ts, ts);
  return getFolder(id);
};

export const updateFolder = (id, { name, parentId }) => {
  const folder = getFolder(id);
  if (!folder) {
    return null;
  }
  db.prepare('UPDATE folders SET name = ?, parent_id = ?, updated_at = ? WHERE id = ?').run(
    name ?? folder.name,
    parentId === undefined ? folder.parent_id : parentId,
    now(),
    id
  );
  return getFolder(id);
};

// Child folders cascade via FK; diagrams in any deleted folder fall back to
// uncategorized via ON DELETE SET NULL.
export const deleteFolder = (id) =>
  db.prepare('DELETE FROM folders WHERE id = ?').run(id).changes > 0;

export const listDiagrams = ({ favorite, folderId, search } = {}) => {
  const where = [];
  const params = [];
  if (folderId === 'root') {
    where.push('folder_id IS NULL');
  } else if (folderId) {
    where.push('folder_id = ?');
    params.push(folderId);
  }
  if (search) {
    where.push('name LIKE ?');
    params.push(`%${search}%`);
  }
  if (favorite) {
    where.push('favorite = 1');
  }
  const sql = `SELECT * FROM diagrams ${
    where.length > 0 ? `WHERE ${where.join(' AND ')}` : ''
  } ORDER BY updated_at DESC`;
  return db.prepare(sql).all(...params);
};

export const getDiagram = (id) => db.prepare('SELECT * FROM diagrams WHERE id = ?').get(id) ?? null;

export const createDiagram = ({ folderId = null, name, state }) => {
  const id = randomUUID();
  const ts = now();
  db.prepare(
    'INSERT INTO diagrams (id, folder_id, name, state, favorite, created_at, updated_at) VALUES (?, ?, ?, ?, 0, ?, ?)'
  ).run(id, folderId, name, state, ts, ts);
  return getDiagram(id);
};

export const updateDiagram = (id, { favorite, folderId, name, state }) => {
  const diagram = getDiagram(id);
  if (!diagram) {
    return null;
  }
  db.prepare(
    'UPDATE diagrams SET name = ?, folder_id = ?, state = ?, favorite = ?, updated_at = ? WHERE id = ?'
  ).run(
    name ?? diagram.name,
    folderId === undefined ? diagram.folder_id : folderId,
    state ?? diagram.state,
    favorite === undefined ? diagram.favorite : favorite,
    now(),
    id
  );
  return getDiagram(id);
};

export const deleteDiagram = (id) =>
  db.prepare('DELETE FROM diagrams WHERE id = ?').run(id).changes > 0;

export const importAll = (payload) => {
  let imported = 0;
  let skipped = 0;
  const insertFolder = db.prepare(
    'INSERT OR IGNORE INTO folders (id, name, parent_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)'
  );
  for (const folder of payload.folders ?? []) {
    const result = insertFolder.run(
      folder.id,
      folder.name,
      folder.parentId ?? null,
      folder.createdAt ?? now(),
      folder.updatedAt ?? now()
    );
    if (result.changes > 0) {
      imported++;
    } else {
      skipped++;
    }
  }
  const insertDiagram = db.prepare(
    'INSERT OR IGNORE INTO diagrams (id, folder_id, name, state, favorite, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)'
  );
  for (const diagram of payload.diagrams ?? []) {
    const result = insertDiagram.run(
      diagram.id,
      diagram.folderId ?? null,
      diagram.name,
      diagram.state,
      diagram.favorite ? 1 : 0,
      diagram.createdAt ?? now(),
      diagram.updatedAt ?? now()
    );
    if (result.changes > 0) {
      imported++;
    } else {
      skipped++;
    }
  }
  return { imported, skipped };
};
```

- [ ] **Step 2: 创建 `server/index.mjs`**

```js
import { createServer } from 'node:http';
import {
  createDiagram,
  createFolder,
  deleteDiagram,
  deleteFolder,
  getDiagram,
  getFolder,
  importAll,
  isSelfOrAncestor,
  listAllDiagrams,
  listDiagrams,
  listFolders,
  updateDiagram,
  updateFolder
} from './db.mjs';

const HOST = process.env.HOST ?? '127.0.0.1';
const PORT = Number(process.env.PORT ?? 18781);

const diagramJson = (row) =>
  row && {
    createdAt: row.created_at,
    favorite: Boolean(row.favorite),
    folderId: row.folder_id,
    id: row.id,
    name: row.name,
    state: row.state,
    updatedAt: row.updated_at
  };

const folderJson = (row) =>
  row && {
    createdAt: row.created_at,
    id: row.id,
    name: row.name,
    parentId: row.parent_id,
    updatedAt: row.updated_at
  };

const json = (res, status, body) => {
  const data = JSON.stringify(body);
  res.writeHead(status, {
    'Content-Length': Buffer.byteLength(data),
    'Content-Type': 'application/json; charset=utf-8'
  });
  res.end(data);
};

const bad = (res, status, error) => json(res, status, { error });

const readBody = (req) =>
  new Promise((resolve, reject) => {
    let raw = '';
    req.on('data', (chunk) => {
      raw += chunk;
      if (raw.length > 5_000_000) {
        reject(new Error('Payload too large'));
        req.destroy();
      }
    });
    req.on('end', () => {
      if (!raw) {
        resolve({});
        return;
      }
      try {
        resolve(JSON.parse(raw));
      } catch {
        reject(new Error('Invalid JSON body'));
      }
    });
    req.on('error', reject);
  });

const isName = (value) => typeof value === 'string' && value.trim().length > 0;
const isState = (value) => typeof value === 'string' && value.trim().length > 0;

const routes = [
  {
    handler: (req, res) => json(res, 200, listFolders().map(folderJson)),
    method: 'GET',
    pattern: /^\/api\/folders$/
  },
  {
    handler: async (req, res) => {
      const body = await readBody(req);
      if (!isName(body.name)) {
        return bad(res, 400, 'name is required');
      }
      if (body.parentId != null && !getFolder(body.parentId)) {
        return bad(res, 404, 'parent folder not found');
      }
      return json(
        res,
        201,
        folderJson(createFolder({ name: body.name.trim(), parentId: body.parentId ?? null }))
      );
    },
    method: 'POST',
    pattern: /^\/api\/folders$/
  },
  {
    handler: async (req, res, match) => {
      const id = match[1];
      if (!getFolder(id)) {
        return bad(res, 404, 'folder not found');
      }
      const body = await readBody(req);
      if (body.name != null && !isName(body.name)) {
        return bad(res, 400, 'name must be a non-empty string');
      }
      if (body.parentId != null) {
        if (body.parentId === id || isSelfOrAncestor(id, body.parentId)) {
          return bad(res, 400, 'cannot move a folder into itself or its descendant');
        }
        if (!getFolder(body.parentId)) {
          return bad(res, 404, 'parent folder not found');
        }
      }
      return json(
        res,
        200,
        folderJson(
          updateFolder(id, {
            name: body.name != null ? body.name.trim() : undefined,
            parentId: body.parentId
          })
        )
      );
    },
    method: 'PATCH',
    pattern: /^\/api\/folders\/([^/]+)$/
  },
  {
    handler: (req, res, match) =>
      deleteFolder(match[1]) ? json(res, 200, { ok: true }) : bad(res, 404, 'folder not found'),
    method: 'DELETE',
    pattern: /^\/api\/folders\/([^/]+)$/
  },
  {
    handler: (req, res, url) => {
      const query = url.searchParams;
      const diagrams = listDiagrams({
        favorite: query.get('favorite') === '1',
        folderId: query.get('folderId') ?? undefined,
        search: query.get('search') ?? undefined
      });
      return json(res, 200, diagrams.map(diagramJson));
    },
    method: 'GET',
    pattern: /^\/api\/diagrams$/
  },
  {
    handler: async (req, res) => {
      const body = await readBody(req);
      if (!isName(body.name)) {
        return bad(res, 400, 'name is required');
      }
      if (!isState(body.state)) {
        return bad(res, 400, 'state is required');
      }
      if (body.folderId != null && !getFolder(body.folderId)) {
        return bad(res, 404, 'folder not found');
      }
      return json(
        res,
        201,
        diagramJson(
          createDiagram({
            folderId: body.folderId ?? null,
            name: body.name.trim(),
            state: body.state
          })
        )
      );
    },
    method: 'POST',
    pattern: /^\/api\/diagrams$/
  },
  {
    handler: (req, res, match) => {
      const diagram = getDiagram(match[1]);
      return diagram ? json(res, 200, diagramJson(diagram)) : bad(res, 404, 'diagram not found');
    },
    method: 'GET',
    pattern: /^\/api\/diagrams\/([^/]+)$/
  },
  {
    handler: async (req, res, match) => {
      const id = match[1];
      if (!getDiagram(id)) {
        return bad(res, 404, 'diagram not found');
      }
      const body = await readBody(req);
      if (body.name != null && !isName(body.name)) {
        return bad(res, 400, 'name must be a non-empty string');
      }
      if (body.state != null && !isState(body.state)) {
        return bad(res, 400, 'state must be a non-empty string');
      }
      if (body.folderId != null && !getFolder(body.folderId)) {
        return bad(res, 404, 'folder not found');
      }
      return json(
        res,
        200,
        diagramJson(
          updateDiagram(id, {
            favorite: typeof body.favorite === 'boolean' ? Number(body.favorite) : undefined,
            folderId: body.folderId,
            name: body.name != null ? body.name.trim() : undefined,
            state: body.state
          })
        )
      );
    },
    method: 'PATCH',
    pattern: /^\/api\/diagrams\/([^/]+)$/
  },
  {
    handler: (req, res, match) =>
      deleteDiagram(match[1]) ? json(res, 200, { ok: true }) : bad(res, 404, 'diagram not found'),
    method: 'DELETE',
    pattern: /^\/api\/diagrams\/([^/]+)$/
  },
  {
    handler: (req, res) =>
      json(res, 200, {
        diagrams: listAllDiagrams().map(diagramJson),
        exportedAt: new Date().toISOString(),
        folders: listFolders().map(folderJson)
      }),
    method: 'GET',
    pattern: /^\/api\/export$/
  },
  {
    handler: async (req, res) => {
      const body = await readBody(req);
      if (!Array.isArray(body.folders) || !Array.isArray(body.diagrams)) {
        return bad(res, 400, 'folders and diagrams arrays are required');
      }
      return json(res, 200, importAll(body));
    },
    method: 'POST',
    pattern: /^\/api\/import$/
  }
];

const server = createServer(async (req, res) => {
  try {
    const url = new URL(req.url ?? '/', `http://${req.headers.host ?? 'localhost'}`);
    for (const route of routes) {
      if (req.method !== route.method) {
        continue;
      }
      const match = url.pathname.match(route.pattern);
      if (!match) {
        continue;
      }
      await route.handler(req, res, url, match);
      return;
    }
    bad(res, 404, 'Not found');
  } catch (error) {
    bad(res, 400, error instanceof Error ? error.message : 'Bad request');
  }
});

server.listen(PORT, HOST, () => {
  console.log(`storage API listening on http://${HOST}:${PORT}`);
});
```

- [ ] **Step 3: `.gitignore` 追加一行 `server-data/`**

- [ ] **Step 4: 冒烟验证（curl 全流程）**

后台启动 `DATA_DIR=$(mktemp -d) node server/index.mjs`（避免污染真实数据目录），然后按序执行并核对：

```bash
B=http://127.0.0.1:18781
# 1. 建根文件夹 + 子文件夹
F1=$(curl -s -X POST $B/api/folders -H 'Content-Type: application/json' -d '{"name":"Work"}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')
F2=$(curl -s -X POST $B/api/folders -H 'Content-Type: application/json' -d "{\"name\":\"Sub\",\"parentId\":\"$F1\"}" | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')
# 2. 存图（挂到 F2）
D1=$(curl -s -X POST $B/api/diagrams -H 'Content-Type: application/json' -d "{\"name\":\"My flow\",\"folderId\":\"$F2\",\"state\":\"pako:eJyrVspLzE1VslLQUcpLzs+NLs4vyklRBABbHQZO\"}" | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')
# 3. 列表/过滤
curl -s "$B/api/diagrams" ; curl -s "$B/api/diagrams?search=flow" ; curl -s "$B/api/diagrams?folderId=$F2"
# 4. 收藏 + 过滤
curl -s -X PATCH $B/api/diagrams/$D1 -H 'Content-Type: application/json' -d '{"favorite":true}' ; curl -s "$B/api/diagrams?favorite=1"
# 5. 环检测：把 F1 移到 F2 下 → 400
curl -s -X PATCH $B/api/folders/$F1 -H 'Content-Type: application/json' -d "{\"parentId\":\"$F2\"}"
# 6. 删除 F1（级联 F2，D1 应回未分类）
curl -s -X DELETE $B/api/folders/$F1 ; curl -s "$B/api/diagrams?folderId=root"
# 7. 导出 → 导入（去重 skipped）
curl -s $B/api/export > /tmp/export.json ; curl -s -X POST $B/api/import -H 'Content-Type: application/json' -d @/tmp/export.json
# 8. 校验错误：空名 400、不存在 404
curl -s -X POST $B/api/diagrams -H 'Content-Type: application/json' -d '{"name":"","state":"x"}' ; curl -s $B/api/diagrams/nope
```

Expected：步骤 5 返回 400 环错误；步骤 6 后 D1 出现在 `folderId=root` 结果中且 `folderId` 为 null；步骤 7 返回 `{"imported":0,"skipped":N}`（N=导出条数）；步骤 8 均为错误 JSON。全部符合后 kill 服务进程。

- [ ] **Step 5: 提交**

```bash
git add server/ .gitignore
git commit -m "feat: zero-dependency SQLite storage service for diagrams

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 2: Vite /api 代理与启动脚本

**Files:**

- Modify: `vite.config.js`
- Modify: `package.json`（scripts）
- Create: `scripts/dev-all.mjs`
- （controller 完成：`.env.local` 追加 `MERMAID_IS_ENABLED_MERMAID_CHART_LINKS='false'`）

**Interfaces:**

- Consumes: Task 1 的服务（默认 `http://127.0.0.1:18781`）
- Produces: dev/preview 下同源 `/api/*` 可达；`pnpm server`（单跑存储服务）、`pnpm dev:all`（并行服务+前端）

- [ ] **Step 1: vite.config.js 增加 apiProxy**

在 `aiProxy` 定义之后加：

```js
const apiProxy = {
  '/api': {
    changeOrigin: true,
    target: serverEnv.API_PROXY_TARGET ?? 'http://127.0.0.1:18781'
  }
};
```

`server` 与 `preview` 的 proxy 改为合并：

```js
    server: { host: true, port: 3000, proxy: { ...(aiProxy ?? {}), ...apiProxy } },
    preview: { host: true, port: 3000, proxy: { ...(aiProxy ?? {}), ...apiProxy } },
```

- [ ] **Step 2: package.json scripts 增加**

```json
    "dev:all": "node scripts/dev-all.mjs",
    "server": "node server/index.mjs",
```

- [ ] **Step 3: 创建 `scripts/dev-all.mjs`**

```js
import { spawn } from 'node:child_process';

const children = [
  spawn('node', ['server/index.mjs'], { stdio: 'inherit' }),
  spawn('pnpm', ['dev'], { stdio: 'inherit' })
];

const shutdown = () => {
  for (const child of children) {
    child.kill();
  }
};

process.on('SIGINT', () => {
  shutdown();
  process.exit(0);
});
process.on('SIGTERM', () => {
  shutdown();
  process.exit(0);
});
```

- [ ] **Step 4: 验证（controller 重启 dev server 后）**

```bash
curl -s http://127.0.0.1:18781/api/folders   # 直接访问服务
curl -s http://localhost:3000/api/folders    # 经 vite 代理
```

Expected：两者都返回 `[]`（或既有文件夹数组）。

- [ ] **Step 5: 提交**

```bash
git add vite.config.js package.json scripts/dev-all.mjs
git commit -m "feat: proxy /api to storage service, add server scripts

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 3: API 客户端与文件夹树工具（TDD）

**Files:**

- Create: `src/lib/util/api.ts`
- Test: `src/lib/util/api.test.ts`
- Create: `src/lib/util/folderTree.ts`
- Test: `src/lib/util/folderTree.test.ts`

**Interfaces:**

- Consumes: Task 1 的 REST API（经 `/api` 相对路径）
- Produces（Task 4/5/6 依赖，签名精确）:
  - `interface Folder { createdAt: string; id: string; name: string; parentId: string | null; updatedAt: string }`
  - `interface Diagram { createdAt: string; favorite: boolean; folderId: string | null; id: string; name: string; state: string; updatedAt: string }`
  - `listFolders(): Promise<Folder[]>`；`createFolder(name: string, parentId?: string | null): Promise<Folder>`；`updateFolder(id: string, patch: { name?: string; parentId?: string | null }): Promise<Folder>`；`deleteFolder(id: string): Promise<{ ok: boolean }>`
  - `listDiagrams(query?: { favorite?: boolean; folderId?: string; search?: string }): Promise<Diagram[]>`；`createDiagram(data: { folderId?: string | null; name: string; state: string }): Promise<Diagram>`；`getDiagram(id: string): Promise<Diagram>`；`updateDiagram(id: string, patch: { favorite?: boolean; folderId?: string | null; name?: string; state?: string }): Promise<Diagram>`；`deleteDiagram(id: string): Promise<{ ok: boolean }>`
  - `exportAll(): Promise<{ diagrams: Diagram[]; exportedAt: string; folders: Folder[] }>`；`importAll(data: { diagrams: unknown[]; folders: unknown[] }): Promise<{ imported: number; skipped: number }>`
  - `buildFolderTree(folders: Folder[]): FolderNode[]`（`interface FolderNode { children: FolderNode[]; folder: Folder }`；孤儿与成环节点按根处理）

- [ ] **Step 1: 写失败测试 `src/lib/util/api.test.ts`**

```ts
import { afterEach, describe, expect, it, vi } from 'vitest';
import { listFolders, listDiagrams, updateDiagram } from './api';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('api client', () => {
  it('parses successful responses as JSON', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response(JSON.stringify([{ id: 'f1', name: 'Work' }]), { status: 200 })
        )
    );
    const folders = await listFolders();
    expect(folders).toEqual([{ id: 'f1', name: 'Work' }]);
  });

  it('sends method, body and query parameters', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response('[]', { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    await listDiagrams({ favorite: true, folderId: 'root', search: 'flow' });
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe('/api/diagrams?folderId=root&search=flow&favorite=1');
    expect(init.method).toBeUndefined();
    await updateDiagram('d1', { name: 'New' });
    const [url2, init2] = fetchMock.mock.calls[1] as [string, RequestInit];
    expect(url2).toBe('/api/diagrams/d1');
    expect(init2.method).toBe('PATCH');
    expect(init2.body).toBe(JSON.stringify({ name: 'New' }));
  });

  it('throws with server error message on non-2xx', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response(JSON.stringify({ error: 'folder not found' }), { status: 404 })
        )
    );
    await expect(updateDiagram('nope', { name: 'x' })).rejects.toThrow('folder not found');
  });

  it('throws with status when error body is not JSON', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('<html>', { status: 502 })));
    await expect(updateDiagram('d1', { name: 'x' })).rejects.toThrow('API request failed (502)');
  });
});
```

- [ ] **Step 2: 运行确认失败**

Run: `pnpm vitest run src/lib/util/api.test.ts`
Expected: FAIL（`Failed to resolve import "./api"`）

- [ ] **Step 3: 实现 `src/lib/util/api.ts`**

```ts
export interface Folder {
  createdAt: string;
  id: string;
  name: string;
  parentId: string | null;
  updatedAt: string;
}

export interface Diagram {
  createdAt: string;
  favorite: boolean;
  folderId: string | null;
  id: string;
  name: string;
  state: string;
  updatedAt: string;
}

export interface ExportData {
  diagrams: Diagram[];
  exportedAt: string;
  folders: Folder[];
}

const request = async <T>(path: string, init?: RequestInit): Promise<T> => {
  const response = await fetch(`/api${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init
  });
  if (!response.ok) {
    const body = (await response.json().catch(() => undefined)) as { error?: string } | undefined;
    throw new Error(body?.error ?? `API request failed (${response.status})`);
  }
  return (await response.json()) as T;
};

export const listFolders = (): Promise<Folder[]> => request<Folder[]>('/folders');

export const createFolder = (name: string, parentId: string | null = null): Promise<Folder> =>
  request<Folder>('/folders', { body: JSON.stringify({ name, parentId }), method: 'POST' });

export const updateFolder = (
  id: string,
  patch: { name?: string; parentId?: string | null }
): Promise<Folder> =>
  request<Folder>(`/folders/${id}`, { body: JSON.stringify(patch), method: 'PATCH' });

export const deleteFolder = (id: string): Promise<{ ok: boolean }> =>
  request<{ ok: boolean }>(`/folders/${id}`, { method: 'DELETE' });

export const listDiagrams = (
  query: { favorite?: boolean; folderId?: string; search?: string } = {}
): Promise<Diagram[]> => {
  const params = new URLSearchParams();
  if (query.folderId) {
    params.set('folderId', query.folderId);
  }
  if (query.search) {
    params.set('search', query.search);
  }
  if (query.favorite) {
    params.set('favorite', '1');
  }
  const qs = params.toString();
  return request<Diagram[]>(`/diagrams${qs ? `?${qs}` : ''}`);
};

export const createDiagram = (data: {
  folderId?: string | null;
  name: string;
  state: string;
}): Promise<Diagram> =>
  request<Diagram>('/diagrams', { body: JSON.stringify(data), method: 'POST' });

export const getDiagram = (id: string): Promise<Diagram> => request<Diagram>(`/diagrams/${id}`);

export const updateDiagram = (
  id: string,
  patch: { favorite?: boolean; folderId?: string | null; name?: string; state?: string }
): Promise<Diagram> =>
  request<Diagram>(`/diagrams/${id}`, { body: JSON.stringify(patch), method: 'PATCH' });

export const deleteDiagram = (id: string): Promise<{ ok: boolean }> =>
  request<{ ok: boolean }>(`/diagrams/${id}`, { method: 'DELETE' });

export const exportAll = (): Promise<ExportData> => request<ExportData>('/export');

export const importAll = (data: {
  diagrams: unknown[];
  folders: unknown[];
}): Promise<{ imported: number; skipped: number }> =>
  request<{ imported: number; skipped: number }>('/import', {
    body: JSON.stringify(data),
    method: 'POST'
  });
```

- [ ] **Step 4: 运行 api 测试确认通过**

Run: `pnpm vitest run src/lib/util/api.test.ts`
Expected: PASS（4 用例）

- [ ] **Step 5: 写失败测试 `src/lib/util/folderTree.test.ts`**

```ts
import { describe, expect, it } from 'vitest';
import { buildFolderTree } from './folderTree';
import type { Folder } from './api';

const folder = (id: string, name: string, parentId: string | null = null): Folder => ({
  createdAt: '2026-11-03T00:00:00.000Z',
  id,
  name,
  parentId,
  updatedAt: '2026-11-03T00:00:00.000Z'
});

describe('buildFolderTree', () => {
  it('nests children under parents', () => {
    const tree = buildFolderTree([folder('a', 'A'), folder('b', 'B', 'a'), folder('c', 'C', 'b')]);
    expect(tree).toHaveLength(1);
    expect(tree[0]?.folder.id).toBe('a');
    expect(tree[0]?.children[0]?.folder.id).toBe('b');
    expect(tree[0]?.children[0]?.children[0]?.folder.id).toBe('c');
  });

  it('treats orphans (missing parent) as roots', () => {
    const tree = buildFolderTree([folder('a', 'A'), folder('b', 'B', 'missing')]);
    expect(tree).toHaveLength(2);
  });

  it('breaks cycles safely instead of recursing forever', () => {
    const tree = buildFolderTree([folder('a', 'A', 'b'), folder('b', 'B', 'a')]);
    expect(tree).toHaveLength(2);
  });
});
```

- [ ] **Step 6: 运行确认失败**

Run: `pnpm vitest run src/lib/util/folderTree.test.ts`
Expected: FAIL（模块不存在）

- [ ] **Step 7: 实现 `src/lib/util/folderTree.ts`**

```ts
import type { Folder } from './api';

export interface FolderNode {
  children: FolderNode[];
  folder: Folder;
}

export const buildFolderTree = (folders: Folder[]): FolderNode[] => {
  const nodes = new Map<string, FolderNode>();
  for (const f of folders) {
    nodes.set(f.id, { children: [], folder: f });
  }
  const roots: FolderNode[] = [];
  for (const node of nodes.values()) {
    const parent = node.folder.parentId ? nodes.get(node.folder.parentId) : undefined;
    if (parent && parent !== node) {
      parent.children.push(node);
    } else {
      roots.push(node);
    }
  }
  return roots;
};
```

- [ ] **Step 8: 运行全部新测试确认通过**

Run: `pnpm vitest run src/lib/util/api.test.ts src/lib/util/folderTree.test.ts`
Expected: PASS（7 用例）

- [ ] **Step 9: 提交**

```bash
git add src/lib/util/api.ts src/lib/util/api.test.ts src/lib/util/folderTree.ts src/lib/util/folderTree.test.ts
git commit -m "feat: typed API client and folder tree utils

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 4: currentDiagram 模块与编辑器 Save

**Files:**

- Create: `src/lib/util/currentDiagram.svelte.ts`
- Create: `src/lib/components/Dashboard/SaveDialog.svelte`
- Modify: `src/routes/(app)/edit/+page.svelte`（Navbar children 加 Save 按钮 + SaveDialog）
- Modify: `src/routes/(app)/+layout.svelte`（hashchange 清空 currentDiagram）
- Modify: `src/lib/constants.ts`（TID 增加 `saveButton`、`saveDialogInput`，保持字母序）

**Interfaces:**

- Consumes: Task 3 的 `createDiagram`/`updateDiagram`/`Diagram`；既有 `validatedState.current.serialized`（string）、`notify`
- Produces（Task 6 依赖）:
  - `currentDiagram`：`{ get diagram(): Diagram | null }`
  - `setCurrentDiagram(diagram: Diagram): void`；`clearCurrentDiagram(): void`

- [ ] **Step 1: 创建 `src/lib/util/currentDiagram.svelte.ts`**

```ts
import type { Diagram } from './api';

let current: Diagram | null = $state.raw(null);

export const currentDiagram = {
  get diagram(): Diagram | null {
    return current;
  }
};

export const setCurrentDiagram = (diagram: Diagram): void => {
  current = diagram;
};

export const clearCurrentDiagram = (): void => {
  current = null;
};
```

- [ ] **Step 2: 创建 `src/lib/components/Dashboard/SaveDialog.svelte`**

（Dialog 组件 API 参考 `src/lib/components/Share.svelte` 的用法；本对话框只负责"首次保存"= POST。）

```svelte
<script lang="ts">
  import { Button } from '$/components/ui/button';
  import * as Dialog from '$/components/ui/dialog';
  import { Input } from '$/components/ui/input';
  import { TID } from '$/constants';
  import { createDiagram } from '$/util/api';
  import { setCurrentDiagram } from '$/util/currentDiagram.svelte';
  import { notify } from '$/util/notify';
  import { validatedState } from '$/util/state.svelte';

  interface Props {
    open: boolean;
    onOpenChange: (open: boolean) => void;
  }

  let { open = $bindable(), onOpenChange }: Props = $props();

  let name = $state('');
  let saving = $state(false);
  let error = $state('');

  $effect(() => {
    if (open) {
      name = '';
      error = '';
    }
  });

  const save = async () => {
    const trimmed = name.trim();
    if (!trimmed) {
      error = 'Name is required';
      return;
    }
    saving = true;
    try {
      const diagram = await createDiagram({
        name: trimmed,
        state: validatedState.current.serialized
      });
      setCurrentDiagram(diagram);
      notify('Saved');
      onOpenChange(false);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Save failed';
    } finally {
      saving = false;
    }
  };
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="sm:max-w-sm">
    <Dialog.Header>
      <Dialog.Title>Save diagram</Dialog.Title>
      <Dialog.Description>Choose a name for your diagram.</Dialog.Description>
    </Dialog.Header>
    <div class="flex flex-col gap-2">
      <Input
        bind:value={name}
        data-testid={TID.saveDialogInput}
        placeholder="Diagram name"
        onkeydown={(event) => {
          if (event.key === 'Enter' && !saving) {
            void save();
          }
        }} />
      {#if error}
        <p class="text-sm text-destructive">{error}</p>
      {/if}
    </div>
    <Dialog.Footer>
      <Button disabled={saving} variant="outline" onclick={() => onOpenChange(false)}
        >Cancel</Button>
      <Button disabled={saving} variant="accent" onclick={() => void save()}>
        {saving ? 'Saving…' : 'Save'}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
```

- [ ] **Step 3: edit 页接线**

`src/routes/(app)/edit/+page.svelte`：

imports 增加（顺序由 `pnpm lint:fix` 修正）：

```ts
import SaveDialog from '$/components/Dashboard/SaveDialog.svelte';
import { TID } from '$/constants';
import { updateDiagram } from '$/util/api';
import { currentDiagram, setCurrentDiagram } from '$/util/currentDiagram.svelte';
import { notify } from '$/util/notify';
```

state 区（`let isHistoryOpen = $state(false);` 附近）加：

```ts
let saveDialogOpen = $state(false);

const saveExisting = async () => {
  const existing = currentDiagram.diagram;
  if (!existing) {
    return;
  }
  try {
    setCurrentDiagram(
      await updateDiagram(existing.id, { state: validatedState.current.serialized })
    );
    notify('Saved');
  } catch (error) {
    notify(error instanceof Error ? error.message : 'Save failed');
  }
};
```

Navbar children 中 `<Share />` 之后加（不受 `env.isEnabledMermaidChartLinks` 门控，本地存储始终可用）：

```svelte
<Button
  variant="accent"
  size="sm"
  data-testid={TID.saveButton}
  onclick={() => {
    if (currentDiagram.diagram) {
      void saveExisting();
    } else {
      saveDialogOpen = true;
    }
  }}>
  Save
</Button>
```

页面末尾（Navbar 结束标签后）加：

```svelte
<SaveDialog bind:open={saveDialogOpen} onOpenChange={(open) => (saveDialogOpen = open)} />
```

- [ ] **Step 4: layout 的 hashchange 清空**

`src/routes/(app)/+layout.svelte` imports 加 `import { clearCurrentDiagram } from '$/util/currentDiagram.svelte';`，onMount 中监听器改为：

```ts
window.addEventListener('hashchange', () => {
  clearCurrentDiagram();
  void initHandler();
});
```

- [ ] **Step 5: constants.ts TID 增加（保持字母序）**

```ts
  saveButton: 'save-button',
  saveDialogInput: 'save-dialog-input',
```

- [ ] **Step 6: 检查与验证**

Run: `pnpm check && pnpm lint`
Expected: 无错误（lint:fix 处理 import 顺序）

功能验证（controller，浏览器打开 `/edit`）：点 Save → 命名框输入 "Test diagram" → Save → toast "Saved"；`curl -s http://localhost:3000/api/diagrams` 应含 `Test diagram`；再点 Save（不弹框）后 `updatedAt` 变化。

- [ ] **Step 7: 提交**

```bash
git add src/lib/util/currentDiagram.svelte.ts src/lib/components/Dashboard/SaveDialog.svelte "src/routes/(app)/edit/+page.svelte" "src/routes/(app)/+layout.svelte" src/lib/constants.ts
git commit -m "feat: local Save button with naming dialog in editor

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 5: Dashboard 子组件（FolderTree、Thumbnail、DiagramCard）

**Files:**

- Create: `src/lib/components/Dashboard/FolderTree.svelte`
- Create: `src/lib/components/Dashboard/Thumbnail.svelte`
- Create: `src/lib/components/Dashboard/DiagramCard.svelte`

**Interfaces:**

- Consumes: Task 3 的 `buildFolderTree`/`FolderNode`/`Folder`/`Diagram`、既有 `render(config, code, id)`（`src/lib/util/mermaid.ts`）、`deserializeState`（`$/util/serde`）、`prompt`/`confirm` 风格参照 `src/lib/util/notify.ts`
- Produces（Task 6 依赖）:
  - `FolderTree` props：`folders: Folder[]`、`selection: 'all' | 'favorites' | 'root' | string`（string=文件夹 id）、回调 `onSelect(selection): void`、`onCreate(name: string, parentId: string | null): void`、`onRename(id: string, name: string): void`、`onDelete(id: string): void`
  - `DiagramCard` props：`diagram: Diagram`、`folders: Folder[]`、回调 `onOpen(diagram: Diagram): void`、`onRename(diagram: Diagram, name: string): void`、`onMove(diagram: Diagram, folderId: string | null): void`、`onDelete(diagram: Diagram): void`、`onToggleFavorite(diagram: Diagram): void`
  - `Thumbnail` props：`id: string`、`state: string`

- [ ] **Step 1: 创建 `Thumbnail.svelte`（渲染队列串行化，避免并发 initialize 互踩）**

```svelte
<script lang="ts">
  import { render } from '$/util/mermaid';
  import { deserializeState } from '$/util/serde';

  let { id, state }: { id: string; state: string } = $props();

  let failed = $state(false);
  let svg = $state('');

  // mermaid.initialize is global; serialize renders so cards do not race.
  let queue: Promise<unknown> = Promise.resolve();
  const renderQueued = (config: unknown, code: string) => {
    const run = queue.then(() => render(config as never, code, `thumbnail-${id}`));
    queue = run.catch(() => {});
    return run;
  };

  $effect(() => {
    failed = false;
    svg = '';
    try {
      const parsed = deserializeState(state);
      void renderQueued(JSON.parse(parsed.mermaid || '{}'), parsed.code)
        .then((result) => {
          svg = result.svg;
        })
        .catch(() => {
          failed = true;
        });
    } catch {
      failed = true;
    }
  });
</script>

{#if svg}
  <div
    class="pointer-events-none flex h-32 items-center justify-center overflow-hidden p-2 [&_svg]:max-h-full [&_svg]:max-w-full">
    {@html svg}
  </div>
{:else if failed}
  <div class="flex h-32 items-center justify-center text-sm text-muted-foreground">
    Preview unavailable
  </div>
{:else}
  <div class="flex h-32 items-center justify-center text-sm text-muted-foreground">Loading…</div>
{/if}
```

- [ ] **Step 2: 创建 `FolderTree.svelte`**

```svelte
<script lang="ts">
  import { Button } from '$/components/ui/button';
  import { buildFolderTree, type FolderNode } from '$/util/folderTree';
  import type { Folder } from '$/util/api';
  import AddIcon from '~icons/material-symbols/add-rounded';
  import DeleteIcon from '~icons/material-symbols/delete-outline-rounded';
  import EditIcon from '~icons/material-symbols/edit-outline-rounded';
  import StarIcon from '~icons/material-symbols/star-rounded';
  import FolderIcon from '~icons/material-symbols/folder-rounded';

  interface Props {
    folders: Folder[];
    selection: 'all' | 'favorites' | 'root' | string;
    onCreate: (name: string, parentId: string | null) => void;
    onDelete: (id: string) => void;
    onRename: (id: string, name: string) => void;
    onSelect: (selection: 'all' | 'favorites' | 'root' | string) => void;
  }

  let { folders, selection, onCreate, onDelete, onRename, onSelect }: Props = $props();

  const quickLinks: { key: 'all' | 'favorites' | 'root'; label: string }[] = [
    { key: 'all', label: 'All diagrams' },
    { key: 'favorites', label: 'Favorites' },
    { key: 'root', label: 'Uncategorized' }
  ];

  const createFolder = (parentId: string | null) => {
    const name = window.prompt('Folder name');
    if (name && name.trim()) {
      onCreate(name.trim(), parentId);
    }
  };

  const renameFolder = (folder: Folder) => {
    const name = window.prompt('Rename folder', folder.name);
    if (name && name.trim() && name.trim() !== folder.name) {
      onRename(folder.id, name.trim());
    }
  };

  const deleteFolder = (folder: Folder) => {
    if (
      window.confirm(
        `Delete "${folder.name}"? Sub-folders are deleted; diagrams move to Uncategorized.`
      )
    ) {
      onDelete(folder.id);
    }
  };
</script>

<div class="flex w-56 shrink-0 flex-col gap-2 border-r border-border p-2">
  <Button size="sm" variant="outline" onclick={() => createFolder(null)}>
    <AddIcon class="size-4" /> New folder
  </Button>
  {#each quickLinks as link (link.key)}
    <button
      class="flex items-center gap-2 rounded-sm px-2 py-1 text-left text-sm hover:bg-muted {selection ===
      link.key
        ? 'bg-muted font-medium'
        : ''}"
      onclick={() => onSelect(link.key)}>
      {#if link.key === 'favorites'}
        <StarIcon class="size-4" />
      {:else}
        <FolderIcon class="size-4" />
      {/if}
      {link.label}
    </button>
  {/each}
  {#each buildFolderTree(folders) as node (node.folder.id)}
    {@render treeNode(node, 0)}
  {/each}
</div>

{#snippet treeNode(node: FolderNode, depth: number)}
  <div class="group flex items-center" style="padding-left: {depth * 12}px">
    <button
      class="flex flex-1 cursor-pointer items-center gap-2 truncate rounded-sm px-2 py-1 text-left text-sm hover:bg-muted {selection ===
      node.folder.id
        ? 'bg-muted font-medium'
        : ''}"
      onclick={() => onSelect(node.folder.id)}>
      <FolderIcon class="size-4 shrink-0" />
      <span class="truncate">{node.folder.name}</span>
    </button>
    <span class="hidden gap-0.5 group-hover:flex">
      <button
        class="p-1 text-muted-foreground hover:text-foreground"
        title="Rename"
        onclick={() => renameFolder(node.folder)}>
        <EditIcon class="size-3.5" />
      </button>
      <button
        class="p-1 text-muted-foreground hover:text-foreground"
        title="New sub-folder"
        onclick={() => createFolder(node.folder.id)}>
        <AddIcon class="size-3.5" />
      </button>
      <button
        class="p-1 text-muted-foreground hover:text-foreground"
        title="Delete"
        onclick={() => deleteFolder(node.folder)}>
        <DeleteIcon class="size-3.5" />
      </button>
    </span>
  </div>
  {#each node.children as child (child.folder.id)}
    {@render treeNode(child, depth + 1)}
  {/each}
{/snippet}
```

- [ ] **Step 3: 创建 `DiagramCard.svelte`**

```svelte
<script lang="ts">
  import { Button } from '$/components/ui/button';
  import * as Popover from '$/components/ui/popover';
  import { TID } from '$/constants';
  import type { Diagram, Folder } from '$/util/api';
  import { buildFolderTree, type FolderNode } from '$/util/folderTree';
  import dayjs from 'dayjs';
  import relativeTime from 'dayjs/plugin/relativeTime';
  import DotsIcon from '~icons/material-symbols/more-vert-rounded';
  import StarIcon from '~icons/material-symbols/star-rounded';
  import StarOutlineIcon from '~icons/material-symbols/star-outline-rounded';
  import Thumbnail from './Thumbnail.svelte';

  dayjs.extend(relativeTime);

  interface Props {
    diagram: Diagram;
    folders: Folder[];
    onDelete: (diagram: Diagram) => void;
    onMove: (diagram: Diagram, folderId: string | null) => void;
    onOpen: (diagram: Diagram) => void;
    onRename: (diagram: Diagram, name: string) => void;
    onToggleFavorite: (diagram: Diagram) => void;
  }

  let { diagram, folders, onDelete, onMove, onOpen, onRename, onToggleFavorite }: Props = $props();

  const rename = () => {
    const name = window.prompt('Rename diagram', diagram.name);
    if (name && name.trim() && name.trim() !== diagram.name) {
      onRename(diagram, name.trim());
    }
  };

  const remove = () => {
    if (window.confirm(`Delete "${diagram.name}"?`)) {
      onDelete(diagram);
    }
  };
</script>

<div
  class="flex flex-col rounded-xl border border-border bg-background shadow-sm"
  data-testid={TID.diagramCard}>
  <button class="cursor-pointer text-left" onclick={() => onOpen(diagram)}>
    <Thumbnail id={diagram.id} state={diagram.state} />
  </button>
  <div class="flex items-center gap-1 border-t border-border p-2">
    <button
      class="min-w-0 flex-1 cursor-pointer text-left"
      title="Open"
      onclick={() => onOpen(diagram)}>
      <div class="truncate text-sm font-medium">{diagram.name}</div>
      <div class="text-xs text-muted-foreground">{dayjs(diagram.updatedAt).fromNow()}</div>
    </button>
    <button
      class="p-1 {diagram.favorite ? 'text-accent' : 'text-muted-foreground hover:text-foreground'}"
      title={diagram.favorite ? 'Unfavorite' : 'Favorite'}
      onclick={() => onToggleFavorite(diagram)}>
      {#if diagram.favorite}
        <StarIcon class="size-4" />
      {:else}
        <StarOutlineIcon class="size-4" />
      {/if}
    </button>
    <Popover.Root>
      <Popover.Trigger class="p-1 text-muted-foreground hover:text-foreground" title="Actions">
        <DotsIcon class="size-4" />
      </Popover.Trigger>
      <Popover.Content class="w-44 p-1">
        <div class="flex flex-col">
          <Button size="sm" variant="ghost" onclick={() => onOpen(diagram)}>Open</Button>
          <Button size="sm" variant="ghost" onclick={rename}>Rename</Button>
          <Button size="sm" variant="ghost" onclick={remove}>Delete</Button>
          <div class="my-1 border-t border-border"></div>
          <div class="px-2 py-1 text-xs text-muted-foreground">Move to…</div>
          <Button
            size="sm"
            variant="ghost"
            class={diagram.folderId === null ? 'font-medium' : ''}
            onclick={() => onMove(diagram, null)}>
            Uncategorized
          </Button>
          {#each buildFolderTree(folders) as node (node.folder.id)}
            {@render folderOption(node, 0)}
          {/each}
        </div>
      </Popover.Content>
    </Popover.Root>
  </div>
</div>

{#snippet folderOption(node: FolderNode, depth: number)}
  <Button
    size="sm"
    variant="ghost"
    class="justify-start {diagram.folderId === node.folder.id ? 'font-medium' : ''}"
    style="padding-left: {12 + depth * 12}px"
    onclick={() => onMove(diagram, node.folder.id)}>
    {node.folder.name}
  </Button>
  {#each node.children as child (child.folder.id)}
    {@render folderOption(child, depth + 1)}
  {/each}
{/snippet}
```

- [ ] **Step 4: 检查**

Run: `pnpm check && pnpm lint`
Expected: 无错误

- [ ] **Step 5: 提交**

```bash
git add src/lib/components/Dashboard/
git commit -m "feat: dashboard folder tree, thumbnail and diagram card components

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 6: Dashboard 页面与导航入口

**Files:**

- Create: `src/routes/(app)/dashboard/+page.svelte`
- Modify: `src/routes/(app)/edit/+page.svelte`（Navbar children 加 Diagrams 链接）
- Modify: `src/routes/(app)/view/+page.svelte`（若有同样的 Navbar children 区，同样加 Diagrams 链接）
- Modify: `src/lib/constants.ts`（TID 增加 `dashboardPage`、`diagramsLink`、`exportButton`、`importInput`、`newDiagramButton`、`searchInput`，保持字母序）

**Interfaces:**

- Consumes: Task 3 全部 API 函数与类型、Task 4 `currentDiagram`/`setCurrentDiagram`/`clearCurrentDiagram`、Task 5 三个组件、既有 `replaceInputState`/`defaultState`（`$/util/state.svelte`）、`deserializeState`（`$/util/serde`）、`goto`（`$app/navigation`）
- Produces: `/dashboard` 路由；Navbar "Diagrams" 入口

- [ ] **Step 1: 创建 dashboard 页面**

```svelte
<script lang="ts">
  import DiagramCard from '$/components/Dashboard/DiagramCard.svelte';
  import FolderTree from '$/components/Dashboard/FolderTree.svelte';
  import Navbar from '$/components/Navbar';
  import { Button } from '$/components/ui/button';
  import { Input } from '$/components/ui/input';
  import { TID } from '$/constants';
  import {
    createFolder,
    deleteDiagram,
    deleteFolder,
    exportAll,
    importAll,
    listDiagrams,
    listFolders,
    updateDiagram,
    updateFolder,
    type Diagram,
    type Folder
  } from '$/util/api';
  import { clearCurrentDiagram, setCurrentDiagram } from '$/util/currentDiagram.svelte';
  import { notify } from '$/util/notify';
  import { deserializeState } from '$/util/serde';
  import { defaultState, replaceInputState } from '$/util/state.svelte';
  import { goto } from '$app/navigation';
  import AddIcon from '~icons/material-symbols/add-rounded';
  import DownloadIcon from '~icons/material-symbols/download-rounded';
  import { onMount } from 'svelte';

  type Selection = 'all' | 'favorites' | 'root' | string;

  let diagrams = $state<Diagram[]>([]);
  let folders = $state<Folder[]>([]);
  let importInput: HTMLInputElement | undefined = $state();
  let loading = $state(true);
  let search = $state('');
  let selection = $state<Selection>('all');

  const reload = async () => {
    loading = true;
    try {
      folders = await listFolders();
      diagrams = await listDiagrams({
        favorite: selection === 'favorites',
        folderId:
          selection === 'all' || selection === 'favorites' ? undefined : (selection as string),
        search: search.trim() || undefined
      });
    } catch (error) {
      notify(error instanceof Error ? error.message : 'Failed to load');
    } finally {
      loading = false;
    }
  };

  let searchTimer: ReturnType<typeof setTimeout> | undefined;
  const onSearch = (value: string) => {
    search = value;
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => void reload(), 300);
  };

  const onSelect = (next: Selection) => {
    selection = next;
    void reload();
  };

  const openDiagram = (diagram: Diagram) => {
    clearCurrentDiagram();
    replaceInputState(deserializeState(diagram.state));
    setCurrentDiagram(diagram);
    void goto('/edit');
  };

  const newDiagram = () => {
    clearCurrentDiagram();
    replaceInputState(defaultState);
    void goto('/edit');
  };

  const withReload =
    (action: (diagramOrId: never, second?: never) => Promise<unknown>) =>
    async (...args: unknown[]) => {
      try {
        await action(...(args as [never, never]));
        await reload();
      } catch (error) {
        notify(error instanceof Error ? error.message : 'Operation failed');
      }
    };

  const downloadExport = async () => {
    try {
      const data = await exportAll();
      const url = URL.createObjectURL(
        new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
      );
      const link = document.createElement('a');
      link.download = `mermaid-diagrams-${new Date().toISOString().slice(0, 10)}.json`;
      link.href = url;
      link.click();
      URL.revokeObjectURL(url);
    } catch (error) {
      notify(error instanceof Error ? error.message : 'Export failed');
    }
  };

  const onImportFile = async (event: Event) => {
    const input = event.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) {
      return;
    }
    try {
      const result = await importAll(JSON.parse(await file.text()));
      notify(`Imported ${result.imported}, skipped ${result.skipped}`);
      await reload();
    } catch (error) {
      notify(error instanceof Error ? error.message : 'Import failed');
    } finally {
      input.value = '';
    }
  };

  onMount(() => void reload());
</script>

<Navbar hidePromotion>
  <div class="flex flex-1 items-center justify-end gap-2">
    <Input
      class="h-8 w-48"
      data-testid={TID.searchInput}
      placeholder="Search diagrams…"
      value={search}
      oninput={(event) => onSearch(event.currentTarget.value)} />
    <Button size="sm" variant="accent" data-testid={TID.newDiagramButton} onclick={newDiagram}>
      <AddIcon class="size-4" /> New diagram
    </Button>
    <Button
      size="sm"
      variant="outline"
      data-testid={TID.exportButton}
      onclick={() => void downloadExport()}>
      <DownloadIcon class="size-4" /> Export
    </Button>
    <Button size="sm" variant="outline" onclick={() => importInput?.click()}>Import</Button>
    <input
      bind:this={importInput}
      accept="application/json"
      class="hidden"
      data-testid={TID.importInput}
      type="file"
      onchange={(event) => void onImportFile(event)} />
  </div>
</Navbar>

<div class="flex h-[calc(100dvh-5rem)] gap-2 p-2" data-testid={TID.dashboardPage}>
  <FolderTree
    {folders}
    {selection}
    onCreate={withReload((name: string, parentId: string | null) => createFolder(name, parentId))}
    onDelete={withReload((id: string) => deleteFolder(id))}
    onRename={withReload((id: string, name: string) => updateFolder(id, { name }))}
    {onSelect} />
  <div class="flex-1 overflow-y-auto p-2">
    {#if loading}
      <div class="flex h-full items-center justify-center text-muted-foreground">Loading…</div>
    {:else if diagrams.length === 0}
      <div class="flex h-full flex-col items-center justify-center gap-2 text-muted-foreground">
        <p>No diagrams yet.</p>
        <Button size="sm" variant="outline" onclick={newDiagram}>Create your first diagram</Button>
      </div>
    {:else}
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {#each diagrams as diagram (diagram.id)}
          <DiagramCard
            {diagram}
            {folders}
            onDelete={withReload((d: Diagram) => deleteDiagram(d.id))}
            onMove={withReload((d: Diagram, folderId: string | null) =>
              updateDiagram(d.id, { folderId })
            )}
            onOpen={openDiagram}
            onRename={withReload((d: Diagram, name: string) => updateDiagram(d.id, { name }))}
            onToggleFavorite={withReload((d: Diagram) =>
              updateDiagram(d.id, { favorite: !d.favorite })
            )} />
        {/each}
      </div>
    {/if}
  </div>
</div>
```

- [ ] **Step 2: Navbar 加 Diagrams 入口**

edit 页 Navbar children 最前面加：

```svelte
<Button size="sm" variant="ghost" data-testid={TID.diagramsLink} href="/dashboard">Diagrams</Button>
```

view 页如有同构 Navbar children 区，同样添加。

- [ ] **Step 3: constants.ts TID 增加（保持字母序）**

```ts
  dashboardPage: 'dashboard-page',
  diagramsLink: 'diagrams-link',
  exportButton: 'export-button',
  importInput: 'import-input',
  newDiagramButton: 'new-diagram-button',
  searchInput: 'search-input',
```

（`diagramCard` 已在 Task 5 需要时一并加入。）

- [ ] **Step 4: 检查**

Run: `pnpm check && pnpm lint`
Expected: 无错误

- [ ] **Step 5: 提交**

```bash
git add "src/routes/(app)/dashboard" "src/routes/(app)/edit/+page.svelte" "src/routes/(app)/view/+page.svelte" src/lib/constants.ts
git commit -m "feat: dashboard route with search, favorites and import/export

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 7: 生产部署配置

**Files:**

- Modify: `Dockerfile`（增加 storage-api stage）
- Modify: `docker-compose.yml`（增加 api 服务）
- Modify: `nginx.conf`（增加 /api location）

**Interfaces:**

- Consumes: Task 1 的 `server/`
- Produces: 容器化部署形态（nginx 静态 + api 服务 + /api 反代）

- [ ] **Step 1: Dockerfile 末尾追加 stage**

```dockerfile
FROM docker.io/library/node:24.16.0-alpine3.22 AS storage-api

ENV HOST=0.0.0.0
WORKDIR /app
COPY ./server ./server
VOLUME /app/server-data
EXPOSE 18781
CMD ["node", "server/index.mjs"]
```

- [ ] **Step 2: docker-compose.yml 增加服务**

```yaml
api:
  build:
    context: .
    target: storage-api
  volumes:
    - ./server-data:/app/server-data
  ports:
    - 18781:18781
```

- [ ] **Step 3: nginx.conf 增加 location（`location /` 之前）**

```nginx
 location /api {
   proxy_pass http://api:18781;
   proxy_set_header Host $host;
 }
```

- [ ] **Step 4: 验证**

Run: `docker compose config 2>&1 | head -30`（docker 不可用则人工核对 YAML/nginx 语法）
Expected: compose 配置解析通过，含 `api` 服务

- [ ] **Step 5: 提交**

```bash
git add Dockerfile docker-compose.yml nginx.conf
git commit -m "feat: containerized storage-api service with nginx /api proxy

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 8: 端到端验证（controller 执行）

**Files:**

- 无新增入库文件（Playwright 脚本放 SDD 工作区，gitignored）

**Interfaces:**

- Consumes: 全部前序任务 + 运行中的 dev server + 存储服务

- [ ] **Step 1: 全量单元测试**

Run: `pnpm vitest run --dir src`
Expected: 全部 PASS（含既有用例与新增 7 例）

- [ ] **Step 2: Playwright 全流程（真实事件，模式同 AI 功能验证）**

脚本要点（完整代码执行时写入工作区）：打开 `/edit` → Save → 命名 "E2E Test" → toast Saved → 经 `/api/diagrams` 确认存在 → 编辑器追加新节点 → Save（无弹框）→ `updatedAt` 变化 → 打开 `/dashboard` → 卡片可见且缩略图非 Loading → 搜索 "E2E" 过滤 → 收藏 → Favorites 过滤可见 → 建文件夹并移动 → 文件夹树显示 → 删除图表 → 列表为空 → 导出文件含数据 → 重新导入 → skipped 计数 = 导出条数（去重验证）

- [ ] **Step 3: 最终检查**

Run: `pnpm check && pnpm lint`
Expected: 通过

---

## Self-Review 记录

- **规格覆盖**：存储服务/REST API（Task 1）、代理与脚本（Task 2）、api 客户端+树工具（Task 3）、Save+currentDiagram+hash 失效（Task 4）、缩略图/树/卡片（Task 5）、dashboard 页/搜索/收藏/导入导出/导航（Task 6）、生产部署（Task 7）、测试计划四项（Task 1 冒烟、Task 3 单测、Task 8 E2E、各任务 check+lint）✓；商业 UI 清理 = Task 2 的 controller 步骤（.env.local）✓
- **占位符**：Task 8 Step 2 为执行时编写的 controller 脚本（模式已在 AI 功能验证中建立并注明）；其余步骤均含完整代码 ✓
- **类型一致性**：`Folder`/`Diagram`/`FolderNode`/`currentDiagram`/各回调签名在 Task 3/4/5/6 间一致 ✓；`withReload` 用宽松泛化签名适配多种回调参数 ✓
