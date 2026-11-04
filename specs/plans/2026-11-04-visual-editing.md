# 可视化编辑实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 编辑器图表支持可视化编辑：单击选中（SVG 高亮 + 代码定位 + AI 上下文）、双击改标签（写回代码）、拖拽移动节点（位置 overlay 持久化到 SQLite）。

**Architecture:** 节点识别基于渲染 SVG 的 DOM（flowchart 类 `id`=`${viewId}-${diagramType}-${name}-${counter}`、sequence `name` 属性）；选中态与位置 overlay 各为一个模块级状态（`selectedNode`/`nodePositions`，模式同 `currentDiagram`）；`View.svelte` 承载交互（点击/拖拽/双击）并在每次渲染后重放位置 transform；`DesktopEditor` 响应选中跳转 Monaco；`AIEditBar` 提交时附加上下文；位置经 `PATCH /api/diagrams/:id` 持久化（新 `positions` 列）。

**Tech Stack:** SvelteKit 2 + Svelte 5 runes、Monaco、svg-pan-zoom（`beforePan` 抑制）、node:sqlite。规格：`specs/2026-11-04-visual-editing-design.md`。

**Spec 修正（controller 裁定，交付时向用户说明）**：`/view` 只读页不做位置重放——该页从 URL hash 加载状态、无 diagram id，而位置按设计不随 URL 走；位置重放仅在编辑器内生效。

## Global Constraints

- ESLint 强制 `src/` 下 5+ 键对象按字母序排 key；`pnpm lint:fix` 处理 import 顺序
- `docs/` 是构建输出目录，绝不编辑；文档放 `specs/`
- dev server（:3000）与存储服务（:18790）由 controller 管理，子代理不得 kill 或启动；服务端冒烟用临时端口 + 临时 DATA_DIR
- `server/` 零 npm 依赖、纯 ESM `.mjs`
- 提交信息以 `Co-Authored-By: Claude Code <noreply@anthropic.com>` 结尾
- 实测事实：flowchart 节点 SVG id 形如 `graph-1-flowchart-A-0`（viewId=`graph-1`）；sequence actor 无 id 但 rect 有 `name` 属性；`.env` 冲突键优先于 `.env.local`（env 修改直接改 `.env`）

---

### Task 1: diagramNodes 工具与状态模块（TDD）

**Files:**

- Create: `src/lib/util/diagramNodes.ts`
- Test: `src/lib/util/diagramNodes.test.ts`
- Create: `src/lib/util/selectedNode.svelte.ts`
- Create: `src/lib/util/nodePositions.svelte.ts`

**Interfaces:**

- Consumes: 无
- Produces（Task 3/4/5 依赖）:
  - `parseNodeId(id: string, viewId: string, diagramType: string): string | null` —— id 匹配 `${viewId}-${diagramType}-${name}-${counter}` 时返回 `name`（支持 name 含连字符/数字；不匹配返回 null）
  - `findDefinitionLine(code: string, nodeName: string): number | null` —— 返回 1-based 定义行号（首个命中行；无命中 null）
  - `replaceNodeLabel(code: string, nodeName: string, newLabel: string): string | null` —— flowchart 括号语法与 sequence participant 的精确 label 替换；不支持返回 null
  - `selectedNode`（`{ get current(): { lineText: string; name: string } | null }`）、`setSelectedNode(node)`、`clearSelectedNode()`
  - `nodePositions`（`{ get value(): Record<string, { dx: number; dy: number }>; get zoom(): number }`）、`setNodePosition(name, dx, dy)`、`setZoom(value)`、`resetNodePositions(next)`（模块内不持久化——持久化由 View 调 API 完成；zoom 由 View 的 panZoom 回调同步，供拖拽坐标换算）

- [ ] **Step 1: 写失败测试 `src/lib/util/diagramNodes.test.ts`**

