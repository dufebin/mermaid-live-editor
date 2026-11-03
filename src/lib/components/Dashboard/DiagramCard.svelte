<script lang="ts">
  import { Button } from '$/components/ui/button';
  import * as Popover from '$/components/ui/popover';
  import { TID } from '$/constants';
  import type { Diagram, Folder } from '$/util/api';
  import { buildFolderTree, type FolderNode } from '$/util/folderTree';
  import dayjs from 'dayjs';
  import relativeTime from 'dayjs/plugin/relativeTime';
  import DotsIcon from '~icons/material-symbols/more-vert-rounded';
  import StarIcon from '~icons/material-symbols/star-rounded';
  import StarOutlineIcon from '~icons/material-symbols/star-outline-rounded';
  import Thumbnail from './Thumbnail.svelte';

  dayjs.extend(relativeTime);

  interface Props {
    diagram: Diagram;
    folders: Folder[];
    onDelete: (diagram: Diagram) => void;
    onMove: (diagram: Diagram, folderId: string | null) => void;
    onOpen: (diagram: Diagram) => void;
    onRename: (diagram: Diagram, name: string) => void;
    onToggleFavorite: (diagram: Diagram) => void;
  }

  let { diagram, folders, onDelete, onMove, onOpen, onRename, onToggleFavorite }: Props = $props();

  const rename = () => {
    const name = window.prompt('Rename diagram', diagram.name);
    if (name && name.trim() && name.trim() !== diagram.name) {
      onRename(diagram, name.trim());
    }
  };

  const remove = () => {
    if (window.confirm(`Delete "${diagram.name}"?`)) {
      onDelete(diagram);
    }
  };
</script>

<div
  class="flex flex-col rounded-xl border border-border bg-background shadow-sm"
  data-testid={TID.diagramCard}>
  <button class="cursor-pointer text-left" onclick={() => onOpen(diagram)}>
    <Thumbnail id={diagram.id} state={diagram.state} />
  </button>
  <div class="flex items-center gap-1 border-t border-border p-2">
    <button
      class="min-w-0 flex-1 cursor-pointer text-left"
      title="Open"
      onclick={() => onOpen(diagram)}>
      <div class="truncate text-sm font-medium">{diagram.name}</div>
      <div class="text-xs text-muted-foreground">{dayjs(diagram.updatedAt).fromNow()}</div>
    </button>
    <button
      class="p-1 {diagram.favorite ? 'text-accent' : 'text-muted-foreground hover:text-foreground'}"
      title={diagram.favorite ? 'Unfavorite' : 'Favorite'}
      onclick={() => onToggleFavorite(diagram)}>
      {#if diagram.favorite}
        <StarIcon class="size-4" />
      {:else}
        <StarOutlineIcon class="size-4" />
      {/if}
    </button>
    <Popover.Root>
      <Popover.Trigger class="p-1 text-muted-foreground hover:text-foreground" title="Actions">
        <DotsIcon class="size-4" />
      </Popover.Trigger>
      <Popover.Content class="w-44 p-1">
        <div class="flex flex-col">
          <Button size="sm" variant="ghost" onclick={() => onOpen(diagram)}>Open</Button>
          <Button size="sm" variant="ghost" onclick={rename}>Rename</Button>
          <Button size="sm" variant="ghost" onclick={remove}>Delete</Button>
          <div class="my-1 border-t border-border"></div>
          <div class="px-2 py-1 text-xs text-muted-foreground">Move to…</div>
          <Button
            size="sm"
            variant="ghost"
            class={diagram.folderId === null ? 'font-medium' : ''}
            onclick={() => onMove(diagram, null)}>
            Uncategorized
          </Button>
          {#each buildFolderTree(folders) as node (node.folder.id)}
            {@render folderOption(node, 0)}
          {/each}
        </div>
      </Popover.Content>
    </Popover.Root>
  </div>
</div>

{#snippet folderOption(node: FolderNode, depth: number)}
  <Button
    size="sm"
    variant="ghost"
    class="justify-start {diagram.folderId === node.folder.id ? 'font-medium' : ''}"
    style="padding-left: {12 + depth * 12}px"
    onclick={() => onMove(diagram, node.folder.id)}>
    {node.folder.name}
  </Button>
  {#each node.children as child (child.folder.id)}
    {@render folderOption(child, depth + 1)}
  {/each}
{/snippet}
