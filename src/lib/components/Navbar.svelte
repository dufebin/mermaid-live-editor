<script lang="ts" module>
  import { logEvent, logMermaidChartClick } from '$lib/util/stats';
  import { version } from 'mermaid/package.json';

  void logEvent('version', {
    mermaidVersion: version
  });
</script>

<script lang="ts">
  import { resolve } from '$app/paths';
  import MainMenu from '$/components/MainMenu.svelte';
  import { Button } from '$/components/ui/button';
  import * as Popover from '$/components/ui/popover';
  import { Separator } from '$/components/ui/separator';
  import { TID } from '$/constants';
  import { authState, fetchMe, logout } from '$/util/auth.svelte';
  import { dismissPromotion, getActivePromotion } from '$lib/util/promos/promo.svelte';
  import { onMount, untrack, type ComponentProps, type Snippet } from 'svelte';
  import MermaidIcon from '~icons/custom/mermaid';
  import CloseIcon from '~icons/material-symbols/close-rounded';
  import GithubIcon from '~icons/mdi/github';
  import DropdownNavMenu from './DropdownNavMenu.svelte';

  interface Props {
    mobileToggle?: Snippet;
    children: Snippet;
    hidePromotion?: boolean;
  }

  let { children, mobileToggle, hidePromotion = false }: Props = $props();

  type Links = ComponentProps<typeof DropdownNavMenu>['links'];

  const githubLinks: Links = [
    { title: 'Mermaid JS', href: 'https://github.com/mermaid-js/mermaid' },
    {
      title: 'Mermaid Live Editor',
      href: 'https://github.com/mermaid-js/mermaid-live-editor'
    },
    {
      title: 'Mermaid CLI',
      href: 'https://github.com/mermaid-js/mermaid-cli'
    }
  ];

  let activePromotion = $state(untrack(() => (hidePromotion ? undefined : getActivePromotion())));

  // Populate the auth state on first load; the navbar renders nothing
  // auth-related until fetchMe() resolves (undefined = not yet checked).
  onMount(() => {
    void fetchMe();
  });

  const trackBannerClick = () => {
    if (!activePromotion) {
      return;
    }
    logEvent('bannerClick', {
      promotion: activePromotion.id
    });
    logMermaidChartClick('banner');
  };
</script>

{#if activePromotion}
  <div class="top-bar z-10 flex h-fit w-full bg-primary">
    <div
      class="flex grow"
      role="button"
      tabindex="0"
      onclick={trackBannerClick}
      onkeypress={trackBannerClick}>
      <activePromotion.component {closeBanner} />
    </div>
    {#snippet closeBanner()}
      <Button
        title="Dismiss banner"
        variant="ghost"
        class="hover:bg-transparent hover:text-[#261A56]"
        size="sm"
        onclick={() => {
          dismissPromotion(activePromotion?.id);
          activePromotion = undefined;
        }}>
        <CloseIcon />
      </Button>
    {/snippet}
  </div>
{/if}

<nav class="z-50 flex p-4 sm:p-6">
  <div class="flex flex-1 items-center gap-2">
    <MainMenu />
    <MermaidIcon class="size-6" />
    <a href={resolve('/', {})} class="whitespace-nowrap text-accent">
      {#if !mobileToggle}
        Mermaid
      {/if}
      Live Editor
    </a>
  </div>
  <div
    id="menu"
    class="hidden flex-nowrap items-center justify-between gap-3 overflow-hidden md:flex">
    <DropdownNavMenu icon={GithubIcon} links={githubLinks} />
    <Separator orientation="vertical" />
    {@render children()}
    {#if authState.current}
      <Separator orientation="vertical" />
      <Popover.Root>
        <Popover.Trigger class="flex items-center gap-0">
          <Button
            variant="ghost"
            size="sm"
            class="max-w-40 truncate"
            data-testid={TID.navbarUserMenu}
            title={authState.current.email}>
            {authState.current.email}
          </Button>
        </Popover.Trigger>
        <Popover.Content>
          <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
          <ul tabindex="0" class="flex flex-col">
            <li class="rounded-md p-2 hover:bg-muted">
              <button
                class="whitespace-nowrap underline"
                data-testid={TID.navbarLogout}
                onclick={() => {
                  void logout().then(() => location.assign(resolve('/login', {})));
                }}>
                Log out
              </button>
            </li>
          </ul>
        </Popover.Content>
      </Popover.Root>
    {:else if authState.current === null}
      <a
        class="whitespace-nowrap text-accent"
        data-testid={TID.navbarLogin}
        href={resolve('/login', {})}>
        Log in
      </a>
    {/if}
  </div>
  {@render mobileToggle?.()}
  <!-- Mobile counterpart of the auth block in #menu above (hidden md:flex):
       without it, mobile users can log in via the guard redirect but never
       log out. Anonymous-checked (null) shows the login link. -->
  <div class="flex flex-nowrap items-center gap-2 md:hidden">
    {#if authState.current}
      <span class="max-w-24 truncate text-sm" title={authState.current.email}>
        {authState.current.email}
      </span>
      <button
        class="whitespace-nowrap underline"
        data-testid={TID.navbarLogoutMobile}
        onclick={() => {
          void logout().then(() => location.assign(resolve('/login', {})));
        }}>
        Log out
      </button>
    {:else if authState.current === null}
      <a
        class="whitespace-nowrap text-accent"
        data-testid={TID.navbarLoginMobile}
        href={resolve('/login', {})}>
        Log in
      </a>
    {/if}
  </div>
</nav>
