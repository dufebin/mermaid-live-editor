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
