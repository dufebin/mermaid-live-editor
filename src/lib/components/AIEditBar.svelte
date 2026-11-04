<script lang="ts">
  import { Button } from '$/components/ui/button';
  import { TID } from '$/constants';
  import { editDiagramWithAI } from '$/util/ai';
  import { notify } from '$/util/notify';
  import { updateCode, validatedState } from '$/util/state.svelte';
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
      const newCode = await editDiagramWithAI(instruction, validatedState.current.code);
      updateCode(newCode);
      input = '';
    } catch (error) {
      notify(error instanceof Error ? error.message : 'AI request failed');
    } finally {
      isLoading = false;
      textarea?.focus();
    }
  };
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
      if (event.key === 'Enter' && !event.shiftKey) {
        event.preventDefault();
        void submit();
      }
    }} />
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
