# 本地运行手册（mermaid-live-editor 定制版）

本仓库在开源版基础上做了本地化定制：AI 图表编辑（本地 LLM 网关）、图表存储 + Dashboard（SQLite）、可视化编辑（选中/拖拽/改标签）。本文只讲怎么跑起来。

## 架构（3 个端口）

| 服务       | 端口                      | 进程                                             | 说明                                                          |
| ---------- | ------------------------- | ------------------------------------------------ | ------------------------------------------------------------- |
| 前端编辑器 | **http://localhost:3000** | `pnpm dev`（vite）                               | 主入口，`/edit` 编辑、`/dashboard` 管理                       |
| 存储服务   | 127.0.0.1:18790           | `pnpm run server`（Go：`make -C server-go run`） | 图表/文件夹/位置的 REST API，数据落 `server-data/diagrams.db` |
| LLM 网关   | 127.0.0.1:18780           | （外部已有，勿动）                               | deepseek-v4.1-flash，前端经 vite `/ai` 代理访问               |

前端通过 vite 代理访问另两个服务（`/api` → 18790、`/ai` → 18780），浏览器只需访问 3000 一个端口。

## 启动

### 一键启动（推荐）

```bash
cd /data/opensource/mermaid-live-editor
pnpm dev:all        # 同时启动存储服务 + 前端
```

### 分开启动（两个终端）

```bash
pnpm run server   # 终端 1：存储服务（先启动）
pnpm dev          # 终端 2：前端（首次含 embed 构建，约几分钟）
```

> ⚠️ 必须写 `pnpm run server`，不能写 `pnpm server`——后者命中 pnpm 内建命令，静默退出 0，服务根本没启动。

> 存储服务是 Go 写的（`server-go/`）。改了 Go 代码要先编译：`make -C server-go build`；`make -C server-go test` 跑 Go 测试。

> 依赖变化后才需要 `pnpm install`；日常启动不需要。

## 重启

### 重启存储服务（数据不丢）

```bash
# 找到并停掉旧进程
pkill -f "server-go/bin/mermaid-storage"
# 重新启动（后台常驻）
pnpm run server &   # 或 nohup pnpm run server > /tmp/mermaid-storage.log 2>&1 &
# 生产环境（systemd）直接：
sudo systemctl restart mermaid-storage
```

SQLite 文件持久 + 迁移幂等，重启/升级代码后数据（图表、文件夹、节点位置）完整保留。

### 重启前端

```bash
# dev:all / pnpm dev 的终端里 Ctrl+C，然后
pnpm dev
```

**必须重启前端的情况**：改了 `vite.config.js`、`.env` 或 `.env.local`（vite 代理与 env 注入在启动时读取；注意本栈中 `.env` 冲突键优先于 `.env.local`——改默认值直接改 `.env`）。

改 `src/` 下的代码**不用重启**（自动整页刷新）。

## 配置

| 文件         | 内容                                                                      | 是否入库      |
| ------------ | ------------------------------------------------------------------------- | ------------- |
| `.env`       | 默认配置；`MERMAID_IS_ENABLED_MERMAID_CHART_LINKS='false'` 已关商业 UI    | ✅ 入库       |
| `.env.local` | AI 网关地址与密钥（`AI_PROXY_TARGET`/`AI_PROXY_KEY`）、`MERMAID_AI_MODEL` | ❌ gitignored |

## 账号（登录 / 注册）

- `/edit` 和 `/dashboard` 需要登录；未登录访问会跳到 `/login`（带 `?redirect=`，登录后原路返回）。`/view`、`/embed` 保持公开。
- **首个注册的用户自动继承库里已有的全部图表/文件夹**（历史无主数据一次性划归首个注册者）；之后注册的用户从空库开始，相互完全隔离。
- JWT 会话密钥自动生成并保存在 `<DATA_DIR>/jwt-secret`（默认 `server-data/jwt-secret`，32 字节随机数）。换密钥会使所有登录会话失效。
- 可用环境变量 `MERMAID_JWT_SECRET` 覆盖文件密钥（优先级更高）。
- 存储服务（18790）不启动时，登录接口不可用，前端会表现为注册/登录失败。

