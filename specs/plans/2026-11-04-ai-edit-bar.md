# AI 编辑输入条实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在编辑器图表区底部添加常驻 AI 输入条，输入自然语言指令经本地网关修改当前图表。

**Architecture:** 单个自包含组件 `AIEditBar.svelte`（textarea + Send），提交时调用既有的 `editDiagramWithAI(instruction, currentCode)` 并把结果经 `updateCode()` 写入既有状态管线（校验→持久化→URL hash→重渲染）。页面接线把图表栏底部两个绝对定位工具栏（SyncRoughToolbar / VersionSecurityToolbar）改为正常文档流的底部行，输入条居中弹性填充。

**Tech Stack:** SvelteKit 2 + Svelte 5 runes、既有 `src/lib/util/ai.ts`（8 个单测覆盖）、Playwright（E2E）。规格：`specs/2026-11-04-ai-edit-bar-design.md`。

## Global Constraints

- ESLint 强制 `src/` 下 5+ 键对象按字母序排 key；`pnpm lint:fix` 处理 import 顺序
- `docs/` 是构建输出目录，绝不编辑；文档放 `specs/`
- dev server（:3000）与存储服务（:18790）由 controller 管理，子代理不得 kill 或启动
- TID 常量按字母序插入 `src/lib/constants.ts`
- 提交信息以 `Co-Authored-By: Claude Code <noreply@anthropic.com>` 结尾
- 参考：`src/lib/components/AIPromptPopup.svelte`（既有 AI 输入 UI 风格）、`src/routes/(app)/edit/+page.svelte:200-207`（图表栏现结构）

---

### Task 1: AIEditBar 组件与 TID 常量

**Files:**

- Create: `src/lib/components/AIEditBar.svelte`
- Modify: `src/lib/constants.ts`（TID 增加 3 键）

**Interfaces:**

- Consumes: `editDiagramWithAI(instruction: string, currentCode: string): Promise<string>`（`$/util/ai`，抛 Error 带网关信息）；`updateCode(code: string, opts?): void`、`validatedState.current.code: string`（`$/util/state.svelte`）；`notify(message: string): void`（`$/util/notify`）
- Produces: `AIEditBar`（无 props；根元素自带 `flex-1` 弹性布局；data-testid 为 `ai-edit-bar`/`ai-edit-input`/`ai-edit-send`）

- [ ] **Step 1: 创建 `src/lib/components/AIEditBar.svelte`**

```svelte
<script lang="ts">
  import { Button } from '$/components/ui/button';
  import { TID } from '$/constants';
  import { editDiagramWithAI } from '$/util/ai';
  import { notify } from '$/util/notify';
  import { updateCode, validatedState } from '$/util/state.svelte';
  import SendIcon from '~icons/material-symbols/send-rounded';

  let input = $state('');
  let isLoading = $state(false);
  let textarea: HTMLTextAreaElement | undefined = $state();

  // Single line by default, auto-grow up to ~3 lines.
  const resizeTextarea = () => {
    if (!textarea) {
      return;
    }
    textarea.style.height = 'auto';
    textarea.style.height = `${Math.min(textarea.scrollHeight, 66)}px`;
  };

  const submit = async () => {
    const instruction = input.trim();
    if (!instruction || isLoading) {
      return;
    }
    isLoading = true;
    try {
      const newCode = await editDiagramWithAI(instruction, validatedState.current.code);
      updateCode(newCode);
      input = '';
      textarea?.focus();
    } catch (error) {
      notify(error instanceof Error ? error.message : 'AI request failed');
    } finally {
      isLoading = false;
    }
  };
</script>

<div class="flex min-w-0 flex-1 items-center gap-2" data-testid={TID.aiEditBar}>
  <textarea
    bind:this={textarea}
    bind:value={input}
    class="font-recursive h-9 min-w-0 flex-1 resize-none rounded-md border border-border bg-transparent px-2 py-1.5 text-sm text-foreground placeholder:text-muted-foreground focus:ring-1 focus:ring-accent focus:outline-none disabled:opacity-50 dark:border-border-dark dark:text-foreground"
    data-testid={TID.aiEditInput}
    disabled={isLoading}
    placeholder="Describe changes to this diagram…"
    rows="1"
    oninput={resizeTextarea}
    onkeydown={(event) => {
      if (event.key === 'Enter' && !event.shiftKey) {
        event.preventDefault();
        void submit();
      }
    }} />
  <Button
    variant="accent"
    size="sm"
    class="shrink-0"
    data-testid={TID.aiEditSend}
    disabled={isLoading || !input.trim()}
    onclick={() => void submit()}>
    {#if isLoading}
      Editing…
    {:else}
      <SendIcon class="size-4" /> Edit
    {/if}
  </Button>
</div>
```

