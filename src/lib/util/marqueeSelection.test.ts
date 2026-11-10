import { describe, expect, it } from 'vitest';
import { isMarqueeClick, namesInRect, normalizeRect, rectsIntersect } from './marqueeSelection';

describe('normalizeRect', () => {
  it('normalizes dragged-in-any-direction corners', () => {
    expect(normalizeRect(100, 50, 40, 10)).toEqual({
      height: 40,
      left: 40,
      top: 10,
      width: 60
    });
  });
});

describe('rectsIntersect', () => {
  it('detects overlap', () => {
    expect(
      rectsIntersect(
        { height: 10, left: 0, top: 0, width: 10 },
        {
          height: 10,
          left: 5,
          top: 5,
          width: 10
        }
      )
    ).toBe(true);
  });

  it('rejects disjoint rects', () => {
    expect(
      rectsIntersect(
        { height: 10, left: 0, top: 0, width: 10 },
        {
          height: 10,
          left: 11,
          top: 0,
          width: 10
        }
      )
    ).toBe(false);
  });

  it('treats touching edges as intersecting', () => {
    expect(
      rectsIntersect(
        { height: 10, left: 0, top: 0, width: 10 },
        {
          height: 10,
          left: 10,
          top: 0,
          width: 10
        }
      )
    ).toBe(true);
  });
});

describe('isMarqueeClick', () => {
  it('treats sub-threshold movement as a click', () => {
    expect(isMarqueeClick(10, 10, 12, 12)).toBe(true);
  });

  it('treats real drags as a marquee', () => {
    expect(isMarqueeClick(10, 10, 40, 12)).toBe(false);
  });
});

describe('namesInRect', () => {
  const nodes = [
    { name: 'A', rect: { height: 10, left: 0, top: 0, width: 10 } },
    { name: 'B', rect: { height: 10, left: 100, top: 0, width: 10 } },
    // sequence-actor case: two boxes, same name — hitting either selects it
    { name: 'C', rect: { height: 10, left: 200, top: 0, width: 10 } },
    { name: 'C', rect: { height: 10, left: 0, top: 500, width: 10 } }
  ];

  it('returns intersecting names in DOM order', () => {
    expect(namesInRect(nodes, { height: 100, left: 0, top: 0, width: 300 })).toEqual([
      'A',
      'B',
      'C'
    ]);
  });

  it('dedupes names with multiple boxes', () => {
    expect(namesInRect(nodes, { height: 20, left: 0, top: 490, width: 20 })).toEqual(['C']);
  });

  it('returns empty for no hits', () => {
    expect(namesInRect(nodes, { height: 5, left: 400, top: 400, width: 5 })).toEqual([]);
  });
});
