<script lang="ts">
  import { Button } from '$/components/ui/button';
  import { buildFolderTree, type FolderNode } from '$/util/folderTree';
  import type { Folder } from '$/util/api';
  import AddIcon from '~icons/material-symbols/add-rounded';
  import DeleteIcon from '~icons/material-symbols/delete-outline-rounded';
  import EditIcon from '~icons/material-symbols/edit-outline-rounded';
  import StarIcon from '~icons/material-symbols/star-rounded';
  import FolderIcon from '~icons/material-symbols/folder-rounded';

  interface Props {
    folders: Folder[];
    selection: 'all' | 'favorites' | 'root' | string;
    onCreate: (name: string, parentId: string | null) => void;
    onDelete: (id: string) => void;
    onRename: (id: string, name: string) => void;
    onSelect: (selection: 'all' | 'favorites' | 'root' | string) => void;
  }

  let { folders, selection, onCreate, onDelete, onRename, onSelect }: Props = $props();

  const quickLinks: { key: 'all' | 'favorites' | 'root'; label: string }[] = [
    { key: 'all', label: 'All diagrams' },
    { key: 'favorites', label: 'Favorites' },
    { key: 'root', label: 'Uncategorized' }
  ];

  const createFolder = (parentId: string | null) => {
    const name = window.prompt('Folder name');
    if (name && name.trim()) {
      onCreate(name.trim(), parentId);
    }
  };

  const renameFolder = (folder: Folder) => {
    const name = window.prompt('Rename folder', folder.name);
    if (name && name.trim() && name.trim() !== folder.name) {
      onRename(folder.id, name.trim());
    }
  };

  const deleteFolder = (folder: Folder) => {
    if (
      window.confirm(
        `Delete "${folder.name}"? Sub-folders are deleted; diagrams move to Uncategorized.`
      )
    ) {
      onDelete(folder.id);
    }
  };
</script>

<div class="flex w-56 shrink-0 flex-col gap-2 border-r border-border p-2">
  <Button size="sm" variant="outline" onclick={() => createFolder(null)}>
    <AddIcon class="size-4" /> New folder
  </Button>
  {#each quickLinks as link (link.key)}
    <button
      class="flex items-center gap-2 rounded-sm px-2 py-1 text-left text-sm hover:bg-muted {selection ===
      link.key
        ? 'bg-muted font-medium'
        : ''}"
      onclick={() => onSelect(link.key)}>
      {#if link.key === 'favorites'}
        <StarIcon class="size-4" />
      {:else}
        <FolderIcon class="size-4" />
      {/if}
      {link.label}
    </button>
  {/each}
  {#each buildFolderTree(folders) as node (node.folder.id)}
    {@render treeNode(node, 0)}
  {/each}
</div>

{#snippet treeNode(node: FolderNode, depth: number)}
  <div class="group flex items-center" style="padding-left: {depth * 12}px">
    <button
      class="flex flex-1 cursor-pointer items-center gap-2 truncate rounded-sm px-2 py-1 text-left text-sm hover:bg-muted {selection ===
      node.folder.id
        ? 'bg-muted font-medium'
        : ''}"
      onclick={() => onSelect(node.folder.id)}>
      <FolderIcon class="size-4 shrink-0" />
      <span class="truncate">{node.folder.name}</span>
    </button>
    <span class="hidden gap-0.5 group-hover:flex">
      <button
        class="p-1 text-muted-foreground hover:text-foreground"
        title="Rename"
        onclick={() => renameFolder(node.folder)}>
        <EditIcon class="size-3.5" />
      </button>
      <button
        class="p-1 text-muted-foreground hover:text-foreground"
        title="New sub-folder"
        onclick={() => createFolder(node.folder.id)}>
        <AddIcon class="size-3.5" />
      </button>
      <button
        class="p-1 text-muted-foreground hover:text-foreground"
        title="Delete"
        onclick={() => deleteFolder(node.folder)}>
        <DeleteIcon class="size-3.5" />
      </button>
    </span>
  </div>
  {#each node.children as child (child.folder.id)}
    {@render treeNode(child, depth + 1)}
  {/each}
{/snippet}
