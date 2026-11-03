import { afterEach, describe, expect, it, vi } from 'vitest';
import { editDiagramWithAI, parseAIContent } from './ai';

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

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('editDiagramWithAI error handling', () => {
  it('throws with status and body excerpt on non-200', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response('{"error":"bad key"}', { status: 401 }))
    );
    await expect(editDiagramWithAI('test', 'flowchart TD')).rejects.toThrow(
      'AI request failed (401): {"error":"bad key"}'
    );
  });

  it('throws on non-JSON success response', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response('<html>oops</html>', { status: 200 }))
    );
    await expect(editDiagramWithAI('test', 'flowchart TD')).rejects.toThrow(
      'invalid (non-JSON) response'
    );
  });

  it('throws on empty content', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ choices: [{ message: { content: '  ' } }] }), {
          status: 200
        })
      )
    );
    await expect(editDiagramWithAI('test', 'flowchart TD')).rejects.toThrow('empty response');
  });
});
