export interface Rect {
  height: number;
  left: number;
  top: number;
  width: number;
}

export interface NodeRect {
  name: string;
  rect: Rect;
}

export const MARQUEE_CLICK_THRESHOLD_PX = 3;

export const normalizeRect = (x1: number, y1: number, x2: number, y2: number): Rect => ({
  height: Math.abs(y2 - y1),
  left: Math.min(x1, x2),
  top: Math.min(y1, y2),
  width: Math.abs(x2 - x1)
});

export const rectsIntersect = (a: Rect, b: Rect): boolean =>
  a.left <= b.left + b.width &&
  b.left <= a.left + a.width &&
  a.top <= b.top + b.height &&
  b.top <= a.top + a.height;

export const isMarqueeClick = (x1: number, y1: number, x2: number, y2: number): boolean =>
  Math.abs(x2 - x1) < MARQUEE_CLICK_THRESHOLD_PX && Math.abs(y2 - y1) < MARQUEE_CLICK_THRESHOLD_PX;

export const namesInRect = (nodes: NodeRect[], marquee: Rect): string[] => {
  const hits: string[] = [];
  for (const node of nodes) {
    if (rectsIntersect(node.rect, marquee) && !hits.includes(node.name)) {
      hits.push(node.name);
    }
  }
  return hits;
};
