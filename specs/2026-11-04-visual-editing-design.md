# 设计：可视化编辑（选中 / 改标签 / 移动）

日期：2026-11-04
状态：已批准

关联：AI 编辑条（已合并，`AIEditBar`/`editDiagramWithAI`）、图表存储（已合并，`/api/diagrams`、SQLite）。对标 mermaid.ai Visual Editor 的可观测交互。

## 需求确认记录

- 范围 B：单击选中（SVG 高亮 + 代码定位 + AI 上下文）+ 双击改标签（写回代码）+ 拖拽移动（位置 overlay 持久化）
- 位置持久化：SQLite `diagrams.positions` 列（JSON），拖拽结束即 PATCH；未保存图位置仅内存
- 不做：多选/框选、连线选中与拖拽建边（C 档）、URL 携带位置

## 交互模型

| 操作            | 行为                                                                                       |
| --------------- | ------------------------------------------------------------------------------------------ |
| 单击节点        | 选中：SVG 高亮描边；Monaco 跳转高亮该节点定义行；AI 输入条显示引用 chip `⊙ A`（可 × 清除） |
| 双击节点        | 原地输入框改标签 → Enter 写回代码 → 重渲染                                                 |
| 按住节点拖动    | 移动该节点（抑制画布平移）；松手后 transform 保留并保存坐标                                |
| 空白拖动 / 滚轮 | 画布平移缩放（现状不变）                                                                   |
| Esc / 单击空白  | 取消选中（清 chip、去高亮）                                                                |

## 1. 节点识别与代码定位

- mermaid 渲染的节点 SVG `id` 格式：`${viewId}-${diagramType}-${nodeName}-${counter}`（实证：`graph-1-flowchart-A-0`）。`viewId` 由 View 生成（`uniqueID('graph-')`）并传入解析器；剥离前缀与 `-\d+$` 后缀得 `nodeName`
- 选择器：`svg .node`（flowchart/class/state 等）+ `svg .actor`（sequence）；点击命中向上找最近匹配元素；未命中按空白处理
- 代码定位：按 `nodeName` 正则匹配定义行（`/(^|\W)nodeName[\[({"']/` 类似模式，命中首个非引用行）→ `editor.revealLineInCenter(lineNumber)` + `setSelection` 整行
- 解析与定位函数为纯函数放 `src/lib/util/diagramNodes.ts`（单测覆盖）

## 2. 跨组件桥 `src/lib/util/selectedNode.svelte.ts`

模块状态（模式同 `currentDiagram.svelte.ts`）：`selectedNode`（`{name, lineText} | null` 的 getter）+ `setSelectedNode` / `clearSelectedNode`。`View` 写入；`DesktopEditor` 与 `AIEditBar` 读取响应。

## 3. 位置 overlay

- 数据：`{[nodeName: string]: {dx: number, dy: number}}`（diagram 坐标系，在 pan/zoom 变换之内应用，随缩放平移天然正确）
- 应用：`View.svelte` 的 `handleStateChange` 在 `renderAndPlaceDiagram` 返回 `graphDiv` 后，对每个有 offset 的节点 `g` 设 `transform: translate(dx,dy)`（重放）；纯位置变化不改 code/config → View 现有去重不触发重渲染
- 拖拽：节点 mousedown 记录起点 → mousemove 更新 `transform`（实时）→ mouseup 计算 dx/dy 累加进 overlay 并持久化
- panZoom 协调：拖节点期间 `PanZoomState` 暴露 `isNodeDragging` 标志，`panzoom` 初始化时 `beforePan` 回调返回 false 抑制画布平移；hammerjs 触摸路径在节点上 `stopPropagation`
- 持久化：mouseup 时若 `currentDiagram.diagram` 存在 → `updateDiagram(id, {positions})`（即时 PATCH，失败 toast）；否则仅内存。SaveDialog 首次保存（POST）时附带当前 positions
- `/view` 只读页：加载 diagram 的 positions 并重放（复用同一重放工具函数）
- AI/代码变更导致重渲染 → 重放使位置保持（节点名消失则忽略该坐标）

## 4. 双击改标签

- 双击节点 → 在 View 容器内按节点 SVG 的 `getBoundingClientRect` 绝对定位一个 `<input>`（初值=当前节点文本）→ Enter 提交 / Esc 取消 / 失焦提交
- 写回：`src/lib/util/diagramNodes.ts` 的 `replaceNodeLabel(code, nodeName, newLabel)`——支持 flowchart 括号语法（`A[label]` / `A(label)` / `A{label}` / `A[("label")]`）与 sequence `participant A as Label`；成功后 `updateCode(newCode)`
- 不支持的语法 → `notify` 提示"Label editing not supported here, try the AI bar"
- 替换不改变节点名与连边，仅 label 文本；重渲染后位置重放保持

## 5. AI 上下文（AIEditBar 增强）

- `selectedNode` 存在时：输入条右侧显示 chip `⊙ {name}`（点击 × 调 `clearSelectedNode`）
- 提交时指令自动附加：`\n\nFocus on node {name}, currently: {lineText}`（用户可见的输入框内容不变，拼接发生在 submit 内）
- 清除选中（Esc/点空白/chip ×）后不带上下文

## 6. API / DB 变更

- `server/db.mjs`：启动时 `ALTER TABLE diagrams ADD COLUMN positions TEXT`（try/catch 忽略已存在）；新建表语句同步加列；`createDiagram`/`updateDiagram`/`getDiagram`/导出导入 支持 `positions`（存 JSON 字符串，读时解析为对象，NULL → `{}`）
- `server/index.mjs`：POST/PATCH 校验 positions 为对象（值为 `{dx,dy}` 数字对）
- 前端 `api.ts`：`Diagram` 增加 `positions: Record<string, {dx: number; dy: number}>`；`createDiagram`/`updateDiagram` 的请求类型增加 `positions`
- 导出/导入随行携带 positions（既有事务逻辑不变）

## 7. 测试计划

1. 单测（`diagramNodes.test.ts`）：节点 id 解析（含节点名带连字符）、定义行定位、label 替换（各括号语法/sequence/不支持返回 null）；`selectedNode` 模块行为
2. Playwright E2E：单击节点 → SVG 高亮 + 编辑器行选中；双击改标签 → 代码与图更新；拖动节点 → 保存 → 刷新 → 位置保持；选中后 AI 提交 → 指令含 "Focus on node"；Esc 清除
3. 服务端冒烟：positions 列迁移（旧库启动不炸）、PATCH positions、导出导入往返
4. `pnpm check && pnpm lint`

## 8. 范围边界

- 单选；连线不可交互；不做框选/多选/对齐吸附
- 改标签覆盖 flowchart + sequence 常见语法，其余降级提示
- 位置不随 URL hash 分享（仅服务端保存的图）
- 移动端：桌面优先——鼠标路径完整支持选中/拖拽/双击；触摸拖拽为后续项（hammer 触摸协调未实现，触摸路径下画布平移照旧）。决策：2026-11-04 用户裁定桌面优先。
