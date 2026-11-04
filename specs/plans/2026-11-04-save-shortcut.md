# Save 快捷键 + 目录选择实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ctrl+S/Cmd+S 触发保存（未保存弹命名对话框，已保存直接 PATCH），SaveDialog 增加文件夹下拉选择。

**Architecture:** edit 页 `svelte:window onkeydown` 捕获 Ctrl/Cmd+S（`preventDefault`），复用 Save 按钮的既有分流逻辑；SaveDialog 打开时拉取文件夹列表（`listFolders`），原生 `<select>`（默认"未分类"=null），POST 带 `folderId`。再保存（PATCH）不改目录（移动走 dashboard 卡片菜单，职责分离）。

**Tech Stack:** SvelteKit 2 + Svelte 5。设计：对话中已批准（Ctrl+S + 目录选择）。

## Global Constraints

- ESLint 字母序 5+ 键对象；`pnpm lint:fix` 处理 import 顺序
- dev server（:3000）/存储服务（:18790）controller 管理，子代理不得动
- **E2E/测试脚本只删自己创建的数据（按 id/名称），绝不全量 DELETE**（此前脚本全删导致用户数据丢失——教训）
- 提交以 `Co-Authored-By: Claude Code <noreply@anthropic.com>` 结尾

---

### Task 1: SaveDialog 目录选择 + Ctrl+S 快捷键

**Files:**

- Modify: `src/lib/components/Dashboard/SaveDialog.svelte`
- Modify: `src/routes/(app)/edit/+page.svelte`

**Interfaces:**

- Consumes: `listFolders(): Promise<Folder[]>`（`$/util/api`）、既有 `saveExisting`/`saveDialogOpen`/`currentDiagram`
- Produces: POST 请求体带 `folderId: string | null`

- [ ] **Step 1: SaveDialog 增加文件夹下拉**

script 增加状态与加载（`$effect` 在 open 时重置并拉取，失败静默为空列表）：

```ts
import { createDiagram, listFolders, type Folder } from '$/util/api';
let folders = $state<Folder[]>([]);
let folderId = $state<string | null>(null);

$effect(() => {
  if (open) {
    name = '';
    error = '';
    folderId = null;
    void listFolders()
      .then((list) => {
        folders = list;
      })
      .catch(() => {
        folders = [];
      });
  }
});
```

`createDiagram` 调用带 `folderId`（字母序插在 `name` 前）：

```ts
const diagram = await createDiagram({
  folderId,
  name: trimmed,
  positions: nodePositions.value,
  state: validatedState.current.serialized
});
```

模板（名称 Input 所在的 flex 容器之后、错误信息之后、Dialog.Footer 之前）：

```svelte
<select
  bind:value={folderId}
  class="h-9 w-full rounded-md border border-border bg-transparent px-2 text-sm text-foreground focus:ring-1 focus:ring-accent focus:outline-none dark:border-border-dark"
  data-testid={TID.saveDialogFolderSelect}
  disabled={saving}>
  <option value={null}>Uncategorized</option>
  {#each folders as folder (folder.id)}
    <option value={folder.id}>{folder.name}</option>
  {/each}
</select>
```

- [ ] **Step 2: TID 常量**

`constants.ts` 字母序插入 `saveDialogFolderSelect: 'save-dialog-folder-select',`（`saveDialogInput` 之后、`saveButton` 之前）。

- [ ] **Step 3: edit 页 Ctrl+S**

把 Save 按钮的 onclick 逻辑提为函数（按钮 onclick 改用之）：

```ts
const onSave = () => {
  if (currentDiagram.diagram) {
    void saveExisting();
  } else {
    saveDialogOpen = true;
  }
};
```

页面底部（或已有 `<svelte:window>` 合并）：

```svelte
<svelte:window
  onkeydown={(event) => {
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
      event.preventDefault();
      onSave();
    }
  }} />
```

（Mac metaKey / Win/Linux ctrlKey 都覆盖；注意 `event.key` 在 Ctrl+S 下为 's' 或 'S'，用 `toLowerCase()` 匹配。）

- [ ] **Step 4: 检查与提交**

Run: `pnpm check && pnpm lint`
Expected: 无错误

```bash
git add src/lib/components/Dashboard/SaveDialog.svelte "src/routes/(app)/edit/+page.svelte" src/lib/constants.ts
git commit -m "feat: Ctrl+S save shortcut with folder selection in save dialog

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 2: controller 验证（E2E + 重启数据安全）

- [ ] **Step 1: Playwright E2E**：`/edit` → Ctrl+S（`page.keyboard.press('Control+s')`）→ 对话框出现 → 若无文件夹先建一个（API，名称带测试标记）→ 选择它 → 命名保存 → API 确认 `folderId` 为所选值 → 再 Ctrl+S（已保存路径：不弹框）→ **清理只删本测试创建的 diagram/folder（按名称前缀过滤，绝不全量删）**
- [ ] **Step 2: 重启数据安全**：记录当前 folders/diagrams → 重启存储服务（kill + 后台重启）→ 数据完全一致；`pnpm vitest run --dir src` 全过 + `pnpm check && pnpm lint`
