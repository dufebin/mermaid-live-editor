<script lang="ts">
  import { goto } from '$app/navigation';
  import { base, resolve } from '$app/paths';
  import { Button } from '$/components/ui/button';
  import { Input } from '$/components/ui/input';
  import { TID } from '$/constants';
  import { login, register } from '$/util/auth.svelte';

  let { mode }: { mode: 'login' | 'register' } = $props();

  let email = $state('');
  let password = $state('');
  let confirm = $state('');
  let error = $state('');
  let busy = $state(false);

  const emailOk = (value: string): boolean => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value);
  const passwordOk = (value: string): boolean => value.length >= 8 && value.length <= 128;

  const submit = async () => {
    error = '';
    if (!emailOk(email)) {
      error = 'Enter a valid email address';
      return;
    }
    if (!passwordOk(password)) {
      error = 'Password must be 8-128 characters';
      return;
    }
    if (mode === 'register' && password !== confirm) {
      error = 'Passwords do not match';
      return;
    }
    busy = true;
    try {
      await (mode === 'login' ? login(email, password) : register(email, password));
      const redirect = new URLSearchParams(location.search).get('redirect');
      // Only same-site paths inside `base` are honored; the leading-slash check
      // keeps the check meaningful even when `base` is the empty string, and
      // the scheme-relative check keeps `//evil.com` out (it would otherwise
      // pass both startsWith checks and make goto throw).
      const target =
        redirect !== null &&
        redirect.startsWith('/') &&
        !redirect.startsWith('//') &&
        redirect.startsWith(base)
          ? redirect
          : resolve('/edit', {});
      await goto(target);
    } catch (err) {
      error = err instanceof Error ? err.message : 'Something went wrong';
    } finally {
      busy = false;
    }
  };
</script>

<div class="mx-auto mt-24 w-full max-w-sm rounded-lg border border-border p-6 shadow-sm">
  <h1 class="mb-4 text-center text-xl font-semibold">
    {mode === 'login' ? 'Log in' : 'Create an account'}
  </h1>
  <form
    class="flex flex-col gap-3"
    onsubmit={(event) => {
      event.preventDefault();
      void submit();
    }}>
    <Input
      bind:value={email}
      data-testid={TID.authEmail}
      placeholder="email@example.com"
      type="email" />
    <Input
      bind:value={password}
      data-testid={TID.authPassword}
      placeholder="Password"
      type="password" />
    {#if mode === 'register'}
      <Input
        bind:value={confirm}
        data-testid={TID.authConfirm}
        placeholder="Confirm password"
        type="password" />
    {/if}
    {#if error}
      <p class="text-sm text-destructive" data-testid={TID.authError}>{error}</p>
    {/if}
    <Button data-testid={TID.authSubmit} disabled={busy} type="submit" variant="accent">
      {busy ? 'Please wait…' : mode === 'login' ? 'Log in' : 'Register'}
    </Button>
  </form>
  <!-- location.search carries the ?redirect= param: keep it when switching
       login <-> register so the return path survives the mode switch. -->
  <a
    class="mt-4 block text-center text-sm text-accent hover:underline"
    data-testid={TID.authSwitchLink}
    href={`${base}/${mode === 'login' ? 'register' : 'login'}${location.search}`}>
    {mode === 'login' ? 'Need an account? Register' : 'Already have an account? Log in'}
  </a>
</div>
