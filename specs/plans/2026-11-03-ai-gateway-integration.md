# AI 网关接入实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 mermaid-live-editor 的 AIPromptPopup 接到本地 OpenAI 兼容网关（deepseek-v4.1-flash），实现"编辑当前图表"，并完成单元测试与端到端验证。

**Architecture:** 浏览器请求同源路径 `/ai/v1/chat/completions`，由 Vite dev/preview 代理转发到网关（绕过 CORS），鉴权头在代理层注入（密钥不进前端 bundle）。新模块 `src/lib/util/ai.ts` 封装 LLM 调用，`DesktopEditor.svelte` 的 `onTryFree` 改为调用它并把结果经既有 `onUpdate` → `updateCode()` 状态管线写回。

**Tech Stack:** SvelteKit 2 + Svelte 5 (runes)、Vite 8、vitest（jsdom）、Monaco。规格文档：`specs/2026-11-03-ai-gateway-integration-design.md`。

## Global Constraints

- Node ≥ 24.16.0，pnpm 10.34.9（corepack 自动锁定）
- `docs/` 是构建输出目录，**绝不编辑**；文档放 `specs/`
- `envPrefix: 'MERMAID_'`：`MERMAID_` 前缀变量会进客户端 bundle → **密钥变量必须用无前缀名**（`AI_PROXY_TARGET` / `AI_PROXY_KEY`），只在 vite.config.js 读取
- ESLint 强制 `src/` 下 5+ 键对象按字母序排 key（sort-keys）
- 应用 env 读取统一走 `src/lib/util/env.ts`（CLAUDE.md 约定）
- HMR 被禁用（整页刷新属正常）；改 `vite.config.js` / `.env.local` 后必须重启 dev server
- `.env.local` 已被 gitignore（.gitignore 第 3 行），真实密钥只写入该文件，绝不提交
- 提交信息以 `Co-Authored-By: Claude Code <noreply@anthropic.com>` 结尾

---

### Task 1: AI 模块与环境变量（TDD）

**Files:**

- Modify: `src/lib/util/env.ts`
- Create: `src/lib/util/ai.ts`
- Test: `src/lib/util/ai.test.ts`

**Interfaces:**

- Consumes: 无（首个任务）
- Produces:
  - `env.aiEnabled: boolean`、`env.aiModel: string`（env.ts 导出对象新增字段）
  - `ai.ts` 导出：`aiEnabled: boolean`；`parseAIContent(raw: string): string`；`editDiagramWithAI(instruction: string, currentCode: string): Promise<string>`

- [ ] **Step 1: 写失败测试**

创建 `src/lib/util/ai.test.ts`：

````ts
import { describe, expect, it } from 'vitest';
import { parseAIContent } from './ai';

describe('parseAIContent', () => {
  it('returns plain content unchanged, trimmed', () => {
    expect(parseAIContent('  flowchart TD\n    A --> B  ')).toBe('flowchart TD\n    A --> B');
  });

  it('strips ```mermaid fences', () => {
    expect(parseAIContent('```mermaid\nflowchart TD\n    A --> B\n```')).toBe(
      'flowchart TD\n    A --> B'
    );
  });

  it('strips bare ``` fences', () => {
    expect(parseAIContent('```\nflowchart TD\n    A --> B\n```')).toBe('flowchart TD\n    A --> B');
  });

  it('returns empty string for empty input', () => {
    expect(parseAIContent('')).toBe('');
  });

  it('keeps content when a fence does not span the whole string', () => {
    const raw = 'Here is the diagram:\n```mermaid\nflowchart TD\n```';
    expect(parseAIContent(raw)).toBe(raw.trim());
  });
});
````

- [ ] **Step 2: 运行测试确认失败**

Run: `pnpm vitest run src/lib/util/ai.test.ts`
Expected: FAIL（`Failed to resolve import "./ai"` 或模块不存在）

- [ ] **Step 3: 实现**

`src/lib/util/env.ts` 的 `env` 对象**顶部**（保持字母序）加两个字段：

```ts
export const env = {
  aiEnabled: import.meta.env.MERMAID_AI_ENABLED === 'true',
  aiModel: import.meta.env.MERMAID_AI_MODEL ?? '',
  analyticsUrl: import.meta.env.MERMAID_ANALYTICS_URL ?? '',
  // ...其余字段不动
```

创建 `src/lib/util/ai.ts`：

````ts
import { env } from './env';

const SYSTEM_PROMPT = [
  'You are a Mermaid diagram editor.',
  'The user provides the current Mermaid diagram code and an instruction.',
  'Apply the instruction to the diagram and return ONLY the complete updated Mermaid diagram code.',
  'Do not include any explanations, comments about your changes, or markdown code fences.'
].join(' ');

