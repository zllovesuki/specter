<script lang="ts">
  import type { SaveOutcome } from './editor.svelte';
  let { outcome, saved, failed, class: className = 'mt-4 bg-surface px-4 py-3' }: {
    outcome: SaveOutcome<unknown> | null;
    saved: string;
    failed: string;
    class?: string;
  } = $props();
  const tone = $derived(outcome?.kind === 'saved' ? 'success' : outcome?.kind === 'unknown' ? 'warning' : 'error');
</script>

{#if outcome && outcome.kind !== 'conflict'}
  <div class={['rounded-md border', className, tone === 'success' ? 'border-success/40' : tone === 'warning' ? 'border-warning/40' : 'border-danger/40']} role={tone === 'success' ? 'status' : 'alert'}>
    <p class="font-medium wrap-anywhere" class:text-success={tone === 'success'} class:text-warning={tone === 'warning'} class:text-danger={tone === 'error'}>
      {outcome.kind === 'saved' ? saved : outcome.kind === 'unknown' ? 'Save result unknown. Your draft is unchanged.' : failed}
    </p>
    {#if outcome.kind !== 'saved'}
      <details class="mt-1"><summary class="min-h-8 cursor-pointer py-1 text-muted">Request details</summary><p class="mt-1 max-w-[72ch] text-sm wrap-anywhere">{outcome.error.message}</p></details>
    {/if}
  </div>
{/if}