### 账号相关 e2e 测试（隔离运行）

```bash
pnpm test:e2e:auth
```

- 自带独立后端栈：每次运行先 `rm -rf server-data/e2e`，再用 `DATA_DIR=server-data/e2e PORT=18799` 启动一个新的 Go 存储服务，前端跑在 3100（`--strictPort`）。
- **不碰真实数据**（`server-data/diagrams.db`）和 systemd 服务；与常驻的 3000/18790 互不影响。

> ⚠️ **默认 e2e 套件（`pnpm test:e2e` / `pnpm test`）没有这种隔离**：它会复用常驻的 3000 前端、把 `/api` 代理到 18790 的真实库。套件夹具会注册 `e2e@example.com`——**若此时库里还没有任何用户**，该测试账号就会作为"首个注册者"静默认领全部真实图表/文件夹，之后本人再注册就只能拿到空库。涉及登录/账号的测试一律用 `pnpm test:e2e:auth`；默认套件要长期安全，应把 `playwright.config.ts` 的 webServer 改成指向一次性后端（做法同 auth 配置），建议后续跟进。
>
> 如果已经发生（发现 `e2e@example.com` 等测试账号占有了真实数据），在**本人尚未注册真实账号**时执行恢复（否则删用户会让该用户的数据变成无主孤儿）：
>
> ```sql
> -- sqlite3 server-data/diagrams.db
> UPDATE folders SET user_id = NULL;
> UPDATE diagrams SET user_id = NULL;
> DELETE FROM users;
> ```

## 生产部署（编译版）

生产模式用单个 Go 进程同时提供静态前端（`bin/static/`）和存储 API，监听 `0.0.0.0:3000`，不再需要 vite dev server。

### 构建产物

所有编译产物集中在仓库根的 `bin/` 目录：

```
bin/
  mermaid-storage   # Go 二进制（API + 静态服务）
  static/           # 前端静态文件（从 docs/ 组装，供 STATIC_DIR 使用）
  static.prev/      # 上一代前端（保留一代，用于回滚；可随时删）
```

```bash
pnpm build:prod   # pnpm build（写 docs/）→ make build（写 bin/mermaid-storage）→ 组装 bin/static/
```

> `pnpm build` 的输出目录仍是 `docs/`（上游/Docker/Pages 约定不变）；`docs/` 只是构建中间产物，可随时删除回收空间。

### 启动方式

**systemd（生产标准方式）**：

```bash
sudo systemctl start mermaid-frontend      # 启动
sudo systemctl restart mermaid-frontend    # 重启（更新后生效）
sudo systemctl status mermaid-frontend     # 状态
journalctl -u mermaid-frontend -f          # 看日志
```

**手动一次性运行（调试用）**——⚠️ 必须先停掉 systemd 生产单元，否则两个进程双写同一个 SQLite：

```bash
sudo systemctl stop mermaid-frontend
cd /data/opensource/mermaid-live-editor
HOST=0.0.0.0 PORT=3000 STATIC_DIR=bin/static DATA_DIR=server-data ./bin/mermaid-storage
# Ctrl+C 结束；结束后 sudo systemctl start mermaid-frontend 恢复生产
```

### 首次切换（从 dev 模式到生产模式）

```bash
sudo systemctl stop mermaid-storage
sudo systemctl stop mermaid-live 2>/dev/null
sudo systemctl stop mermaid-frontend 2>/dev/null  # 已在跑的旧实例不会随 daemon-reload 重启，必须显式停掉
sudo systemctl disable mermaid-live               # 防止重启后与生产单元抢 3000 端口
sudo systemctl disable mermaid-storage            # 防止重启后 18790 回来，与生产进程双写 SQLite
# disable 会把 /etc 里的 storage 符号链接一并删掉，补回以保持"已注册、未启用"（开发模式仍可手动 start）：
sudo ln -sf /data/opensource/mermaid-live-editor/systemd/mermaid-storage.service /etc/systemd/system/mermaid-storage.service
sudo systemctl daemon-reload
sudo systemctl enable --now mermaid-frontend
```

