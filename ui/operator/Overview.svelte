<script lang="ts">
  import Table from './Table.svelte';
  import SnapshotTime from './SnapshotTime.svelte';
  import type { Overview } from './types';

  let { data }: { data: Overview } = $props();

  function stateClass(state: string) {
    if (state === 'Active') return 'border-success/40 text-success';
    if (['Joining', 'Transferring', 'Leaving'].includes(state)) {
      return 'border-warning/40 text-warning';
    }
    return 'border-zinc-500 text-foreground';
  }
</script>

<p class="text-muted">Local snapshot · <SnapshotTime value={data.observed_at} /></p>

<dl class="mt-4 mb-5 grid gap-3 border-y border-line py-3 sm:grid-cols-[2fr_1fr_1fr] sm:gap-5">
  <div class="min-w-0">
    <dt class="mb-0.5 text-xs text-muted">Physical server</dt>
    <dd class="font-mono text-[0.92em] wrap-anywhere">{data.identity.address}</dd>
  </div>
  <div class="min-w-0">
    <dt class="mb-0.5 text-xs text-muted">Root virtual node</dt>
    <dd class="font-mono text-[0.92em] wrap-anywhere">{data.identity.id}</dd>
  </div>
  <div class="min-w-0">
    <dt class="mb-0.5 text-xs text-muted">Storage provider</dt>
    <dd>{data.provider}</dd>
  </div>
</dl>

<div class="mt-5 mb-2.5 flex flex-wrap items-baseline gap-x-4 gap-y-1">
  <h2 class="text-lg font-semibold">Virtual nodes ({data.vnodes.length})</h2>
  <span class="text-[13px] text-muted">Counters since process start</span>
</div>
<Table label="Virtual node counters" tableClass="min-w-[60rem]">
  <thead>
    <tr>
      <th scope="col">Node</th>
      <th scope="col">Membership</th>
      <th scope="col">Last stabilization</th>
      <th scope="col" class="text-right">Chord requests</th>
      <th scope="col" class="text-right">KV requests</th>
      <th scope="col" class="text-right">RPC errors</th>
      <th scope="col" class="text-right">Stale KV</th>
    </tr>
  </thead>
  <tbody>
    {#each data.vnodes as node (node.identity.id)}
      <tr>
        <th scope="row">
          <code class="text-[0.92em]">{node.identity.id}</code>
          {#if node.root}<span class="block text-xs text-muted">Root</span>{/if}
        </th>
        <td><span class="inline-block rounded border px-1.5 py-px text-[13px] whitespace-nowrap {stateClass(node.state)}">{node.state}</span></td>
        <td>
          {#if node.last_stabilized}
            <span class="font-mono text-[0.92em]"><SnapshotTime value={node.last_stabilized} /></span>
          {:else}
            <span class="text-muted">Not observed</span>
          {/if}
        </td>
        <td class="text-right">{node.chord_requests.toLocaleString()}</td>
        <td class="text-right">{node.kv_requests.toLocaleString()}</td>
        <td class="text-right" class:text-warning={node.rpc_errors > 0}>{node.rpc_errors.toLocaleString()}</td>
        <td class="text-right" class:text-warning={node.kv_stale > 0}>{node.kv_stale.toLocaleString()}</td>
      </tr>
    {:else}
      <tr><td colspan="7" class="text-muted">No virtual nodes.</td></tr>
    {/each}
  </tbody>
</Table>

<h2 class="mt-5 mb-2.5 text-lg font-semibold">Known neighbors</h2>
<Table label="Known neighbors">
  <thead><tr><th scope="col">Local node</th><th scope="col">Predecessor</th><th scope="col">Successors</th></tr></thead>
  <tbody>
    {#each data.vnodes as node (node.identity.id)}
      <tr>
        <th scope="row"><code class="text-[0.92em]">{node.identity.id}</code></th>
        <td>
          {#if !node.predecessor_available}
            <span class="text-warning">Unavailable</span>
          {:else if node.predecessor}
            <div class="min-w-48">
              <code class="text-[0.92em]">{node.predecessor.id}</code>
              <small class="block text-muted">{node.predecessor.address}</small>
            </div>
          {:else}
            <span class="text-muted">Not observed</span>
          {/if}
        </td>
        <td>
          {#if !node.successors_available}
            <span class="text-warning">Unavailable</span>
          {:else}
            {#each node.successors as peer}
              <div class="mb-1.5 min-w-48 last:mb-0">
                <code class="text-[0.92em]">{peer.id}</code>
                <small class="block text-muted">{peer.address}</small>
              </div>
            {:else}
              <span class="text-muted">Not observed</span>
            {/each}
          {/if}
        </td>
      </tr>
    {:else}
      <tr><td colspan="3" class="text-muted">No neighbors.</td></tr>
    {/each}
  </tbody>
</Table>

<h2 class="mt-5 mb-2 text-lg font-semibold">Diagnostics</h2>
<div class="flex flex-wrap items-center gap-x-6 gap-y-1">
  <span><a class="inline-flex min-h-10 items-center text-accent-300 underline underline-offset-4 hover:text-accent-100" href="/_internal/chord/stats">Ring statistics</a> <small class="text-muted">Scans keys</small></span>
  <span><a class="inline-flex min-h-10 items-center text-accent-300 underline underline-offset-4 hover:text-accent-100" href="/_internal/chord/graph">Ring graph (DOT)</a> <small class="text-muted">Queries ring</small></span>
  <a class="inline-flex min-h-10 items-center text-accent-300 underline underline-offset-4 hover:text-accent-100" href="/_internal/debug/pprof/">Runtime profiles</a>
</div>
