<script lang="ts">
  import { createQuery } from '@tanstack/svelte-query';
  import Button from '../shared/Button.svelte';
  import Shell from '../shared/Shell.svelte';
  import { RequestError, requestJSON } from '../shared/http';
  import Overview from './Overview.svelte';
  import Clients from './Clients.svelte';
  import ClientTunnels from './ClientTunnels.svelte';
  import DNSConfiguration from './DNSConfiguration.svelte';
  import WAFConfiguration from './WAFConfiguration.svelte';
  import type { ClientList, Overview as OverviewData, Snapshot, TunnelDetail } from './types';

  const path = window.location.pathname;
  const kind = /^\/_internal\/config\/dns\/?$/.test(path)
    ? 'dns'
    : /^\/_internal\/config\/waf\/?$/.test(path)
      ? 'waf'
    : /^\/_internal\/tun\/.+/.test(path)
      ? 'tunnels'
      : /^\/_internal\/tun\/?$/.test(path)
        ? 'clients'
        : 'overview';
  const endpoint = kind === 'overview'
    ? '/_internal/overview.json'
    : path.replace(/^\/_internal\/tun/, '/_internal/api/tun');
  const title = kind === 'dns' ? 'ACME DNS' : kind === 'waf' ? 'Web protection' : kind === 'overview' ? 'Server overview' : kind === 'clients' ? 'Connected clients' : 'Client tunnels';
  const navigation = [
    { label: 'Overview', href: '/_internal/', active: kind === 'overview' },
    { label: 'Clients', href: '/_internal/tun/', active: kind === 'clients' || kind === 'tunnels' },
    { label: 'ACME DNS', href: '/_internal/config/dns', active: kind === 'dns' },
    { label: 'Web protection', href: '/_internal/config/waf', active: kind === 'waf' },
    { label: 'Endpoint reference', href: '/_internal/endpoints' },
  ];

  const query = createQuery(() => ({
    queryKey: ['operator', 'snapshot', endpoint],
    enabled: kind !== 'dns' && kind !== 'waf',
    queryFn: async ({ signal }): Promise<Snapshot> => {
      const data = await requestJSON<unknown>(endpoint, { signal });
      if (!data || typeof data !== 'object' || !Array.isArray((data as Record<string, unknown>)[kind === 'overview' ? 'vnodes' : kind === 'clients' ? 'clients' : 'tunnels'])) {
        throw new Error('Unexpected server response.');
      }
      return kind === 'overview'
        ? { kind, data: data as OverviewData }
        : kind === 'clients'
          ? { kind, data: data as ClientList }
          : { kind: 'tunnels', data: data as TunnelDetail };
    },
  }));
  const snapshot = $derived(query.data);
  const loading = $derived(query.isFetching);
  const error = $derived(loading ? '' : query.error instanceof RequestError && query.error.status === 401
    ? 'Authentication required (401).'
    : query.error instanceof RequestError && query.error.status === 403 ? 'Access denied (403).' : query.error?.message ?? '');
</script>

<svelte:head><title>{title} · Specter</title></svelte:head>

<Shell label="Operator" home="/_internal/" {navigation}>
  {#if kind === 'dns'}
    <DNSConfiguration />
  {:else if kind === 'waf'}
    <WAFConfiguration />
  {:else}
  {#if kind === 'tunnels'}
    <a class="mb-2 inline-flex min-h-10 items-center text-accent-300 underline underline-offset-4 hover:text-accent-100" href="/_internal/tun/">← Back to clients</a>
  {/if}
  <div class="mb-2 flex flex-wrap items-center justify-between gap-3">
    <h1 class="text-[22px] leading-tight font-semibold tracking-tight sm:text-2xl">{title}</h1>
    <div class="flex flex-wrap items-center gap-2">
      {#if kind === 'overview'}
        <a class="inline-flex min-h-10 items-center rounded-md border border-zinc-500 bg-surface px-3 py-1.5 text-sm leading-5 font-semibold text-foreground hover:border-zinc-400 hover:bg-zinc-700" href={endpoint}>View JSON</a>
      {/if}
      <Button variant="secondary" onclick={() => query.refetch()} disabled={loading}>{loading ? 'Refreshing…' : 'Refresh'}</Button>
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
  {/if}
</Shell>
