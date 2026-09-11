<script lang="ts">
  import { onMount } from 'svelte';
  import Button from '../shared/Button.svelte';
  import Shell from '../shared/Shell.svelte';
  import Overview from './Overview.svelte';
  import Clients from './Clients.svelte';
  import ClientTunnels from './ClientTunnels.svelte';
  import type { ClientList, Overview as OverviewData, Snapshot, TunnelDetail } from './types';

  const path = window.location.pathname;
  const kind = /^\/_internal\/tun\/.+/.test(path)
    ? 'tunnels'
    : /^\/_internal\/tun\/?$/.test(path)
      ? 'clients'
      : 'overview';
  const endpoint = kind === 'overview'
    ? '/_internal/overview.json'
    : path.replace(/^\/_internal\/tun/, '/_internal/api/tun');
  const title = kind === 'overview' ? 'Server overview' : kind === 'clients' ? 'Connected clients' : 'Client tunnels';
  const navigation = [
    { label: 'Overview', href: '/_internal/', active: kind === 'overview' },
    { label: 'Clients', href: '/_internal/tun/', active: kind !== 'overview' },
    { label: 'Endpoint reference', href: '/_internal/endpoints' },
  ];

  let snapshot = $state<Snapshot | null>(null);
  let loading = $state(true);
  let error = $state('');
  let pending: AbortController | undefined;

  async function refresh() {
    pending?.abort();
    const controller = new AbortController();
    pending = controller;
    let timedOut = false;
    const timeout = window.setTimeout(() => {
      timedOut = true;
      controller.abort();
    }, 12_000);
    loading = true;
    error = '';

    try {
      const response = await fetch(endpoint, {
        signal: controller.signal,
        credentials: 'same-origin',
        cache: 'no-store',
        headers: { Accept: 'application/json' },
      });
      if (!response.ok) {
        throw new Error(response.status === 401
          ? 'Authentication required (401).'
          : response.status === 403
            ? 'Access denied (403).'
            : `Server returned ${response.status}.`);
      }
      const data = await response.json();
      if (!data || typeof data !== 'object' || !Array.isArray(data[kind === 'overview' ? 'vnodes' : kind === 'clients' ? 'clients' : 'tunnels'])) {
        throw new Error('Unexpected server response.');
      }
      if (controller.signal.aborted) return;
      snapshot = kind === 'overview'
        ? { kind, data: data as OverviewData }
        : kind === 'clients'
          ? { kind, data: data as ClientList }
          : { kind, data: data as TunnelDetail };
    } catch (cause) {
      if (controller.signal.aborted && !timedOut) return;
      error = timedOut ? 'Request timed out.' : cause instanceof Error ? cause.message : 'Request failed.';
    } finally {
      window.clearTimeout(timeout);
      if (pending === controller) {
        pending = undefined;
        loading = false;
      }
    }
  }

  onMount(() => {
    void refresh();
    return () => pending?.abort();
  });
</script>

<svelte:head><title>{title} · Specter</title></svelte:head>

<Shell label="Operator" home="/_internal/" {navigation}>
  {#if kind === 'tunnels'}
    <a class="mb-2 inline-flex min-h-10 items-center text-accent-300 underline underline-offset-4 hover:text-accent-100" href="/_internal/tun/">← Back to clients</a>
  {/if}
  <div class="mb-2 flex flex-wrap items-center justify-between gap-3">
    <h1 class="text-[22px] leading-tight font-semibold tracking-tight sm:text-2xl">{title}</h1>
    <div class="flex flex-wrap items-center gap-2">
      {#if kind === 'overview'}
        <a class="inline-flex min-h-10 items-center rounded-md border border-zinc-500 bg-surface px-3 py-1.5 text-sm leading-5 font-semibold text-foreground hover:border-zinc-400 hover:bg-zinc-700" href={endpoint}>View JSON</a>
      {/if}
      <Button variant="secondary" onclick={refresh} disabled={loading}>{loading ? 'Refreshing…' : 'Refresh'}</Button>
    </div>
  </div>
  {#if error}
    <div class="my-4 rounded-md border border-danger/40 bg-surface px-4 py-3" role="alert">
      <p class="font-medium text-danger">{snapshot ? 'Refresh failed. Showing previous snapshot.' : 'Could not load this view.'}</p>
      <details class="mt-1">
        <summary class="min-h-8 cursor-pointer py-1 text-muted">Request error</summary>
        <p class="mt-1 max-w-[72ch] font-mono text-xs wrap-anywhere">{error}</p>
      </details>
    </div>
  {/if}
  <div aria-busy={loading}>
    {#if snapshot?.kind === 'overview'}
      <Overview data={snapshot.data} />
    {:else if snapshot?.kind === 'clients'}
      <Clients data={snapshot.data} />
    {:else if snapshot?.kind === 'tunnels'}
      <ClientTunnels data={snapshot.data} />
    {:else if loading}
      <div class="mt-5 space-y-3" role="status">
        <span class="sr-only">Loading {title.toLowerCase()}…</span>
        <div class="h-4 w-48 max-w-full rounded bg-surface" aria-hidden="true"></div>
        <div class="h-36 rounded-md border border-line bg-chrome" aria-hidden="true"></div>
      </div>
    {/if}
  </div>
</Shell>
