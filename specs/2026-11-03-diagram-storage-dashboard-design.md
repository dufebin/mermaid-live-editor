# 设计：图表存储 + Dashboard（子项目 1）

日期：2026-11-03
状态：已批准

关联：后续"子项目 2：图下方 AI 输入条"另行设计（复用本仓库已接通的本地网关 `src/lib/util/ai.ts`）。

## 背景与目标

开源 mermaid-live-editor 是纯静态 SPA，无任何后端；持久化仅 localStorage（History 会话时间线）。目标：对标 mermaid.ai dashboard，为本地部署实例提供**图表保存与管理**——列表、文件夹目录、缩略图预览、搜索、收藏、导出备份，数据持久化到服务端 SQLite 数据库。同时清理商业引流 UI。

需求确认记录：

- 存储后端：SQLite（Node 24 内置 `node:sqlite`，零依赖）
- UI 形态：独立 `/dashboard` 路由
- 功能集：核心 CRUD + 缩略图预览 + 搜索 + 收藏 + 导出备份
- Share 按钮（本地 URL 分享）保留；商业引流入口删除
- 单用户、无鉴权（本地部署场景）

## 关键决策与依据

- **存储服务形态（方案 A）**：仓库内 `server/` 目录，零依赖 Node 服务（`node:http` + `node:sqlite`）。否决 Express/better-sqlite3（依赖+原生编译不必要）与浏览器 wa-sqlite（不符合服务端持久化）。
- **商业 UI 清理用配置而非删码**：`MERMAID_IS_ENABLED_MERMAID_CHART_LINKS='false'` 经 `McWrapper`/`getActivePromotion()` 门控，同时隐藏 banner、"Contact sales"、"Save diagram"（商业版）、"Edit with AI"（商业跳转）。上游同步零冲突。
- **序列化复用**：图表内容存与 URL hash 相同的序列化 State；加载走现有 `replaceInputState()`，校验/持久化/主题/AI 全部无缝。

## 1. 存储服务

**文件**：`server/index.mjs`（HTTP 路由）、`server/db.mjs`（SQLite 封装）

- `node:http` 监听 `127.0.0.1:18781`（`PORT`/`HOST` 环境变量可覆盖）；JSON 请求/响应；错误统一 `{ error: string }` + 恰当状态码
- `node:sqlite` `DatabaseSync`，WAL 模式；启动时建表（`CREATE TABLE IF NOT EXISTS`）与索引（diagrams.folder_id、favorite）
- 数据文件 `server-data/diagrams.db`（目录不存在则创建；gitignored：`.gitignore` 追加 `server-data/`）
- 时间戳 ISO 8601 UTC；id 用 `node:crypto` `randomUUID()`

**表结构**：

```sql
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
```

**REST API**：

| 方法   | 路径                | 行为                                                                                                 |
| ------ | ------------------- | ---------------------------------------------------------------------------------------------------- |
| GET    | `/api/folders`      | 返回全部文件夹（平铺列表，前端组树）                                                                 |
| POST   | `/api/folders`      | `{name, parentId?}` 新建（parentId 空为根级）                                                        |
| PATCH  | `/api/folders/:id`  | `{name?, parentId?}` 重命名/移动； parentId 环检测（禁止自引用/成环）                                |
| DELETE | `/api/folders/:id`  | 子文件夹级联删除；**其中图表移至未分类（folder_id 置 NULL）**，不丢数据                              |
| GET    | `/api/diagrams`     | 查询参数 `folderId`（含 `root` 表示未分类）、`search`（名称 LIKE）、`favorite=1`；按 updated_at 倒序 |
| POST   | `/api/diagrams`     | `{name, folderId?, state}` 新建                                                                      |
| GET    | `/api/diagrams/:id` | 单个详情                                                                                             |
| PATCH  | `/api/diagrams/:id` | `{name?, folderId?, state?, favorite?}` 任意子集更新，刷新 updated_at                                |
| DELETE | `/api/diagrams/:id` | 删除                                                                                                 |
| GET    | `/api/export`       | `{folders: [...], diagrams: [...], exportedAt}` 全量 JSON                                            |
| POST   | `/api/import`       | 合并导入：按 id 已存在则**跳过**（不覆盖），返回 `{imported, skipped}`                               |

输入校验：必填字段缺失/类型不对 → 400；不存在 id → 404；环 → 400。

## 2. 前端 API 客户端 `src/lib/util/api.ts`

```ts
export interface Folder {
  id;
  name;
  parentId;
  createdAt;
  updatedAt;
}
export interface Diagram {
  id;
  folderId;
  name;
  state;
  favorite;
  createdAt;
  updatedAt;
}
```

