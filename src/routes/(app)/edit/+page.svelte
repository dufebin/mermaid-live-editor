<script lang="ts">
  import AIEditBar from '$/components/AIEditBar.svelte';
  import Actions from '$/components/Actions.svelte';
  import Card from '$/components/Card/Card.svelte';
  import SaveDialog from '$/components/Dashboard/SaveDialog.svelte';
  import DiagramDocButton from '$/components/DiagramDocumentationButton.svelte';
  import Editor from '$/components/Editor.svelte';
  import EnhancedEditsButton from '$/components/EnhancedEditsButton.svelte';
  import History from '$/components/History/History.svelte';
  import { startAutoSave } from '$/components/History/historyState.svelte';
  import McWrapper from '$/components/McWrapper.svelte';
  import MermaidChartIcon from '$/components/MermaidChartIcon.svelte';
  import EditorChooserModal from '$/components/migration/EditorChooserModal.svelte';
  import Navbar from '$/components/Navbar.svelte';
  import PanZoomToolbar from '$/components/PanZoomToolbar.svelte';
  import Preset from '$/components/Preset.svelte';
  import Share from '$/components/Share.svelte';
  import SyncRoughToolbar from '$/components/SyncRoughToolbar.svelte';
  import { Button } from '$/components/ui/button';
  import { Separator } from '$/components/ui/separator';
  import * as Resizable from '$/components/ui/resizable';
  import { Switch } from '$/components/ui/switch';
  import { Toggle } from '$/components/ui/toggle';
  import VersionSecurityToolbar from '$/components/VersionSecurityToolbar.svelte';
  import View from '$/components/View.svelte';
  import { TID } from '$/constants';
  import type { EditorMode, Tab } from '$/types';
  import { updateDiagram } from '$/util/api';
  import { currentDiagram, setCurrentDiagram } from '$/util/currentDiagram.svelte';
  import { env } from '$/util/env';
  import { shouldShowEditorChooser } from '$/util/migration/domainMigration';
  import { notify } from '$/util/notify';
  import { PanZoomState } from '$/util/panZoom';
  import { validatedState, updateCodeStore, urls } from '$/util/state.svelte';
  import { logEvent, logMermaidChartClick } from '$/util/stats';
  import { getContactSalesUrl, initHandler } from '$/util/util';
  import { resolve } from '$app/paths';
  import { onMount } from 'svelte';
  import CodeIcon from '~icons/custom/code';
  import HistoryIcon from '~icons/material-symbols/history';
  import GearIcon from '~icons/material-symbols/settings-outline-rounded';

  const panZoomState = new PanZoomState();

  const tabSelectHandler = (tab: Tab) => {
    const editorMode: EditorMode = tab.id === 'code' ? 'code' : 'config';
    updateCodeStore({ editorMode });
  };

  const editorTabs: Tab[] = [
    {
      icon: CodeIcon,
      id: 'code',
      title: 'Code'
    },
    {
      icon: GearIcon,
      id: 'config',
      title: 'Config'
    }
  ];

  let width = $state(0);
  let isMobile = $derived(width < 640);
  let isViewMode = $state(true);
  let showEditorChooser = $state(false);

  onMount(async () => {
    showEditorChooser = shouldShowEditorChooser();
    await initHandler();
    window.addEventListener('appinstalled', () => {
      logEvent('pwaInstalled', { isMobile });
    });
  });

  // Record the Timeline for the whole session, not just while the panel is open.
  onMount(() => startAutoSave());

  let isHistoryOpen = $state(false);

  let saveDialogOpen = $state(false);

  const onSave = () => {
    if (currentDiagram.diagram) {
      void saveExisting();
    } else {
      saveDialogOpen = true;
    }
  };

  const saveExisting = async () => {
    const existing = currentDiagram.diagram;
    if (!existing) {
      return;
    }
    try {
      setCurrentDiagram(
        await updateDiagram(existing.id, { state: validatedState.current.serialized })
      );
      notify('Saved');
    } catch (error) {
      notify(error instanceof Error ? error.message : 'Save failed');
    }
  };

  let editorPane: Resizable.Pane | undefined;
  $effect(() => {
    if (isMobile) {
      editorPane?.resize(50);
    }
  });
</script>