```ts
import { describe, expect, it } from 'vitest';
import { findDefinitionLine, parseNodeId, replaceNodeLabel } from './diagramNodes';

describe('parseNodeId', () => {
  it('extracts node name from a flowchart-style id', () => {
    expect(parseNodeId('graph-1-flowchart-A-0', 'graph-1', 'flowchart')).toBe('A');
  });

  it('supports hyphenated and numeric node names', () => {
    expect(parseNodeId('graph-2-flowchart-my-node-3', 'graph-2', 'flowchart')).toBe('my-node');
    expect(parseNodeId('g-1-flowchart-A2-3', 'g-1', 'flowchart')).toBe('A2');
  });

  it('returns null for non-matching ids', () => {
    expect(parseNodeId('graph-1-flowchart', 'graph-1', 'flowchart')).toBeNull();
    expect(parseNodeId('other-flowchart-A-0', 'graph-1', 'flowchart')).toBeNull();
    expect(parseNodeId('graph-1-sequence-A-0', 'graph-1', 'flowchart')).toBeNull();
  });
});

describe('findDefinitionLine', () => {
  const code =
    'flowchart TD\n    A[Christmas] -->|Get money| B(Go shopping)\n    B --> C{Let me think}\n';

  it('finds the definition line (1-based)', () => {
    expect(findDefinitionLine(code, 'A')).toBe(2);
    expect(findDefinitionLine(code, 'B')).toBe(2);
    expect(findDefinitionLine(code, 'C')).toBe(3);
  });

  it('does not match node references in labels', () => {
    expect(findDefinitionLine(code, 'Christmas')).toBeNull();
  });

  it('does not match when the name is a prefix of another', () => {
    expect(findDefinitionLine(code, 'AB')).toBeNull();
  });
});

describe('replaceNodeLabel', () => {
  it('replaces square-bracket labels', () => {
    expect(replaceNodeLabel('flowchart TD\n  A[Old] --> B', 'A', 'New')).toBe(
      'flowchart TD\n  A[New] --> B'
    );
  });

  it('replaces round, curly and quoted labels', () => {
    expect(replaceNodeLabel('A(Old) --> B', 'A', 'New')).toBe('A(New) --> B');
    expect(replaceNodeLabel('A{Old} --> B', 'A', 'New')).toBe('A{New} --> B');
    expect(replaceNodeLabel('A["Old"] --> B', 'A', 'New')).toBe('A["New"] --> B');
  });

  it('replaces the label of the definition only, not edge labels', () => {
    expect(replaceNodeLabel('A[Old] -->|Old| B', 'A', 'New')).toBe('A[New] -->|Old| B');
  });

  it('replaces sequence participant aliases', () => {
    expect(
      replaceNodeLabel('sequenceDiagram\n  participant A as Old\n  A->>B: hi', 'A', 'New')
    ).toBe('sequenceDiagram\n  participant A as New\n  A->>B: hi');
  });

  it('replaces bare sequence actor declarations', () => {
    expect(replaceNodeLabel('sequenceDiagram\n  Old->>B: hi', 'Old', 'New')).toBe(
      'sequenceDiagram\n  New->>B: hi'
    );
  });

  it('escapes regex specials in the node name', () => {
    expect(replaceNodeLabel('a.b[Old] --> B', 'a.b', 'New')).toBe('a.b[New] --> B');
  });

  it('returns null when no supported syntax matches', () => {
    expect(replaceNodeLabel('graph TD\n  A --> B', 'A', 'New')).toBeNull();
  });
});
```

- [ ] **Step 2: 运行确认失败**

Run: `pnpm vitest run src/lib/util/diagramNodes.test.ts`
Expected: FAIL（模块不存在）

- [ ] **Step 3: 实现 `src/lib/util/diagramNodes.ts`**