类型化 fetch 包装（listFolders / createFolder / updateFolder / deleteFolder / listDiagrams / createDiagram / getDiagram / updateDiagram / deleteDiagram / exportAll / importAll），非 2xx 与网络错误统一抛 `Error`（携带服务端 error 信息），调用侧 `notify()` 提示。

## 3. Dashboard 路由 `src/routes/(app)/dashboard/+page.svelte`

（子组件放 `src/lib/components/Dashboard/`）

- **左侧栏**：文件夹树（层级缩进、当前选中高亮）、每项悬浮菜单：重命名/删除/新建子文件夹；顶部按钮"新建文件夹"；快捷项："全部图表"、"收藏"
- **主区**：卡片网格，每卡 = mermaid 客户端实时渲染缩略图（小尺寸 SVG，渲染失败显示占位）+ 名称 + 相对更新时间（dayjs，已有依赖）+ 收藏星标切换；卡片菜单：打开/重命名/移动到文件夹/删除（删除需 confirm）
- **顶栏**：`New diagram` 按钮（跳 `/edit` 空白，复用 `defaultState`）、搜索输入框（300ms 防抖 → `search` 参数）、导出（下载 JSON）/导入（文件选择 → `/api/import` → 刷新列表）按钮
- 空态文案引导首次使用
- Navbar 增加 `Diagrams` 入口（所有页面可见，桌面/移动一致）

## 4. 编辑器集成

- **Save 按钮**（Navbar，替换被 `isEnabledMermaidChartLinks=false` 隐藏的商业按钮位置；不包 McWrapper）：
  - 会话内无 `currentDiagramId` → 弹命名 Dialog（复用现有 ui/dialog 组件）→ `POST /api/diagrams` → 记录 id → toast "Saved"
  - 已有 `currentDiagramId` → 直接 `PATCH state`（名称不变）→ toast "Saved"
  - 保存的是 `validatedState.current` 的序列化 State（与 URL hash 同源：`serde.ts` 序列化）
- **打开**：dashboard 卡片"打开"→ `replaceInputState(deserializeState(state))` → 跳 `/edit` → 内存记录 `currentDiagramId`（模块级 `$state`，不入 URL、不持久化）
- **失效**：URL hash 变化（initHandler 已有流程）→ `currentDiagramId` 置空；此时 Save = 另存为新图表
- `currentDiagramId` 管理：`src/lib/util/currentDiagram.svelte.ts` 小模块（get/set/clear）

## 5. 商业 UI 清理

`.env.local` 追加 `MERMAID_IS_ENABLED_MERMAID_CHART_LINKS='false'`。效果：banner、Contact sales、商业 Save diagram、商业 Edit with AI 全部隐藏（`McWrapper` isVisible + `getActivePromotion` 早退）。Share 保留。

## 6. 部署

- **dev**：`vite.config.js` 的 aiProxy 旁新增 `/api` 代理 → `http://127.0.0.1:18781`（读 `API_PROXY_TARGET`，默认该地址）；`package.json` 新增 scripts：`server`（`node server/index.mjs`）、`dev:all`（并行 `pnpm server` 与 `pnpm dev`，零依赖起见用 shell 并行 + trap）
- **生产**：`docker-compose.yml` 增加 `api` 服务（`docker.io/library/node:24-alpine`，挂载 `./server-data` 卷，运行 `node server/index.mjs`）；`nginx.conf` 增加 `location /api { proxy_pass http://api:18781; }`；Dockerfile 增加 api stage。此部分为配置交付，验证以 compose 起 dev 环境为准。

## 7. 测试计划

1. **单元测试**（vitest，jsdom）：`api.ts`（stub fetch：正常解析、错误抛出）；文件夹树工具函数（组树、环检测辅助）
2. **服务端冒烟**（脚本或 curl 序列）：启动服务 → 建文件夹（含嵌套）→ 存图 → 列表/搜索/收藏过滤 → 移动/重命名 → 删除文件夹验证图表降级未分类 → 导出/导入去重 → 环检测 400
3. **浏览器 E2E**（Playwright 真实事件，模式同 AI 功能验证）：编辑器保存（命名框）→ dashboard 见卡片与缩略图 → 再编辑再存 → 打开恢复 → 搜索过滤 → 收藏 → 移动文件夹 → 删除 → 导出下载文件 → 导入
4. `pnpm check && pnpm lint`

## 8. 范围边界

- 单用户无鉴权；不做回收站、协作、版本历史（History 面板已有）、分享权限
- 缩略图客户端实时渲染，不落库
- `server/` 不引入 npm 依赖、不用 TypeScript（保持零构建；纯 ESM `.mjs`）
- 子项目 2（图下方 AI 输入条）不在本 spec 范围