export const aiEnabled = env.aiEnabled;

export const parseAIContent = (raw: string): string => {
  const trimmed = raw.trim();
  const fenceMatch = trimmed.match(/^```(?:mermaid)?\s*([\s\S]*?)```$/);
  return (fenceMatch ? (fenceMatch[1] ?? '') : trimmed).trim();
};

export const editDiagramWithAI = async (
  instruction: string,
  currentCode: string
): Promise<string> => {
  const response = await fetch('/ai/v1/chat/completions', {
    body: JSON.stringify({
      messages: [
        { role: 'system', content: SYSTEM_PROMPT },
        {
          content: `Current diagram code:\n\n\`\`\`\n${currentCode}\n\`\`\`\n\nInstruction: ${instruction}`,
          role: 'user'
        }
      ],
      model: env.aiModel
    }),
    headers: { 'Content-Type': 'application/json' },
    method: 'POST'
  });

  if (!response.ok) {
    throw new Error(`AI request failed (${response.status})`);
  }

  const data = (await response.json()) as {
    choices?: { message?: { content?: string } }[];
  };
  const content = data.choices?.[0]?.message?.content ?? '';
  if (!content.trim()) {
    throw new Error('AI returned an empty response');
  }
  return parseAIContent(content);
};
````

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm vitest run src/lib/util/ai.test.ts`
Expected: PASS（5 个用例全过）

- [ ] **Step 5: 提交**

```bash
git add src/lib/util/ai.ts src/lib/util/ai.test.ts src/lib/util/env.ts
git commit -m "feat: add AI module calling local OpenAI-compatible gateway

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 2: Vite 代理与本地配置

**Files:**

- Modify: `vite.config.js`
- Create: `.env.local`（gitignored，不入库）

**Interfaces:**

- Consumes: 无
- Produces: dev/preview server 上生效的同源代理路径 `/ai/*` → `${AI_PROXY_TARGET}/*`，代理请求带 `Authorization: Bearer ${AI_PROXY_KEY}` 头

- [ ] **Step 1: 修改 vite.config.js**

整体从对象形式改为函数形式（其余内容原样保留），顶部 import 增加 `loadEnv`：

```js
import { defineConfig, loadEnv } from 'vite';
```

`export default defineConfig(...)` 改为：

```js
export default defineConfig(({ mode }) => {
  const serverEnv = loadEnv(mode, process.cwd(), '');
  const aiProxy = serverEnv.AI_PROXY_TARGET
    ? {
        '/ai': {
          changeOrigin: true,
          headers: { Authorization: `Bearer ${serverEnv.AI_PROXY_KEY ?? ''}` },
          rewrite: (path) => path.replace(/^\/ai/, ''),
          target: serverEnv.AI_PROXY_TARGET
        }
      }
    : undefined;

  return {
    plugins: [
      tailwindcss(),
      sveltekit(),
      Icons({
        compiler: 'svelte',
        customCollections: {
          custom: FileSystemIconLoader('./static/icons')
        }
      }),
      alwaysFullReload,
      devtoolsJson()
    ],
    envPrefix: 'MERMAID_',
    server: { host: true, port: 3000, proxy: aiProxy },
    preview: { host: true, port: 3000, proxy: aiProxy },
    // Vitest otherwise resolves Svelte's server build, where $effect is a no-op.
    resolve: process.env.VITEST ? { conditions: ['browser'] } : undefined,
    test: {
      environment: 'jsdom',
      // in-source testing
      includeSource: ['src/**/*.{js,ts,svelte}'],
      // Ignore E2E tests
      exclude: [
        'tests/**/*',
        '**/node_modules/**',
        '**/dist/**',
        '**/.{idea,git,cache,output,temp}/**',
        '**/{karma,rollup,webpack,vite,vitest,jest,ava,babel,nyc,cypress,tsup,build,eslint,prettier}.config.*'
      ],
      setupFiles: ['./src/tests/setup.ts'],
      coverage: {
        exclude: ['src/mocks', '.svelte-kit', 'src/**/*.test.ts'],
        reporter: ['text', 'json', 'html', 'lcov']
      }
    }
  };
});
```

（`alwaysFullReload` 插件定义保持在 `export default` 之前，原样不动。）

- [ ] **Step 2: 创建 .env.local**

```bash
MERMAID_AI_ENABLED='true'
MERMAID_AI_MODEL='deepseek-v4.1-flash'
AI_PROXY_TARGET='http://127.0.0.1:18780'
AI_PROXY_KEY='<执行时填入用户提供的网关 key：300001934:…>'
```

- [ ] **Step 3: 重启 dev server 并验证代理**

停掉现有后台 dev server（TaskStop 或 kill），重新 `pnpm dev`（后台），等待 ready 后：

