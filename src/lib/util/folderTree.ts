import type { Folder } from './api';

export interface FolderNode {
  children: FolderNode[];
  folder: Folder;
}

export const buildFolderTree = (folders: Folder[]): FolderNode[] => {
  const nodes = new Map<string, FolderNode>();
  for (const f of folders) {
    nodes.set(f.id, { children: [], folder: f });
  }
  // A node whose parent chain loops back on itself never reaches a root, so it
  // would be dropped from the tree entirely. Detect cycle members and promote
  // them to roots instead. Nodes that merely hang off a cycle keep nesting
  // under their (now rooted) cycle-member parent.
  const cycleMembers = new Set<string>();
  for (const start of nodes.keys()) {
    const path: string[] = [];
    const position = new Map<string, number>();
    let current: string | undefined = start;
    while (current !== undefined) {
      if (position.has(current)) {
        // The cycle is the path from the first visit of the repeated node.
        for (const id of path.slice(position.get(current))) {
          cycleMembers.add(id);
        }
        break;
      }
      if (cycleMembers.has(current)) {
        break;
      }
      const parentId: string | null | undefined = nodes.get(current)?.folder.parentId;
      if (!parentId || !nodes.has(parentId)) {
        break; // Chain terminates at a root or an orphaned (missing) parent.
      }
      position.set(current, path.length);
      path.push(current);
      current = parentId;
    }
  }
  const roots: FolderNode[] = [];
  for (const node of nodes.values()) {
    if (cycleMembers.has(node.folder.id)) {
      roots.push(node);
      continue;
    }
    const parent = node.folder.parentId ? nodes.get(node.folder.parentId) : undefined;
    if (parent && parent !== node) {
      parent.children.push(node);
    } else {
      roots.push(node);
    }
  }
  return roots;
};
