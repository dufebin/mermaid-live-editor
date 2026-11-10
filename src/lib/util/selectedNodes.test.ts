import { describe, expect, it } from 'vitest';
import {
  clearSelectedNodes,
  selectedNodes,
  setSelectedNodes,
  toggleSelectedNode
} from './selectedNodes.svelte';

describe('selectedNodes store', () => {
  it('starts empty', () => {
    expect(selectedNodes.current).toEqual([]);
  });

  it('sets and dedupes', () => {
    setSelectedNodes(['A', 'B', 'A']);
    expect(selectedNodes.current).toEqual(['A', 'B']);
  });

  it('toggles membership', () => {
    setSelectedNodes(['A', 'B']);
    toggleSelectedNode('C');
    expect(selectedNodes.current).toEqual(['A', 'B', 'C']);
    toggleSelectedNode('A');
    expect(selectedNodes.current).toEqual(['B', 'C']);
  });

  it('clears', () => {
    setSelectedNodes(['A']);
    clearSelectedNodes();
    expect(selectedNodes.current).toEqual([]);
  });
});