注：若 `~icons/material-symbols/send-rounded` 报不可解析（`pnpm check` 会报），换成集合里存在的最接近图标（如 `play-arrow-rounded`），并在报告中记录替换。

- [ ] **Step 2: TID 常量（`src/lib/constants.ts`，按字母序插在 `aiHelpText` 之前）**

```ts
  aiEditBar: 'ai-edit-bar',
  aiEditInput: 'ai-edit-input',
  aiEditSend: 'ai-edit-send',
```

- [ ] **Step 3: 检查**

Run: `pnpm check && pnpm lint`
Expected: 无错误（组件暂无消费方，svelte-check 通过；lint:fix 处理 import 顺序）

- [ ] **Step 4: 提交**

```bash
git add src/lib/components/AIEditBar.svelte src/lib/constants.ts
git commit -m "feat: AI edit bar component below diagram

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 2: 编辑器页面接线（底部行）

**Files:**

- Modify: `src/routes/(app)/edit/+page.svelte`（图表栏 ~200-207 行）

**Interfaces:**

- Consumes: Task 1 的 `AIEditBar`；`env.aiEnabled`（`$/util/env`，edit 页已 import `env`）
- Produces: 图表区底部行为正常文档流；AI 启用时输入条常驻

- [ ] **Step 1: imports 增加（放其他组件 import 旁，顺序由 lint:fix 修）**

```ts
import AIEditBar from '$/components/AIEditBar.svelte';
```

- [ ] **Step 2: 图表栏底部改造**

把（当前 ~203-207 行）：

```svelte
<div class="absolute right-0 bottom-0"><VersionSecurityToolbar /></div>
<div class="absolute bottom-0 left-0 sm:left-5"><SyncRoughToolbar /></div>
```

替换为（放在 `View` 与顶部两个 absolute 工具栏之后）：

```svelte
<div class="flex items-center justify-between gap-2 p-2">
  <SyncRoughToolbar />
  {#if env.aiEnabled}
    <AIEditBar />
  {/if}
  <VersionSecurityToolbar />
</div>
```

要点：`justify-between` 使无 AI 时两工具栏仍分居左右两端（与原角落布局近似）；AIEditBar 根元素 `flex-1` 填充中间。顶部两个 absolute 工具栏（EnhancedEditsButton、PanZoomToolbar）不动。

- [ ] **Step 3: 检查**

Run: `pnpm check && pnpm lint`
Expected: 无错误

- [ ] **Step 4: 提交**

```bash
git add "src/routes/(app)/edit/+page.svelte"
git commit -m "feat: wire AI edit bar into diagram pane bottom row

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 3: 端到端验证（controller 执行）

**Files:**

- 无新增入库文件（Playwright 脚本放 SDD 工作区，gitignored）

**Interfaces:**

- Consumes: 前序任务产出 + 运行中的 dev server

- [ ] **Step 1: 全量单元测试**

Run: `pnpm vitest run --dir src`
Expected: 179/179 PASS（无新增单测，确认无回归）

- [ ] **Step 2: Playwright E2E（真实事件）**

脚本要点（执行时写入工作区）：`/edit` → `[data-testid="ai-edit-bar"]` 可见 → 填 `[data-testid="ai-edit-input"]` 指令 `Add a new node called TestResult` → 点 `[data-testid="ai-edit-send"]` → 等待 `#editor .view-lines` 文本含 `TestResult` → 渲染图出现该节点 → 输入框已清空 → 截图核对底部行布局（SyncRough 左、输入条中、VersionSecurity 右）

- [ ] **Step 3: 最终检查**

Run: `pnpm check && pnpm lint`
Expected: 通过

---

## Self-Review 记录

- **规格覆盖**：组件（Task 1）、数据流（Task 1 submit 函数：editDiagramWithAI → updateCode → 清空/notify/finally 复位）、页面接线与底部行（Task 2）、TID（Task 1）、测试计划三项（Task 1-2 check/lint、Task 3 E2E+全量单测）✓；无成功 toast、aiEnabled 门控在父级 ✓
- **占位符**：Task 3 Step 2 为 controller 执行时脚本（既有模式）；其余步骤含完整代码 ✓
- **类型一致性**：`editDiagramWithAI(instruction, currentCode)` / `updateCode(code)` / `validatedState.current.code` / TID 三键与 Task 1 产出一致 ✓