<div class="flex h-full flex-col overflow-hidden">
  {#snippet mobileToggle()}
    <div class="flex items-center gap-2">
      Edit <Switch
        id="editorMode"
        class="data-[state=checked]:bg-accent"
        bind:checked={isViewMode}
        onclick={() => {
          logEvent('mobileViewToggle');
        }} /> View
    </div>
  {/snippet}

  <Navbar mobileToggle={isMobile ? mobileToggle : undefined}>
    <Button
      size="sm"
      variant="ghost"
      data-testid={TID.diagramsLink}
      href={resolve('/dashboard', {})}>
      Diagrams
    </Button>
    <Toggle bind:pressed={isHistoryOpen} size="sm" title="History" aria-label="History">
      <HistoryIcon />
    </Toggle>
    <Share />
    <Button variant="accent" size="sm" data-testid={TID.saveButton} onclick={onSave}>Save</Button>
    {#if env.isEnabledMermaidChartLinks}
      <Separator orientation="vertical" />
      <McWrapper labelPrefix="Opens ">
        <Button
          size="sm"
          href={getContactSalesUrl()}
          target="_blank"
          onclick={() => logMermaidChartClick('contactSales')}>
          <MermaidChartIcon />
          Contact sales
        </Button>
      </McWrapper>
      <McWrapper>
        <Button
          variant="accent"
          size="sm"
          href={urls.current.mermaidChart({ medium: 'save_diagram' }).save}
          target="_blank"
          onclick={() => logMermaidChartClick('saveDiagram')}>
          <MermaidChartIcon />
          Save diagram
        </Button>
      </McWrapper>
    {/if}
  </Navbar>

  <div class="flex flex-1 flex-col overflow-hidden" bind:clientWidth={width}>
    <div
      class={[
        'size-full',
        isMobile && ['w-[200%] duration-300', isViewMode && '-translate-x-1/2']
      ]}>
      <Resizable.PaneGroup
        direction="horizontal"
        autoSaveId="liveEditor"
        class="gap-4 p-2 pt-0 sm:gap-0 sm:p-6 sm:pt-0">
        <Resizable.Pane bind:this={editorPane} defaultSize={30} minSize={15}>
          <div class="flex h-full flex-col gap-4 sm:gap-6">
            <Card
              onselect={tabSelectHandler}
              isOpen
              tabs={editorTabs}
              activeTabID={validatedState.current.editorMode}
              isClosable={false}>
              {#snippet actions()}
                <DiagramDocButton />
              {/snippet}
              <Editor {isMobile} />
            </Card>

            <div class="group flex flex-wrap justify-between gap-4 sm:gap-6">
              <Preset />
              <Actions />
            </div>
          </div>
        </Resizable.Pane>
        <Resizable.Handle class="mr-1 hidden opacity-0 sm:block" />
        <Resizable.Pane minSize={15} class="relative flex h-full flex-1 flex-col overflow-hidden">
          <View {panZoomState} editable={true} shouldShowGrid={validatedState.current.grid} />
          <div class="absolute top-0 left-5 hidden md:block"><EnhancedEditsButton /></div>
          <div class="absolute top-0 right-0">
            <PanZoomToolbar {panZoomState} fullScreenHref={urls.current.view} />
          </div>
          <div class="flex items-center justify-between gap-2 p-2">
            <SyncRoughToolbar />
            {#if env.aiEnabled}
              <AIEditBar />
            {/if}
            <VersionSecurityToolbar />
          </div>
        </Resizable.Pane>
        {#if isHistoryOpen}
          <Resizable.Handle class="ml-1 hidden opacity-0 sm:block" />
          <Resizable.Pane minSize={15} defaultSize={30} class="hidden h-full grow flex-col sm:flex">
            <History />
          </Resizable.Pane>
        {/if}
      </Resizable.PaneGroup>
    </div>
  </div>
</div>

<svelte:window
  onkeydown={(event) => {
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
      if (event.repeat) {
        event.preventDefault();
        return;
      }
      event.preventDefault();
      onSave();
    }
  }} />

<EditorChooserModal bind:open={showEditorChooser} />
<SaveDialog bind:open={saveDialogOpen} onOpenChange={(open) => (saveDialogOpen = open)} />