```ts
const escapeRegExp = (text: string): string => text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

export const parseNodeId = (id: string, viewId: string, diagramType: string): string | null => {
  const prefix = `${viewId}-${diagramType}-`;
  if (!id.startsWith(prefix)) {
    return null;
  }
  const rest = id.slice(prefix.length);
  // Strip the trailing -<counter> segment added by mermaid's renderer.
  const withoutCounter = rest.replace(/-\d+$/, '');
  if (!withoutCounter || withoutCounter === rest) {
    return null;
  }
  return withoutCounter;
};

export const findDefinitionLine = (code: string, nodeName: string): number | null => {
  const pattern = new RegExp(`(^|\\W)${escapeRegExp(nodeName)}([\\[({"']|\\s*$)`, 'm');
  const match = pattern.exec(code);
  if (!match) {
    return null;
  }
  return code.slice(0, match.index).split('\n').length;
};
```

`replaceNodeLabel` 实现要求（骨架语义，正则细节以测试全绿为准，不引入第三方依赖）：

按顺序尝试五个变体（命中第一个即执行替换并返回）：

1. `A[...]`（含可选引号 `A["x"]`）：保留前缀与节点名，仅替换括号内载荷
2. `A(...)`：同上
3. `A{...}`：同上
4. `participant A as Label`（sequence 别名）：替换别名部分
5. 行首裸 actor：`(^|\n)(\s*)(A)(\s*->>|-->|--)`：替换节点名本身（此时节点名即 label）

全部未命中返回 `null`。节点名需 `escapeRegExp`。要求：仅替换**定义处**（首个命中），后续引用不动。

- [ ] **Step 4: 运行测试确认通过**

Run: `pnpm vitest run src/lib/util/diagramNodes.test.ts`
Expected: PASS（11 用例）

- [ ] **Step 5: 创建两个状态模块**

`src/lib/util/selectedNode.svelte.ts`：

```ts
export interface SelectedNode {
  lineText: string;
  name: string;
}

let current: SelectedNode | null = $state.raw(null);

export const selectedNode = {
  get current(): SelectedNode | null {
    return current;
  }
};

export const setSelectedNode = (node: SelectedNode): void => {
  current = node;
};

export const clearSelectedNode = (): void => {
  current = null;
};
```

`src/lib/util/nodePositions.svelte.ts`：

```ts
export interface NodeOffset {
  dx: number;
  dy: number;
}

let positions: Record<string, NodeOffset> = $state.raw({});
let zoom = 1;

export const nodePositions = {
  get value(): Record<string, NodeOffset> {
    return positions;
  },
  get zoom(): number {
    return zoom;
  }
};

export const setNodePosition = (name: string, dx: number, dy: number): void => {
  positions = { ...positions, [name]: { dx, dy } };
};

export const setZoom = (value: number): void => {
  zoom = value;
};

export const resetNodePositions = (next: Record<string, NodeOffset> = {}): void => {
  positions = next;
};
```

- [ ] **Step 6: 检查与提交**

Run: `pnpm check && pnpm lint`
Expected: 无错误（两个模块暂无消费方）

```bash
git add src/lib/util/diagramNodes.ts src/lib/util/diagramNodes.test.ts src/lib/util/selectedNode.svelte.ts src/lib/util/nodePositions.svelte.ts
git commit -m "feat: diagram node utilities and selection/positions state modules

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 2: positions 服务端与 API 客户端

**Files:**

- Modify: `server/db.mjs`（positions 列 + CRUD）
- Modify: `server/index.mjs`（POST/PATCH 校验与映射）
- Modify: `src/lib/util/api.ts`（Diagram 类型 + create/update）

**Interfaces:**

- Consumes: 既有 `server/` 与 `api.ts`
- Produces:
  - DB：`diagrams.positions TEXT`（JSON 字符串或 NULL；读出经映射为 `Record<string, {dx,dy}>`，NULL/损坏 → `{}`）
  - REST：`POST /api/diagrams` 与 `PATCH /api/diagrams/:id` 接受 `positions`（校验：普通对象、值均为 `{dx:number, dy:number}` 数字对，否则 400）；`GET` 返回解析后的对象；导出/导入随行携带
  - `api.ts`：`NodeOffset` 接口；`Diagram.positions: Record<string, NodeOffset>`；`createDiagram`/`updateDiagram` 请求体可选 `positions`

- [ ] **Step 1: `server/db.mjs` 修改**

1. 建表语句 diagrams 表加列 `positions TEXT`（`favorite` 之后、`created_at` 之前，可空）
2. 建表后追加幂等迁移（旧库升级）：

```js
try {
  db.exec('ALTER TABLE diagrams ADD COLUMN positions TEXT');
} catch {
  // Column already exists — ignore.
}
```

3. `createDiagram`（INSERT 带 positions）与 `updateDiagram`（`positions === undefined` 保持原值，否则写入；null 清空）增加 positions 支持
4. 导出两个辅助：`positionsToRow(positions)`（对象 → JSON 字符串，null/undefined → null）与 `rowToPositions(value)`（row 值 → 对象，NULL/JSON 损坏 → `{}`），index.mjs 的 diagramJson 使用 rowToPositions

- [ ] **Step 2: `server/index.mjs` 修改**

1. `diagramJson` 增加 `positions: rowToPositions(row.positions)`
2. POST /api/diagrams 与 PATCH 的校验函数：

```js
const isPositions = (value) =>
  value != null &&
  typeof value === 'object' &&
  !Array.isArray(value) &&
  Object.values(value).every(
    (offset) =>
      offset != null &&
      typeof offset === 'object' &&
      !Array.isArray(offset) &&
      Number.isFinite(offset.dx) &&
      Number.isFinite(offset.dy)
  );
```

两处路由：`body.positions != null && !isPositions(body.positions)` → 400 `'positions must be an object of {dx, dy} number pairs'`；合法时传 `positionsToRow(body.positions)`（PATCH 传 undefined 保持原值）3. `importAll` 的 insertDiagram 语句与参数增加 positions（`diagram.positions ? JSON.stringify(diagram.positions) : null`）

- [ ] **Step 3: `src/lib/util/api.ts` 修改**

```ts
export interface NodeOffset {
  dx: number;
  dy: number;
}
```

`Diagram` 增加 `positions: Record<string, NodeOffset>;`（字母序：`name` 与 `state` 之间）；`createDiagram` 参数类型与 `updateDiagram` patch 类型增加 `positions?: Record<string, NodeOffset>;`

- [ ] **Step 4: 冒烟验证（临时端口 + 临时 DATA_DIR）**

```bash
DATA_DIR=$(mktemp -d) PORT=28790 node server/index.mjs &  sleep 1
B=http://127.0.0.1:28790
D=$(curl -s -X POST $B/api/diagrams -H 'Content-Type: application/json' -d '{"name":"P","state":"pako:x","positions":{"A":{"dx":10,"dy":-5}}}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')
curl -s $B/api/diagrams/$D
curl -s -X PATCH $B/api/diagrams/$D -H 'Content-Type: application/json' -d '{"positions":{"A":{"dx":99,"dy":0}}}'
curl -s -X POST $B/api/diagrams -H 'Content-Type: application/json' -d '{"name":"Bad","state":"x","positions":{"A":{"dx":"no"}}}'
curl -s $B/api/export > /tmp/pos-export.json && curl -s -X POST $B/api/import -H 'Content-Type: application/json' -d @/tmp/pos-export.json
kill %1
```

Expected：GET 的 positions 为 `{"A":{"dx":10,"dy":-5}}`；PATCH 后 dx=99；非法 positions 400；导出导入往返含 positions。随后**第二次**用同一 DATA_DIR 启动再 kill——确认 ALTER 幂等不报错（旧库迁移路径）。

- [ ] **Step 5: 检查与提交**

Run: `pnpm vitest run --dir src && pnpm check && pnpm lint`
Expected: 190 全过（179+11）；无错误

```bash
git add server/db.mjs server/index.mjs src/lib/util/api.ts
git commit -m "feat: positions column with validation and API support

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 3: View 交互（选中 / 拖拽 / 双击改标签 / 重放）

**Files:**

- Modify: `src/lib/components/View.svelte`
- Modify: `src/lib/util/panZoom.ts`（`isNodeDragging` + `beforePan` 抑制）
- Modify: `src/lib/components/Dashboard/SaveDialog.svelte`（POST 附带 positions）
- Modify: `src/lib/constants.ts`（TID：`nodeLabelInput: 'node-label-input'`，字母序）

**Interfaces:**

- Consumes: Task 1 全部产出；Task 2 的 `updateDiagram`（positions）；既有 `currentDiagram`、`notify`、`updateCode`、`validatedState`、`PanZoomState`、`renderAndPlaceDiagram`（其 `viewId` 参数）
- Produces: 图表区可视化交互全套；`PanZoomState.isNodeDragging` 公共标志

- [ ] **Step 1: `panZoom.ts` 加抑制标志**

`PanZoomState` 类增加公共字段 `public isNodeDragging = false;`；`panzoom(diagramView, { ... })` 选项中新增：

```ts
      beforePan: () => !this.isNodeDragging,
```

（选项对象在类方法内构造，箭头函数捕获 `this`；返回 false 抑制画布平移。）

- [ ] **Step 2: `View.svelte` script 增加核心交互**

imports（顺序由 lint:fix 修正）：

```ts
import { updateDiagram } from '$/util/api';
import { findDefinitionLine, parseNodeId, replaceNodeLabel } from '$/util/diagramNodes';
import { currentDiagram } from '$/util/currentDiagram.svelte';
import { notify } from '$/util/notify';
import {
  nodePositions,
  resetNodePositions,
  setNodePosition,
  setZoom
} from '$/util/nodePositions.svelte';
import { clearSelectedNode, setSelectedNode } from '$/util/selectedNode.svelte';
```

组件级状态与辅助（`viewId` 提升为组件级变量：`const viewId = uniqueID('graph-');`，`renderAndPlaceDiagram` 调用处改用之；`diagramType` 既有变量）：

```ts
const NODE_SELECTOR = '.node, .actor';
let selectedSvg: SVGElement | undefined = $state();
let editingLabel:
  | { height: number; left: number; name: string; top: number; value: string; width: number }
  | undefined = $state();

const findNodeNameFromElement = (element: Element): string | null => {
  const actorName = element.getAttribute('name');
  if (actorName) {
    return actorName;
  }
  return parseNodeId(element.id, viewId, diagramType ?? '');
};

const findNodeName = (target: EventTarget | null): string | null => {
  if (!(target instanceof Element)) {
    return null;
  }
  const element = target.closest(NODE_SELECTOR);
  if (!element) {
    return null;
  }
  return findNodeNameFromElement(element);
};

const findNodeElement = (name: string): SVGElement | null => {
  for (const element of container?.querySelectorAll<SVGElement>(NODE_SELECTOR) ?? []) {
    if (findNodeNameFromElement(element) === name) {
      return element;
    }
  }
  return null;
};

const highlight = (element: SVGElement | undefined) => {
  selectedSvg?.classList.remove('visual-selected');
  element?.classList.add('visual-selected');
  selectedSvg = element;
};

// Apply saved offsets to freshly rendered nodes.
const applyPositions = () => {
  for (const [name, offset] of Object.entries(nodePositions.value)) {
    const element = findNodeElement(name);
    element?.style.setProperty('transform', `translate(${offset.dx}px, ${offset.dy}px)`);
  }
};
```

zoom 同步：把既有 `setupPanZoomObserver` 的 `onPanZoomChange` 回调改为同时调用 `setZoom(zoom ?? 1)`（保留原有 `updateCodeStore`）。

容器事件处理：

```ts
let dragState:
  { baseDx: number; baseDy: number; name: string; startX: number; startY: number } | undefined;

const onContainerMouseDown = (event: MouseEvent) => {
  const name = findNodeName(event.target);
  if (!name) {
    clearSelectedNode();
    highlight(undefined);
    return;
  }
  const element = (event.target as Element).closest(NODE_SELECTOR);
  if (!(element instanceof SVGElement)) {
    return;
  }
  const line = findDefinitionLine(code, name);
  setSelectedNode({
    lineText: (code.split('\n')[Math.max((line ?? 1) - 1, 0)] ?? '').trim(),
    name
  });
  highlight(element);
  const base = nodePositions.value[name] ?? { dx: 0, dy: 0 };
  dragState = {
    baseDx: base.dx,
    baseDy: base.dy,
    name,
    startX: event.clientX,
    startY: event.clientY
  };
};

const onContainerMouseMove = (event: MouseEvent) => {
  if (!dragState) {
    return;
  }
  panZoomState.isNodeDragging = true;
  const dx = dragState.baseDx + (event.clientX - dragState.startX) / nodePositions.zoom;
  const dy = dragState.baseDy + (event.clientY - dragState.startY) / nodePositions.zoom;
  findNodeElement(dragState.name)?.style.setProperty('transform', `translate(${dx}px, ${dy}px)`);
  setNodePosition(dragState.name, dx, dy);
};

const onContainerMouseUp = () => {
  if (!dragState) {
    return;
  }
  const { name } = dragState;
  dragState = undefined;
  panZoomState.isNodeDragging = false;
  persistPositions(name);
};

const persistPositions = (changedName: string) => {
  const diagram = currentDiagram.diagram;
  if (!diagram) {
    return; // unsaved diagram: positions stay in memory only
  }
  void updateDiagram(diagram.id, { positions: nodePositions.value }).catch((error) => {
    notify(error instanceof Error ? error.message : `Failed to save position of ${changedName}`);
  });
};

const onContainerDblClick = (event: MouseEvent) => {
  const name = findNodeName(event.target);
  if (!name) {
    return;
  }
  const element = (event.target as Element).closest(NODE_SELECTOR);
  if (!(element instanceof SVGElement)) {
    return;
  }
  const rect = element.getBoundingClientRect();
  const viewRect = view?.getBoundingClientRect();
  if (!viewRect) {
    return;
  }
  editingLabel = {
    height: Math.max(rect.height, 28),
    left: rect.left - viewRect.left,
    name,
    top: rect.top - viewRect.top,
    value: element.textContent?.trim() ?? '',
    width: Math.max(rect.width, 80)
  };
};

const submitLabel = () => {
  if (!editingLabel) {
    return;
  }
  const { name, value } = editingLabel;
  editingLabel = undefined;
  const nextCode = replaceNodeLabel(code, name, value.trim());
  if (nextCode === null) {
    notify('Label editing not supported here, try the AI bar');
    return;
  }
  updateCode(nextCode);
};

const onLabelKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Enter') {
    event.preventDefault();
    submitLabel();
  } else if (event.key === 'Escape') {
    event.preventDefault();
    editingLabel = undefined;
  }
};
```

- [ ] **Step 3: 渲染后重放 + currentDiagram 初始化**

1. `handleStateChange` 内 `renderAndPlaceDiagram` 成功且 `graphDiv` 存在时调用 `applyPositions()`
2. `$effect` 监听 `currentDiagram.diagram`：图表切换时 `resetNodePositions(diagram?.positions ?? {})`（打开其它图表后清空旧位置并载入新图的）

- [ ] **Step 4: `View.svelte` 模板修改**

```svelte
<div
  id="view"
  bind:this={view}
  class={[
    'relative h-full w-full',
    shouldShowGrid && `grid-bg-${mode.current}`,
    error && 'opacity-50'
  ]}>
  <div
    id="container"
    bind:this={container}
    class="h-full overflow-auto"
    ondblclick={onContainerDblClick}
    onmousedown={onContainerMouseDown}
    onmousemove={onContainerMouseMove}
    onmouseup={onContainerMouseUp}
    onmouseleave={onContainerMouseUp}>
  </div>
  {#if editingLabel}
    <input
      bind:value={editingLabel.value}
      class="absolute z-10 rounded-sm border-2 border-accent bg-background px-1 text-sm text-foreground shadow-lg focus:outline-none"
      data-testid={TID.nodeLabelInput}
      style="height: {editingLabel.height}px; left: {editingLabel.left}px; top: {editingLabel.top}px; width: {editingLabel.width}px"
      onblur={submitLabel}
      onkeydown={onLabelKeydown} />
  {/if}
</div>
<svelte:window
  onkeydown={(event) => {
    if (event.key !== 'Escape') {
      return;
    }
    if (editingLabel) {
      editingLabel = undefined;
      return;
    }
    clearSelectedNode();
    highlight(undefined);
  }} />
```

`<style>` 追加：

```css
:global(.visual-selected) {
  filter: drop-shadow(0 0 4px var(--color-accent, #7c3aed));
}
```

（模板需要 `import { TID } from '$/constants';`。）

- [ ] **Step 5: SaveDialog 附带 positions**

`src/lib/components/Dashboard/SaveDialog.svelte` 的 `createDiagram` 调用改为：

```ts
const diagram = await createDiagram({
  name: trimmed,
  positions: nodePositions.value,
  state: validatedState.current.serialized
});
```

（import `nodePositions` from `$/util/nodePositions.svelte`。）

- [ ] **Step 6: TID 常量**

`constants.ts` 按字母序增加 `nodeLabelInput: 'node-label-input'`（`newDiagramButton` 之后、`saveButton` 之前）。

- [ ] **Step 7: 检查与提交**

Run: `pnpm check && pnpm lint`
Expected: 无错误

```bash
git add src/lib/components/View.svelte src/lib/util/panZoom.ts src/lib/components/Dashboard/SaveDialog.svelte src/lib/constants.ts
git commit -m "feat: visual node selection, drag and inline label editing

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 4: 选中消费方（代码定位 + AI 上下文）

**Files:**

- Modify: `src/lib/components/DesktopEditor.svelte`
- Modify: `src/lib/components/AIEditBar.svelte`

**Interfaces:**

- Consumes: Task 1 的 `selectedNode`/`clearSelectedNode`/`findDefinitionLine`；既有 `currentText`（DesktopEditor）、`editDiagramWithAI`（AIEditBar）
- Produces: 无

- [ ] **Step 1: DesktopEditor 跳转高亮**

script 增加：

```ts
import { findDefinitionLine } from '$/util/diagramNodes';
import { selectedNode } from '$/util/selectedNode.svelte';
```

$effect 区增加：

```ts
$effect(() => {
  const selection = selectedNode.current;
  if (!selection || !editor) {
    return;
  }
  const lineNumber = findDefinitionLine(currentText, selection.name);
  if (!lineNumber) {
    return;
  }
  const maxColumn = editor.getModel()?.getLineMaxColumn(lineNumber) ?? 1;
  editor.revealLineInCenter(lineNumber);
  editor.setSelection(new monaco.Range(lineNumber, 1, lineNumber, maxColumn));
});
```

- [ ] **Step 2: AIEditBar 上下文 chip 与提交拼接**

script 增加 import（`selectedNode`, `clearSelectedNode`）。`submit` 内：

```ts
const selection = selectedNode.current;
const instructionWithContext = selection
  ? `${instruction}\n\nFocus on node ${selection.name}, currently: ${selection.lineText}`
  : instruction;
const newCode = await editDiagramWithAI(instructionWithContext, validatedState.current.code);
```

（成功后 `input = ''` 照旧，并追加 `clearSelectedNode()`。）

模板：Send 按钮之前插入（仅选中态）：

```svelte
{#if selectedNode.current}
  <button
    class="flex shrink-0 items-center gap-1 rounded-full border border-accent px-2 py-0.5 text-xs text-accent"
    title="Clear node reference"
    onclick={clearSelectedNode}>
    ⊙ {selectedNode.current.name} ×
  </button>
{/if}
```

- [ ] **Step 3: 检查与提交**

Run: `pnpm check && pnpm lint`
Expected: 无错误

```bash
git add src/lib/components/DesktopEditor.svelte src/lib/components/AIEditBar.svelte
git commit -m "feat: code reveal and AI context on node selection

Co-Authored-By: Claude Code <noreply@anthropic.com>"
```

---

### Task 5: 端到端验证（controller 执行）

**Files:**

- 无新增入库文件（Playwright 脚本放 SDD 工作区，gitignored）

**Interfaces:**

- Consumes: 全部前序任务 + 运行中的 dev server + 存储服务

- [ ] **Step 1: 全量单元测试**

Run: `pnpm vitest run --dir src`
Expected: 全部 PASS（含新增 11 例）

- [ ] **Step 2: Playwright E2E（真实事件）**

脚本要点（执行时写入工作区）：

1. `/edit` → 等 flowchart 渲染 → click `Christmas` 节点（`.node` 含该文本）→ 断言节点带 `visual-selected` class、Monaco 有选中行且行文本含 `A[Christmas]`、AI 输入条出现 `⊙ A` chip
2. AI 上下文：拦截 `/ai/v1/chat/completions` 请求 → 断言请求体含 `Focus on node A`
3. 双击节点 → `[data-testid="node-label-input"]` → 改值 → Enter → 编辑器代码含新 label
4. 拖动节点（mousedown/mousemove/mouseup 序列）→ Save（命名）→ reload → 从 dashboard 打开该图 → 节点带 translate transform（位置保持）
5. Esc → chip 消失、`visual-selected` 移除

- [ ] **Step 3: 最终检查**

Run: `pnpm check && pnpm lint`
Expected: 通过

---

## Self-Review 记录

- **规格覆盖**：节点识别与定位（Task 1）、selectedNode/nodePositions 模块（Task 1）、位置 overlay 与拖拽/重放/panZoom 协调（Task 3）、双击改标签（Task 3）、AI 上下文（Task 4）、API/DB positions（Task 2）、测试计划（Task 1 单测、Task 2 冒烟、Task 5 E2E）✓；SaveDialog POST 附 positions（Task 3 Step 5）✓；spec 修正：/view 重放移除（controller 裁定）✓
- **占位符**：Task 1 `replaceNodeLabel` 为语义骨架+实现要求（正则细节以测试为准，已明确五变体与仅定义处替换语义）；Task 5 Step 2 为 controller 执行时脚本；其余含完整代码 ✓
- **类型一致性**：`SelectedNode`/`NodeOffset`/`parseNodeId`/`findDefinitionLine`/`replaceNodeLabel`/`nodePositions` 在各任务间一致 ✓
