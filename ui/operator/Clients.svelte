<script lang="ts">
  import Table from './Table.svelte';
  import SnapshotTime from './SnapshotTime.svelte';
  import type { ClientList } from './types';

  let { data }: { data: ClientList } = $props();
  let expanded = $state<Record<string, boolean>>({});
</script>

<p class="text-muted wrap-anywhere">Node <span class="ml-1 font-mono text-[0.92em] text-foreground">{data.node}</span></p>
<div class="mt-6 mb-2.5 flex flex-wrap items-baseline justify-between gap-x-6 gap-y-1">
  <h2 class="text-[15px] font-semibold">{data.clients.length} connected {data.clients.length === 1 ? 'client' : 'clients'}</h2>
  <p class="text-[13px] text-muted">Observed <SnapshotTime value={data.observedAt} /></p>
</div>
{#if data.clients.length}
  <Table label="Connected clients" tableClass="table-fixed min-[600px]:min-w-[42rem]">
    <thead><tr><th scope="col" class="w-full min-[600px]:w-[74%]">Hostname / client</th><th scope="col" class="hidden min-[600px]:table-cell min-[600px]:w-[18%]">Peer address</th><th scope="col" class="hidden min-[600px]:table-cell min-[600px]:w-[8%]">Version</th></tr></thead>
    <tbody>
      {#each data.clients as client, index (`${client.identity}/${client.address}`)}
        {@const key = `${client.identity}/${client.address}`}
        {@const clientID = client.clientId}
        <tr>
          <th scope="row">
            <div class="flex items-baseline gap-2">
              {#if client.sessionMode}
                <span class={['min-w-0 font-mono text-[0.92em]', client.sessionMode === 'ephemeral' && 'break-all']}>{client.hostname || `Client ${clientID}`}</span>
              {:else}
                <a class="-my-2 inline-flex min-h-10 min-w-0 items-center font-mono text-[0.92em] text-accent-300 underline underline-offset-4 hover:text-accent-100" href={client.url}>Client {clientID}</a>
              {/if}
              <span class="shrink-0 rounded-full border border-line bg-chrome px-2 py-0.5 text-xs leading-4 text-muted"><span class="sr-only">Type: </span>{client.sessionMode === 'ephemeral' ? 'Ephemeral' : client.sessionMode === 'token' ? 'Token' : 'Client'}</span>
            </div>
            <div class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-[13px] text-muted">
              {#if client.sessionMode === 'token' && client.ownerIdentity}
                <span>Owner {#if client.ownerUrl}<a class="text-muted underline underline-offset-4 hover:text-foreground" href={client.ownerUrl}>{client.ownerLabel || client.ownerIdentity}</a>{:else}{client.ownerLabel || client.ownerIdentity}{/if}</span>
                <span aria-hidden="true">·</span>
              {/if}
              <button
                type="button"
                class="-my-1.5 inline-flex min-h-9 shrink-0 items-center gap-1 whitespace-nowrap text-muted hover:text-foreground"
                aria-expanded={Boolean(expanded[key])}
                aria-controls={`client-identity-${index}`}
                onclick={() => { expanded[key] = !expanded[key]; }}
              ><span class="inline-block w-3 text-center" aria-hidden="true">{expanded[key] ? '▾' : '▸'}</span>Identity</button>
            </div>
            <div id={`client-identity-${index}`} hidden={!expanded[key]} class="mt-1 text-[13px] leading-relaxed text-muted">
              <p><span class="mr-2">Connection identity</span><code class="font-mono text-[0.92em] break-all text-foreground">{client.identity}</code></p>
              {#if client.ownerIdentity}<p class="mt-1"><span class="mr-2">Owner identity</span><code class="font-mono text-[0.92em] break-all text-foreground">{client.ownerIdentity}</code></p>{/if}
            </div>
            <div class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted min-[600px]:hidden">
              <span>Peer address <span class="font-mono text-foreground">{client.address}</span></span>
              <span>Version <span class="font-mono text-foreground">{client.version || 'Not reported'}</span></span>
            </div>
          </th>
          <td class="hidden font-mono text-[0.92em] min-[600px]:table-cell">{client.address}</td>
          <td class="hidden font-mono text-[0.92em] min-[600px]:table-cell">
            {#if client.version}{client.version}{:else}<span class="font-sans text-muted">Not reported</span>{/if}
          </td>
        </tr>
      {/each}
    </tbody>
  </Table>
{:else}
  <p class="border-y border-line py-6 text-muted">No clients connected to this node.</p>
{/if}
