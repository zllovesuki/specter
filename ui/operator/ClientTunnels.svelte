<script lang="ts">
  import Table from './Table.svelte';
  import SnapshotTime from './SnapshotTime.svelte';
  import type { TunnelDetail } from './types';

  let { data }: { data: TunnelDetail } = $props();
</script>

<p class="text-muted wrap-anywhere">Client <span class="ml-1 font-mono text-[0.92em] text-foreground">{data.identity}</span></p>
{#if data.configurationError}
  <section class="mt-5 rounded-md border border-line bg-surface px-4 py-3" aria-labelledby="configuration-error">
    <h2 id="configuration-error" class="text-[15px] font-semibold text-warning">Configuration unavailable</h2>
    <details class="mt-1">
      <summary class="min-h-8 cursor-pointer py-1 text-muted">Lookup error</summary>
      <p class="mt-1 max-w-[72ch] font-mono text-xs wrap-anywhere">{data.configurationError}</p>
    </details>
  </section>
{/if}
{#if data.registrationError}
  <section class="mt-5 rounded-md border border-line bg-surface px-4 py-3" aria-labelledby="registration-error">
    <h2 id="registration-error" class="text-[15px] font-semibold text-warning">Registration unavailable</h2>
    <details class="mt-1">
      <summary class="min-h-8 cursor-pointer py-1 text-muted">Lookup error</summary>
      <p class="mt-1 max-w-[72ch] font-mono text-xs wrap-anywhere">{data.registrationError}</p>
    </details>
  </section>
{/if}

<div class="mt-6 mb-2.5 flex flex-wrap items-baseline justify-between gap-x-6 gap-y-1">
  <h2 class="text-[15px] font-semibold">Hostnames</h2>
  <p class="text-[13px] text-muted">Observed <SnapshotTime value={data.observedAt} /></p>
</div>
{#if data.tunnels.length}
  <Table label="Client tunnel configuration and registration">
    <thead><tr><th scope="col">Hostname</th><th scope="col">Configured target</th><th scope="col">Configured</th><th scope="col">Registered</th></tr></thead>
    <tbody>
      {#each data.tunnels as tunnel (tunnel.hostname)}
        <tr>
          <th scope="row" class="font-mono text-[0.92em]">{tunnel.hostname}</th>
          <td>
            {#if tunnel.target}
              <span class="font-mono text-[0.92em]">{tunnel.target}</span>
            {:else}
              <span class="text-muted">{tunnel.configured === 'No' ? 'Not configured' : tunnel.configured === 'Unknown' ? 'Unknown' : 'No target reported'}</span>
            {/if}
          </td>
          <td><span class="text-[13px] whitespace-nowrap" class:text-warning={tunnel.configured === 'Unknown'}>{tunnel.configured}</span></td>
          <td><span class="text-[13px] whitespace-nowrap" class:text-warning={tunnel.registered === 'Unknown'}>{tunnel.registered}</span></td>
        </tr>
      {/each}
    </tbody>
  </Table>
{:else}
  <p class="border-y border-line py-6 text-muted">{data.configurationError || data.registrationError ? 'No tunnel details available.' : 'No configured tunnels or registered hostnames.'}</p>
{/if}
<details class="mt-2.5 text-[13px]">
  <summary class="min-h-10 cursor-pointer py-2 text-muted">State key</summary>
  <dl class="mt-1 grid grid-cols-[max-content_minmax(0,1fr)] gap-x-4 gap-y-1">
    <dt>Configured</dt><dd class="text-muted">Current client configuration</dd>
    <dt>Registered</dt><dd class="text-muted">Hostname ownership record</dd>
    <dt>Unknown</dt><dd class="text-muted">Lookup unavailable</dd>
  </dl>
</details>
