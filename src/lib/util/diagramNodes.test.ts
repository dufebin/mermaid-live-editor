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
