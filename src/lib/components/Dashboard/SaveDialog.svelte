<script lang="ts">
  import { Button } from '$/components/ui/button';
  import * as Dialog from '$/components/ui/dialog';
  import { Input } from '$/components/ui/input';
  import { TID } from '$/constants';
  import { createDiagram, listFolders, type Folder } from '$/util/api';
  import { setCurrentDiagram } from '$/util/currentDiagram.svelte';
  import { notify } from '$/util/notify';
  import { nodePositions } from '$/util/nodePositions.svelte';
  import { validatedState } from '$/util/state.svelte';

  interface Props {
    open: boolean;
    onOpenChange: (open: boolean) => void;
  }

  let { open = $bindable(), onOpenChange }: Props = $props();

  let name = $state('');
  let saving = $state(false);
  let error = $state('');
  let folders = $state<Folder[]>([]);
  let folderId = $state<string | null>(null);

  $effect(() => {
    if (open) {
      name = '';
      error = '';
      folderId = null;
      void listFolders()
        .then((list) => {
          folders = list;
        })
        .catch(() => {
          folders = [];
        });
    }
  });

  const save = async () => {
    const trimmed = name.trim();
    if (!trimmed) {
      error = 'Name is required';
      return;
    }
    saving = true;
    try {
      const diagram = await createDiagram({
        folderId,
        name: trimmed,
        positions: nodePositions.value,
        state: validatedState.current.serialized
      });
      setCurrentDiagram(diagram);
      notify('Saved');
      onOpenChange(false);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Save failed';
    } finally {
      saving = false;
    }
  };
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="sm:max-w-sm">
    <Dialog.Header>
      <Dialog.Title>Save diagram</Dialog.Title>
      <Dialog.Description>Choose a name for your diagram.</Dialog.Description>
    </Dialog.Header>
    <div class="flex flex-col gap-2">
      <Input
        bind:value={name}
        data-testid={TID.saveDialogInput}
        placeholder="Diagram name"
        onkeydown={(event) => {
          if (event.key === 'Enter' && !saving) {
            void save();
          }
        }} />
      {#if error}
        <p class="text-sm text-destructive">{error}</p>
      {/if}
      <select
        bind:value={folderId}
        class="h-9 w-full rounded-md border border-border bg-transparent px-2 text-sm text-foreground focus:ring-1 focus:ring-accent focus:outline-none dark:border-border-dark"
        data-testid={TID.saveDialogFolderSelect}
        disabled={saving}>
        <option value={null}>Uncategorized</option>
        {#each folders as folder (folder.id)}
          <option value={folder.id}>{folder.name}</option>
        {/each}
      </select>
    </div>
    <Dialog.Footer>
      <Button disabled={saving} variant="outline" onclick={() => onOpenChange(false)}
        >Cancel</Button>
      <Button disabled={saving} variant="accent" onclick={() => void save()}>
        {saving ? 'Saving…' : 'Save'}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
