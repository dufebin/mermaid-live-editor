import { describe, expect, it } from 'vitest';
import { buildFolderTree } from './folderTree';
import type { Folder } from './api';

const folder = (id: string, name: string, parentId: string | null = null): Folder => ({
  createdAt: '2026-11-03T00:00:00.000Z',
  id,
  name,
  parentId,
  updatedAt: '2026-11-03T00:00:00.000Z'
});

describe('buildFolderTree', () => {
  it('nests children under parents', () => {
    const tree = buildFolderTree([folder('a', 'A'), folder('b', 'B', 'a'), folder('c', 'C', 'b')]);
    expect(tree).toHaveLength(1);
    expect(tree[0]?.folder.id).toBe('a');
    expect(tree[0]?.children[0]?.folder.id).toBe('b');
    expect(tree[0]?.children[0]?.children[0]?.folder.id).toBe('c');
  });

  it('treats orphans (missing parent) as roots', () => {
    const tree = buildFolderTree([folder('a', 'A'), folder('b', 'B', 'missing')]);
    expect(tree).toHaveLength(2);
  });

  it('breaks cycles safely instead of recursing forever', () => {
    const tree = buildFolderTree([folder('a', 'A', 'b'), folder('b', 'B', 'a')]);
    expect(tree).toHaveLength(2);
  });
});
