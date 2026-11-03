<script lang="ts">
  import { Button } from '$/components/ui/button';
  import * as Dialog from '$/components/ui/dialog';
  import { Input } from '$/components/ui/input';
  import { TID } from '$/constants';
  import { createDiagram } from '$/util/api';
  import { setCurrentDiagram } from '$/util/currentDiagram.svelte';
  import { notify } from '$/util/notify';
  import { validatedState } from '$/util/state.svelte';

  interface Props {
    open: boolean;
    onOpenChange: (open: boolean) => void;
  }

  let { open = $bindable(), onOpenChange }: Props = $props();

  let name = $state('');
  let saving = $state(false);
  let error = $state('');

  $effect(() => {
    if (open) {
      name = '';
      error = '';
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
        name: trimmed,
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
