<script lang="ts">
  import DiagramCard from '$/components/Dashboard/DiagramCard.svelte';
  import FolderTree from '$/components/Dashboard/FolderTree.svelte';
  import Navbar from '$/components/Navbar.svelte';
  import { Button } from '$/components/ui/button';
  import { Input } from '$/components/ui/input';
  import { TID } from '$/constants';
  import {
    createFolder,
    deleteDiagram,
    deleteFolder,
    exportAll,
    importAll,
    listDiagrams,
    listFolders,
    updateDiagram,
    updateFolder,
    type Diagram,
    type Folder
  } from '$/util/api';
  import { clearCurrentDiagram, setCurrentDiagram } from '$/util/currentDiagram.svelte';
  import { notify } from '$/util/notify';
  import { deserializeState } from '$/util/serde';
  import { defaultState, replaceInputState, sanitizeConfig } from '$/util/state.svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import AddIcon from '~icons/material-symbols/add-rounded';
  import DownloadIcon from '~icons/material-symbols/download-rounded';
  import { onMount } from 'svelte';

  type Selection = 'all' | 'favorites' | 'root' | string;

  let diagrams = $state<Diagram[]>([]);
  let folders = $state<Folder[]>([]);
  let importInput: HTMLInputElement | undefined = $state();
  let loading = $state(true);
  let search = $state('');
  let selection = $state<Selection>('all');

  const reload = async () => {
    loading = true;
    try {
      folders = await listFolders();
      diagrams = await listDiagrams({
        favorite: selection === 'favorites',
        folderId:
          selection === 'all' || selection === 'favorites' ? undefined : (selection as string),
        search: search.trim() || undefined
      });
    } catch (error) {
      notify(error instanceof Error ? error.message : 'Failed to load');
    } finally {
      loading = false;
    }
  };

  let searchTimer: ReturnType<typeof setTimeout> | undefined;
  const onSearch = (value: string) => {
    search = value;
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => void reload(), 300);
  };

  const onSelect = (next: Selection) => {
    selection = next;
    void reload();
  };

  const openDiagram = (diagram: Diagram) => {
    clearCurrentDiagram();
    const next = deserializeState(diagram.state);
    next.mermaid = sanitizeConfig(next.mermaid || defaultState.mermaid);
    replaceInputState(next);
    setCurrentDiagram(diagram);
    void goto(resolve('/edit', {}));
  };

  const newDiagram = () => {
    clearCurrentDiagram();
    replaceInputState(defaultState);
    void goto(resolve('/edit', {}));
  };

  const withReload =
    <Args extends unknown[]>(action: (...args: Args) => Promise<unknown>) =>
    async (...args: Args): Promise<void> => {
      try {
        await action(...args);
        await reload();
      } catch (error) {
        notify(error instanceof Error ? error.message : 'Operation failed');
      }
    };

  const downloadExport = async () => {
    try {
      const data = await exportAll();
      const url = URL.createObjectURL(
        new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
      );
      const link = document.createElement('a');
      link.download = `mermaid-diagrams-${new Date().toISOString().slice(0, 10)}.json`;
      link.href = url;
      link.click();
      URL.revokeObjectURL(url);
    } catch (error) {
      notify(error instanceof Error ? error.message : 'Export failed');
    }
  };

  const onImportFile = async (event: Event) => {
    const input = event.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) {
      return;
    }
    try {
      const result = await importAll(JSON.parse(await file.text()));
      notify(`Imported ${result.imported}, skipped ${result.skipped}`);
      await reload();
    } catch (error) {
      notify(error instanceof Error ? error.message : 'Import failed');
    } finally {
      input.value = '';
    }
  };

  onMount(() => void reload());
</script>

<Navbar hidePromotion>
  <div class="flex flex-1 items-center justify-end gap-2">
    <Input
      class="h-8 w-48"
      data-testid={TID.searchInput}
      placeholder="Search diagrams…"
      value={search}
      oninput={(event) => onSearch(event.currentTarget.value)} />
    <Button size="sm" variant="accent" data-testid={TID.newDiagramButton} onclick={newDiagram}>
      <AddIcon class="size-4" /> New diagram
    </Button>
    <Button
      size="sm"
      variant="outline"
      data-testid={TID.exportButton}
      onclick={() => void downloadExport()}>
      <DownloadIcon class="size-4" /> Export
    </Button>
    <Button size="sm" variant="outline" onclick={() => importInput?.click()}>Import</Button>
    <input
      bind:this={importInput}
      accept="application/json"
      class="hidden"
      data-testid={TID.importInput}
      type="file"
      onchange={(event) => void onImportFile(event)} />
  </div>
</Navbar>

<div class="flex h-[calc(100dvh-5rem)] gap-2 p-2" data-testid={TID.dashboardPage}>
  <FolderTree
    {folders}
    {selection}
    onCreate={withReload((name: string, parentId: string | null) => createFolder(name, parentId))}
    onDelete={withReload((id: string) => deleteFolder(id))}
    onRename={withReload((id: string, name: string) => updateFolder(id, { name }))}
    {onSelect} />
  <div class="flex-1 overflow-y-auto p-2">
    {#if loading}
      <div class="flex h-full items-center justify-center text-muted-foreground">Loading…</div>
    {:else if diagrams.length === 0}
      <div class="flex h-full flex-col items-center justify-center gap-2 text-muted-foreground">
        <p>No diagrams yet.</p>
        <Button size="sm" variant="outline" onclick={newDiagram}>Create your first diagram</Button>
      </div>
    {:else}
      <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
        {#each diagrams as diagram (diagram.id)}
          <DiagramCard
            {diagram}
            {folders}
            onDelete={withReload((d: Diagram) => deleteDiagram(d.id))}
            onMove={withReload((d: Diagram, folderId: string | null) =>
              updateDiagram(d.id, { folderId })
            )}
            onOpen={openDiagram}
            onRename={withReload((d: Diagram, name: string) => updateDiagram(d.id, { name }))}
            onToggleFavorite={withReload((d: Diagram) =>
              updateDiagram(d.id, { favorite: !d.favorite })
            )} />
        {/each}
      </div>
    {/if}
  </div>
</div>
