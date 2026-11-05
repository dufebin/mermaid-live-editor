<script lang="ts">
  import { buttonVariants } from '$/components/ui/button';
  import * as Popover from '$/components/ui/popover';
  import { TID } from '$/constants';
  import { downloadPNG, downloadSVG } from '$/util/download';
  import { notify } from '$/util/notify';
  import DownloadIcon from '~icons/material-symbols/download';

  interface DownloadAction {
    label: string;
    run: () => void | Promise<void>;
    testID: string;
  }

  const actions: DownloadAction[] = [
    { label: 'PNG', run: () => downloadPNG(), testID: TID.navbarDownloadPNG },
    { label: 'SVG', run: () => downloadSVG(), testID: TID.navbarDownloadSVG }
  ];

  const onDownload = async (run: () => void | Promise<void>) => {
    try {
      await run();
    } catch (error) {
      notify(error instanceof Error ? error.message : 'Download failed');
    }
  };
</script>

<Popover.Root>
  <Popover.Trigger class={buttonVariants({ size: 'sm' })} data-testid={TID.downloadButton}>
    <DownloadIcon />
    Download
  </Popover.Trigger>
  <Popover.Content>
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <ul tabindex="0" class="flex flex-col">
      {#each actions as { label, run, testID } (label)}
        <li class="rounded-md p-2 hover:bg-muted">
          <button
            role="menuitem"
            tabindex="0"
            class="whitespace-nowrap"
            data-testid={testID}
            onclick={() => void onDownload(run)}>
            {label}
          </button>
        </li>
      {/each}
    </ul>
  </Popover.Content>
</Popover.Root>
