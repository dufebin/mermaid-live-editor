# 设计：图表下方 AI 编辑输入条（子项目 2）

日期：2026-11-04
状态：已批准

关联：子项目 1（图表存储 + Dashboard，已完成合并）。复用其前置的 AI 网关集成（`src/lib/util/ai.ts`、vite `/ai` 代理、deepseek-v4.1-flash）。

## 背景与目标

编辑器现有 AI 入口是桌面端 Monaco 行锚定弹窗（AIPromptPopup），移动端无任何 AI 入口。对标 mermaid.ai 的"图表下方 AI 输入条"体验：一个常驻输入条，输入自然语言指令直接修改当前图表。

需求确认记录：

- 布局：底部同行共存（AI 输入条与左下手绘开关、右下版本/隐私工具栏同处一行，正常文档流）
- 保留行锚定弹窗；本输入条为全图编辑主入口，移动端可见（成为移动端唯一 AI 入口）

## 1. 组件 `src/lib/components/AIEditBar.svelte`

- 自包含组件（无 props；接受外部 `class` 透传不必须）
- 内部状态：`input`（textarea 双向绑定）、`isLoading`
- textarea：单行起步、自适应至多 3 行；Enter 提交（Shift+Enter 换行）；placeholder `"Describe changes to this diagram…"`；loading 时禁用
- Send 按钮（`variant="accent"`）：loading 时文案 "Editing…" 并禁用；空输入禁用
- 仅在 `env.aiEnabled` 为 true 时由父级渲染（组件不重复门控）

## 2. 数据流（复用既有管线）

```
提交 → editDiagramWithAI(input, validatedState.current.code)
     → updateCode(newCode)        // 既有状态管线：校验→localStorage 持久化→URL hash→重渲染
     → 清空输入；图表变化即反馈（无成功 toast）
失败 → notify(错误信息)（既有模式）；输入保留；loading 复位（finally）
```

`editDiagramWithAI` 已有 60s 超时、网关错误信息携带、围栏剥离（含单测）。

## 3. 页面接线（edit 页图表栏底部行）

`src/routes/(app)/edit/+page.svelte` 图表栏结构改为：

```
<View … />                                  // flex-1 占满
<div class="flex items-center gap-2 p-2">   // 新增底部行（正常文档流）
  <SyncRoughToolbar />                      // 从 absolute bottom-0 left-0 移入
  {#if env.aiEnabled}<AIEditBar class="flex-1" />{/if}
  <VersionSecurityToolbar />                // 从 absolute bottom-0 right-0 移入
</div>
```

无 AI 时底部行仍生效（两工具栏同行，输入条缺席），布局无回归。

## 4. TID 常量

`aiEditBar: 'ai-edit-bar'`、`aiEditInput: 'ai-edit-input'`、`aiEditSend: 'ai-edit-send'`（加入 `src/lib/constants.ts`，保持字母序）。

## 5. 测试计划

1. `pnpm check && pnpm lint`
2. Playwright E2E（真实事件，复用既有脚本模式）：`/edit` → 输入条可见 → 输入 "Add a node called Test" → Send → 编辑器代码含 Test、渲染图出现该节点 → 底部行布局截图核对
3. 无新增单测（核心逻辑 `editDiagramWithAI` 已有 8 个单测；组件为纯展示接线）

## 6. 范围边界

- 不做：多轮对话历史、流式输出、专门的"生成新图"入口（空代码场景由 editDiagramWithAI 自然覆盖）
- 不改动 AIPromptPopup / DesktopEditor（弹窗入口保持现状）
- 移动端不额外适配（弹性布局自然生效）
