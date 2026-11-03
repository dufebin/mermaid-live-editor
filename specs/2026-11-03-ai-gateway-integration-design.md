# 设计：AI 图表编辑接入本地 DeepSeek 网关

日期：2026-11-03
状态：已批准

## 背景与目标

mermaid-live-editor 的 AI 弹窗（`AIPromptPopup`，行锚定，提示文案 "Describe what to add or change"）目前只是把用户引导到商业版 mermaid.ai，开源代码中没有任何 LLM API 调用。

目标：将该弹窗接入本地 OpenAI 兼容网关，使用 `deepseek-v4.1-flash` 模型，实现"编辑当前图表"——把当前 Mermaid 代码与用户指令一起发给模型，返回修改后的完整代码替换编辑器内容。编辑器为空时等价于从零生成。

## 关键约束（已实测）

- 网关 `http://127.0.0.1:18780/v1` **不支持 CORS**：preflight OPTIONS 返回 404，实际响应无 `Access-Control-Allow-Origin` 头。浏览器无法从 `localhost:3000` 直接 fetch。
- 因此采用 **Vite 开发代理**（方案 A）：浏览器请求同源路径 `/ai/v1/chat/completions`，由 Vite 服务端转发（服务端转发不受 CORS 限制），并在代理层注入 `Authorization` 头。
- 本应用是纯静态 SPA（`@sveltejs/adapter-static`），`envPrefix: 'MERMAID_'` 会把所有 `MERMAID_` 前缀的环境变量打进客户端 bundle —— **密钥变量不得使用该前缀**。

## 1. 配置层

**`.env.local`**（gitignored）：

```bash
# 客户端可见（MERMAID_ 前缀 → import.meta.env 自动暴露）
MERMAID_AI_ENABLED='true'
MERMAID_AI_MODEL='deepseek-v4.1-flash'

# 仅 vite.config.js 服务端使用（无 MERMAID_ 前缀，不会进前端 bundle）
AI_PROXY_TARGET='http://127.0.0.1:18780'
AI_PROXY_KEY='<网关 API key，形如 300001934:xxxx>'
```

**`vite.config.js`**：

- 用 `loadEnv(mode, process.cwd(), '')` 读取 `AI_PROXY_TARGET` / `AI_PROXY_KEY`
- `server.proxy` 与 `preview.proxy` 均添加：`'/ai'` → target，`rewrite` 去掉 `/ai` 前缀，代理请求头注入 `Authorization: Bearer <AI_PROXY_KEY>`
- `AI_PROXY_TARGET` 未配置时不注册代理（零影响默认行为）

## 2. AI 模块（新文件 `src/lib/util/ai.ts`）

```ts
export const aiEnabled: boolean; // import.meta.env.MERMAID_AI_ENABLED === 'true'
export function parseAIContent(raw: string): string; // 纯函数，便于单测
export async function editDiagramWithAI(instruction: string, currentCode: string): Promise<string>;
```

- `editDiagramWithAI`：POST 同源 `/ai/v1/chat/completions`（**非流式**，图表代码整段返回）
- 请求体：`model` 取 `import.meta.env.MERMAID_AI_MODEL`；`messages` 为 [system, user]
- system prompt（英文，要求模型作为 Mermaid 图表编辑器）：只返回完整的更新后 Mermaid 代码，无解释、无 markdown 围栏
- user message：当前代码（围栏包裹）+ 用户指令
- 响应解析：`parseAIContent` 剥离可能出现的 ` ```mermaid ` / ` ``` ` 围栏及前后空白
- 非 200 或内容为空 → 抛出携带网关错误信息的 `Error`

## 3. UI 接线

- **`DesktopEditor.svelte`** 的 `onTryFree` 处理器：
  - `aiEnabled` 时调用 `editDiagramWithAI(input, currentText)`；成功 → `onUpdate(newCode)`（经 `Editor.svelte` 的 onUpdate → `updateCode()` 走既有状态管线：mermaid.parse 校验 → localStorage 持久化 → `validatedState` 发布 → 图表重渲染）→ `closePopup()`
  - 未启用 → 保持现有 mermaid.ai 跳转行为，零破坏
- **`AIPromptPopup.svelte`**：
  - AI 模式下按钮文案 "Try free" → "Generate"，底部 "Sign Up at Mermaid.ai to try AI" 营销文案隐藏
  - 请求中：按钮转 loading 态并禁用，弹窗不可关闭（防止结果写入已关闭的上下文后状态错乱）
  - 新增 `loading` prop 由父组件控制

## 4. 错误处理

- 请求失败 / 返回异常：`src/lib/util/notify.ts` 的 `notify()` toast 报错，弹窗保留用户输入不丢失，loading 复位

## 5. 测试计划

1. **单元测试** `src/lib/util/ai.test.ts`：`parseAIContent` 用例——有/无围栏、带 `mermaid` 语言标记、纯围栏、前后空白、空串
2. **代理链路**：curl `http://localhost:3000/ai/v1/chat/completions` 验证转发与鉴权
3. **浏览器端到端**：`/edit` 页面 → 行旁 AI 图标 → 输入指令（如"给流程图加一个审批节点"）→ 验证代码更新、图表重渲染
4. `pnpm check`（svelte-check）与 `pnpm lint`（prettier + eslint）通过

## 6. 范围边界

- 只接桌面端 Monaco 编辑器的 AI 弹窗这一个入口；`EnhancedEditsButton`、语法错误栏的 "AI Repair" 按钮保持 mermaid.ai 跳转原样
- 不做流式输出
- 不做生产 nginx 代理配置（**后续项**：静态部署若需 AI，在 nginx.conf 加 `/ai/` location 反代网关并注入鉴权头）
- 移动端编辑器（`MobileEditor`）本无 AI 弹窗，不涉及
- `.env.local` 不入库，密钥仅存本地
