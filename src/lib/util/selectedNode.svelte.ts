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
