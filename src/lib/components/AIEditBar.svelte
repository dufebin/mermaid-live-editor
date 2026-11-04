<script lang="ts">
  import { Button } from '$/components/ui/button';
  import { TID } from '$/constants';
  import { editDiagramWithAI } from '$/util/ai';
  import { notify } from '$/util/notify';
  import { clearSelectedNode, selectedNode } from '$/util/selectedNode.svelte';
  import { updateCode, validatedState } from '$/util/state.svelte';
  import { tick } from 'svelte';
  import SendIcon from '~icons/material-symbols/send-rounded';

  let input = $state('');
  let isLoading = $state(false);
  let textarea: HTMLTextAreaElement | undefined = $state();

  // Single line by default, auto-grow up to ~3 lines.
  const resizeTextarea = () => {
    if (!textarea) {
      return;
    }
    textarea.style.height = 'auto';
    textarea.style.height = `${Math.min(textarea.scrollHeight, 66)}px`;
  };

  const submit = async () => {
    const instruction = input.trim();
    if (!instruction || isLoading) {
      return;
    }
    isLoading = true;
    try {
      const selection = selectedNode.current;
      const instructionWithContext = selection
        ? `${instruction}\n\nFocus on node ${selection.name}, currently: ${selection.lineText}`
        : instruction;
      const newCode = await editDiagramWithAI(instructionWithContext, validatedState.current.code);
      updateCode(newCode);
      input = '';
      clearSelectedNode();
    } catch (error) {
      notify(error instanceof Error ? error.message : 'AI request failed');
    } finally {
      isLoading = false;
      await tick();
      textarea?.focus();
    }
  };

  // Programmatic clears (input = '' after submit) don't fire oninput, so shrink manually.
  $effect(() => {
    if (!input) {
      resizeTextarea();
    }
  });
</script>

<div class="flex min-w-0 flex-1 items-center gap-2" data-testid={TID.aiEditBar}>
  <textarea
    bind:this={textarea}
    bind:value={input}
    class="font-recursive h-9 min-w-0 flex-1 resize-none rounded-md border border-border bg-transparent px-2 py-1.5 text-sm text-foreground placeholder:text-muted-foreground focus:ring-1 focus:ring-accent focus:outline-none disabled:opacity-50 dark:border-border-dark dark:text-foreground"
    data-testid={TID.aiEditInput}
    disabled={isLoading}
    placeholder="Describe changes to this diagram…"
    rows="1"
    oninput={resizeTextarea}
    onkeydown={(event) => {
      if (event.isComposing) {
        return;
      }
      if (event.key === 'Enter' && !event.shiftKey) {
        event.preventDefault();
        void submit();
      }
    }} />
  {#if selectedNode.current}
    <button
      class="flex shrink-0 items-center gap-1 rounded-full border border-accent px-2 py-0.5 text-xs text-accent"
      title="Clear node reference"
      onclick={clearSelectedNode}>
      ⊙ {selectedNode.current.name} ×
    </button>
  {/if}
  <Button
    variant="accent"
    size="sm"
    class="shrink-0"
    data-testid={TID.aiEditSend}
    disabled={isLoading || !input.trim()}
    onclick={() => void submit()}>
    {#if isLoading}
      Editing…
    {:else}
      <SendIcon class="size-4" /> Edit
    {/if}
  </Button>
</div>
