<script lang="ts">
  import Table from './Table.svelte';
  import SnapshotTime from './SnapshotTime.svelte';
  import type { ClientList } from './types';

  let { data }: { data: ClientList } = $props();
</script>

<p class="text-muted wrap-anywhere">Node <span class="ml-1 font-mono text-[0.92em] text-foreground">{data.node}</span></p>
<div class="mt-6 mb-2.5 flex flex-wrap items-baseline justify-between gap-x-6 gap-y-1">
  <h2 class="text-[15px] font-semibold">{data.clients.length} connected {data.clients.length === 1 ? 'client' : 'clients'}</h2>
  <p class="text-[13px] text-muted">Observed <SnapshotTime value={data.observedAt} /></p>
</div>
{#if data.clients.length}
  <Table label="Connected clients">
    <thead><tr><th scope="col">Client identity</th><th scope="col">Peer address</th><th scope="col">Version</th></tr></thead>
    <tbody>
      {#each data.clients as client (`${client.identity}/${client.address}`)}
        <tr>
          <th scope="row"><a class="-my-2 inline-flex min-h-10 items-center font-mono text-[0.92em] text-accent-300 underline underline-offset-4 hover:text-accent-100" href={client.url}>{client.identity}</a></th>
          <td class="font-mono text-[0.92em]">{client.address}</td>
          <td class="font-mono text-[0.92em]">
            {#if client.version}{client.version}{:else}<span class="font-sans text-muted">Not reported</span>{/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </Table>
{:else}
  <p class="border-y border-line py-6 text-muted">No clients connected to this node.</p>
{/if}