### 验证

```bash
curl -s localhost:3000/edit | head -c 200        # 应返回 SPA HTML
curl -s localhost:3000/api/auth/me               # 应返回 401 {"error":"unauthorized"}
```

> ⚠️ **生产运行时禁止 `mermaid-storage`（18790）同时运行**——两个进程双写同一个 SQLite 文件会损坏数据。生产单元（`systemd/mermaid-frontend.service`）因此不依赖 `Wants=mermaid-storage.service`，`STATIC_DIR=bin/static` 让同一个二进制直接服务前端。

> 注意：**AI 编辑在生产模式下暂不可用**（Go 未代理 `/ai` 网关，待后续补齐）。下文"常见问题"里的 AI 排查步骤仅适用于开发模式。

### 切换回开发模式

```bash
sudo systemctl stop mermaid-frontend
sudo systemctl disable mermaid-frontend  # 防止开发期间重启机器把生产进程带回来，与 18790 双写 SQLite
# 起存储服务 + vite dev（vite 把 /api 代理到 18790）：
sudo systemctl start mermaid-storage
pnpm dev
```

> 开发模式期间 `mermaid-live` 和 `mermaid-frontend` 均已停止并 disable，重启机器也不会有任何服务抢 3000 端口或双写 SQLite。

### 回到生产模式（与首次切换对称）

```bash
sudo systemctl stop mermaid-storage     # 必须先停，避免双写 SQLite
sudo systemctl enable --now mermaid-frontend
```

### 更新流程（改代码后上线）

```bash
pnpm build:prod
sudo systemctl restart mermaid-frontend   # 数据不丢：SQLite 文件持久 + 迁移幂等
```

> 前端零停机：构建写 `docs/`、线上始终服务旧的 `bin/static/`，组装换新后才由 restart 生效。纯后端改动可跳过前端构建，直接 `make -C server-go build` 后 restart（`go build -o` 对运行中的二进制安全替换）。回滚前端 = 把 `bin/static.prev` 换回 `bin/static` 后 restart。

## 常见问题

- **端口被占**：`ss -tlnp "sport = :3000"`（或 `:18790`）看占用进程，`kill <pid>` 后重启对应服务
- **存储服务起不来**：看日志 `/tmp/mermaid-storage.log`（若用 nohup 启动）
- **AI 编辑无响应**：先确认网关活着 `curl http://127.0.0.1:18780/v1/models -H "Authorization: Bearer <key>"`，再确认存储/前端代理正常 `curl http://localhost:3000/api/folders`
- **备份图表**：拷走 `server-data/diagrams.db` 即可；或在 Dashboard 里用 Export 导出 JSON

## 快捷键与功能速查

- `Ctrl+S` / `⌘S`：保存（未保存弹命名框，可选文件夹；已保存直接更新）
- 点击图中节点：选中高亮 + 代码定位 + AI 上下文；双击改标签；拖拽移动（位置随保存持久化）
- 图下方 AI 输入条：自然语言改图；选中节点后自动附上下文

## 管理命令：

- systemctl status mermaid-live — 前端状态
- journalctl -u mermaid-live -f — 前端日志
- systemctl restart mermaid-live — 重启前端
- systemctl status mermaid-storage — 存储服务状态（`/etc/systemd/system/mermaid-storage.service` 是指向本仓库 `systemd/` 的符号链接；ExecStart 直接跑 Go 二进制 `server-go/bin/mermaid-storage`，改代码后 `make -C server-go build` 再 restart）
- journalctl -u mermaid-storage -f — 存储服务日志
- systemctl restart mermaid-storage — 重启存储服务（数据不丢）
