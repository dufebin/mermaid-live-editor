// Ephemeral multi-selection state for the canvas. Never persisted to
// localStorage or the URL hash. View.svelte is the sole writer and keeps
// the single-selection module (selectedNode.svelte.ts) in sync with it.
let names: string[] = $state.raw([]);

export const selectedNodes = {
  get current(): string[] {
    return names;
  }
};

export const setSelectedNodes = (next: string[]): void => {
  // First-occurrence filter: same semantics as [...new Set(next)] (the repo's
  // svelte/prefer-svelte-reactivity rule rejects plain Set instances).
  names = next.filter((name, index) => next.indexOf(name) === index);
};

export const toggleSelectedNode = (name: string): void => {
  names = names.includes(name) ? names.filter((current) => current !== name) : [...names, name];
};

export const clearSelectedNodes = (): void => {
  names = [];
};
