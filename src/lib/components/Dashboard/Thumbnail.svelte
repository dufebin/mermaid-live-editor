<script lang="ts">
  import { render } from '$/util/mermaid';
  import { deserializeState } from '$/util/serde';

  // The prop is named `state` (Task 6 contract), but a local binding named
  // `state` makes `$state(...)` parse as a store subscription, so alias it.
  let { id, state: serialized }: { id: string; state: string } = $props();

  let failed = $state(false);
  let svg = $state('');

  // mermaid.initialize is global; serialize renders so cards do not race.
  let queue: Promise<unknown> = Promise.resolve();
  const renderQueued = (config: unknown, code: string) => {
    const run = queue.then(() => render(config as never, code, `thumbnail-${id}`));
    queue = run.catch(() => undefined);
    return run;
  };

  $effect(() => {
    failed = false;
    svg = '';
    try {
      const parsed = deserializeState(serialized);
      void renderQueued(JSON.parse(parsed.mermaid || '{}'), parsed.code)
        .then((result) => {
          svg = result.svg;
        })
        .catch(() => {
          failed = true;
        });
    } catch {
      failed = true;
    }
  });
</script>

{#if svg}
  <div
    class="pointer-events-none flex h-32 items-center justify-center overflow-hidden p-2 [&_svg]:max-h-full [&_svg]:max-w-full">
    <!-- eslint-disable-next-line svelte/no-at-html-tags -- trusted: svg is mermaid's own render output of user-stored diagrams, same as the editor view -->
    {@html svg}
  </div>
{:else if failed}
  <div class="flex h-32 items-center justify-center text-sm text-muted-foreground">
    Preview unavailable
  </div>
{:else}
  <div class="flex h-32 items-center justify-center text-sm text-muted-foreground">Loading…</div>
{/if}