```bash
curl -s http://localhost:3000/ai/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"say ok"}],"max_tokens":10}'
```

Expected: HTTP 200，JSON 含 `"content":"ok"`（证明代理转发 + 鉴权注入成功；无需带 Authorization 头，代理已注入）

- [ ] **Step 4: 提交（仅 vite.config.js）**

```bash
git add vite.config.js
git commit -m "feat: proxy /ai to local LLM gateway in dev/preview servers

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 3: AIPromptPopup 加载态与文案

**Files:**

- Modify: `src/lib/components/AIPromptPopup.svelte`

**Interfaces:**

- Consumes: `env.aiEnabled: boolean`、`env.aiModel: string`（Task 1 产出）
- Produces: 新增可选 prop `loading?: boolean`（默认 false）；`onTryFree` 回调签名不变

- [ ] **Step 1: 修改 script 部分**

`interface Props` 与解构加入 `loading`；新增 env import：

```ts
import { env } from '$/util/env';

interface Props {
  show: boolean;
  input: string;
  loading?: boolean;
  onClose: () => void;
  onHeightChange?: (height: number) => void;
  onTryFree: () => void;
}

let {
  show,
  input = $bindable(),
  loading = false,
  onClose,
  onHeightChange,
  onTryFree
}: Props = $props();
```

- [ ] **Step 2: loading 期间阻止关闭**

`handleKeydown` 与 `handleOutsideClick` 各加 `!loading` 条件；textarea 的 Enter 提交同样加 `!loading`：

```ts
function handleKeydown(e: KeyboardEvent) {
  if (show && !loading && e.key === 'Escape') {
    onClose();
  }
}

function handleOutsideClick(e: MouseEvent) {
  if (show && !loading && container && e.target instanceof Node && !container.contains(e.target)) {
    onClose();
  }
}
```

textarea onkeydown 内：

```svelte
onkeydown={(e) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    if (input.trim() && !loading) {
      onTryFree();
    }
  }
}}
```

- [ ] **Step 3: 底部区域改为条件渲染**

把 `<div class="flex items-center justify-between">` 内的营销 span 与按钮替换为：

```svelte
<div class="flex items-center justify-between">
  {#if env.aiEnabled}
    <span class="font-recursive text-xs font-normal text-foreground dark:text-foreground"
      >Powered by {env.aiModel || 'AI'}</span>
  {:else}
    <span class="font-recursive text-xs font-normal text-foreground dark:text-foreground"
      >Sign Up at Mermaid.ai to try AI</span>
  {/if}
  <Button
    class="font-recursive h-6 w-16 gap-1.5 rounded-sm bg-accent p-1 text-xs font-medium text-white no-underline hover:bg-accent/90 hover:text-white hover:no-underline active:bg-accent/80 dark:bg-accent dark:text-white! dark:hover:bg-accent/90 dark:active:bg-accent/80"
    disabled={loading}
    onclick={onTryFree}>
    {loading ? 'Generating…' : env.aiEnabled ? 'Generate' : 'Try free'}
  </Button>
</div>
```

（规格说"营销文案隐藏"；此处替换为展示模型名，保持左右布局平衡，属等价微调。）

- [ ] **Step 4: 类型与规范检查**

Run: `pnpm check && pnpm lint`
Expected: 无错误（如 import 顺序报错，跑 `pnpm lint:fix` 后复查）

- [ ] **Step 5: 提交**

```bash
git add src/lib/components/AIPromptPopup.svelte
git commit -m "feat: AIPromptPopup loading state and AI-mode labels

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 4: DesktopEditor 接线

**Files:**

- Modify: `src/lib/components/DesktopEditor.svelte:1-16`（imports）、`:29-36`（state）、`:237-252`（AIPromptPopup 调用处）

**Interfaces:**

- Consumes: `aiEnabled`、`editDiagramWithAI(instruction, currentCode): Promise<string>`（Task 1）；`loading` prop（Task 3）；`notify(message: string): void`（`src/lib/util/notify.ts` 现有导出）
- Produces: 无（终端消费者）

- [ ] **Step 1: 加 import 与 loading 状态**

script 顶部（紧随 `import { env } from '$/util/env';` 之后，顺序不对由 `pnpm lint:fix` 修正）：

```ts
import { aiEnabled, editDiagramWithAI } from '$/util/ai';
import { notify } from '$/util/notify';
```

state 区（`let lastMouseLine = 0;` 附近）加：

```ts
let isLoading = $state(false);
```

- [ ] **Step 2: 重写 onTryFree 处理器**

把 `AIPromptPopup` 调用处（当前 243-251 行）整体替换为：

```svelte
<AIPromptPopup
  show={showPopup}
  bind:input
  loading={isLoading}
  onHeightChange={(height) => aiPromptManager.updateHeight(height)}
  onClose={closePopup}
  onTryFree={async () => {
    if (!aiEnabled) {
      logMermaidChartClick('vibeDiagramming');
      window.open(
        urls.current.mermaidChart({ medium: 'vibe_diagramming' }).save,
        '_blank',
        'noopener'
      );
      closePopup();
      return;
    }
    isLoading = true;
    try {
      const newCode = await editDiagramWithAI(input, currentText);
      onUpdate(newCode);
      closePopup();
    } catch (error) {
      notify(error instanceof Error ? error.message : 'AI request failed');
    } finally {
      isLoading = false;
    }
  }} />
```

（`onUpdate(newCode)` → `Editor.svelte` 的 `onUpdate` → `updateCode()` → 校验/持久化/渲染，全走既有管线；弹窗只在 mermaidModel 上出现，`currentText` 必为图表代码。）

- [ ] **Step 3: 类型与规范检查**

Run: `pnpm check && pnpm lint`
Expected: 无错误

- [ ] **Step 4: 提交**

```bash
git add src/lib/components/DesktopEditor.svelte
git commit -m "feat: wire AIPromptPopup to local AI gateway

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 5: 端到端验证

**Files:**

- 无新增（验证任务；发现问题则回改对应文件后补充提交）

**Interfaces:**

- Consumes: 全部前序任务产出 + 运行中的 dev server + chrome-devtools MCP 浏览器工具

- [ ] **Step 1: 全量单元测试**

Run: `pnpm vitest run --dir src`
Expected: 全部 PASS（含既有测试，确认无回归）

- [ ] **Step 2: 页面内同源 fetch 验证**

用 chrome-devtools MCP 打开 `http://localhost:3000/edit`（若遇 profile 锁错误：先 `list_pages`；仍失败则 `pkill -f chrome-devtools-mcp` 清理残留进程后重开），执行 evaluate_script：

```js
async () => {
  const r = await fetch('/ai/v1/chat/completions', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      model: 'deepseek-v4.1-flash',
      messages: [{ role: 'user', content: 'reply with exactly: ok' }],
      max_tokens: 10
    })
  });
  const data = await r.json();
  return { status: r.status, content: data.choices?.[0]?.message?.content };
};
```

Expected: `{ status: 200, content: 含 "ok" }`（证明应用源内同源请求全链路通）

- [ ] **Step 3: 真实 UI 流程验证**

在 `/edit` 页面依次：

1. `evaluate_script` 触发 Monaco 行悬浮并点击行旁 AI 图标（glyph 是 `.suggestion-icon` 装饰元素）：

```js
() => {
  const editor = document.querySelector('#editor');
  const line = editor.querySelector('.view-lines .view-line');
  const rect = line.getBoundingClientRect();
  editor.dispatchEvent(
    new MouseEvent('mousemove', { bubbles: true, clientX: rect.left + 20, clientY: rect.top + 5 })
  );
  const glyph = document.querySelector('.suggestion-icon');
  if (!glyph) return { ok: false, reason: 'glyph not found' };
  const g = glyph.getBoundingClientRect();
  editor.dispatchEvent(
    new MouseEvent('mousedown', { bubbles: true, clientX: g.left + 5, clientY: g.top + 5 })
  );
  return { ok: true };
};
```

2. `take_snapshot` 确认弹窗出现（textarea + "Generate" 按钮）
3. `fill` textarea 输入指令：`Add a new node C labeled "Review" between B and the final node`
4. `click` Generate 按钮
5. `wait_for` 等待编辑器代码出现 `Review`（或轮询 `editor.getValue()` 含 "Review"）
6. 截图确认右侧图表重渲染出新节点

Expected: 代码被替换、渲染图新增 "Review" 节点、无报错 toast

（若合成鼠标事件不可靠：降级为让用户手动点一次图标走完流程；Step 2 的 fetch 验证已覆盖网络链路。）

- [ ] **Step 4: 最终检查与收尾**

Run: `pnpm check && pnpm lint`
Expected: 通过。如中途有修复，补充提交；否则无新提交。

---

## Self-Review 记录

- **规格覆盖**：配置层（Task 2）、AI 模块（Task 1）、UI 接线（Task 3+4）、错误处理（Task 4 catch → notify）、测试计划四项（Task 1 单测 / Task 2 Step 3 代理 curl / Task 5 浏览器 E2E / 各任务 check+lint）均有对应任务 ✓
- **占位符**：仅 `.env.local` 的 key 为刻意占位（真实值只在执行时写入 gitignored 文件），其余步骤均含完整代码 ✓
- **类型一致性**：`aiEnabled`/`parseAIContent`/`editDiagramWithAI`/`loading` prop 在各任务间签名一致 ✓
