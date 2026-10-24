<script lang="ts">
  import { tick } from 'svelte';
  import Button from '../shared/Button.svelte';
  import SaveNotice from '../shared/SaveNotice.svelte';
  import { createEditor } from '../shared/editor.svelte';
  import { INPUT_CLASSES } from '../shared/styles';
  import SnapshotTime from './SnapshotTime.svelte';
  import { requestDNSConfig } from './dns-api';

  type Draft = { zone: string; nameservers: { host: string; addresses: string }[] };
  let form = $state<HTMLFormElement>();
  const editor = createEditor({
    key: () => '/_internal/api/config/dns',
    defaults: { zone: '', nameservers: [{ host: '', addresses: '' }] } as Draft,
    load: (signal, after) => requestDNSConfig(signal, undefined, after),
    save: (value, version, signal) => requestDNSConfig(signal, JSON.stringify({
      expected_version: version,
      config: { zone: value.zone.trim(), nameservers: value.nameservers.map(ns => ({ host: ns.host.trim(), addresses: ns.addresses.split(/[\s,]+/).filter(Boolean) })) },
    })),
    draft: snapshot => ({ zone: snapshot.config?.zone ?? '', nameservers: (snapshot.config?.nameservers ?? [{ host: '', addresses: [] }]).map(ns => ({ host: ns.host, addresses: ns.addresses.join('\n') })) }),
    editable: snapshot => !snapshot.error && !(snapshot.version && !snapshot.config),
  });
  const snapshot = $derived(editor.query.data);
  const draft = $derived(editor.values);
  const fields = $derived(editor.fields);
  const readError = $derived(editor.query.error?.message ?? '');

  function fieldError(path: string): string {
    return Object.entries(fields).filter(([key]) => key === path || key.startsWith(`${path}.`)).map(([, message]) => message).join(' ');
  }

  async function save(event: SubmitEvent) {
    await editor.submit(event);
    if (Object.keys(fields).length) {
      await tick();
      form?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus();
    }
  }
</script>

<div class="mb-2 flex flex-wrap items-center justify-between gap-3">
  <h1 class="text-[22px] leading-tight font-semibold tracking-tight sm:text-2xl">ACME DNS</h1>
  <Button variant="secondary" onclick={() => editor.refresh()} disabled={editor.query.isFetching || editor.saving}>{editor.query.isFetching ? 'Refreshing…' : 'Refresh'}</Button>
</div>

{#if snapshot}
  <p class="text-sm text-muted wrap-anywhere">
    {snapshot.version ? 'Saved on' : 'Server'} <code class="text-foreground">{snapshot.server.address}</code>
    · <SnapshotTime value={snapshot.observed_at} />
  </p>
{/if}

{#if readError}
  <div class="my-4 rounded-md border border-danger/40 bg-surface px-4 py-3" role="alert">
    <p class="font-medium text-danger">{snapshot ? 'Refresh failed. Showing previous saved state.' : 'Could not load DNS configuration.'}</p>
    <details class="mt-1">
      <summary class="min-h-8 cursor-pointer py-1 text-muted">Request error</summary>
      <p class="mt-1 max-w-[72ch] font-mono text-xs wrap-anywhere">{readError}</p>
    </details>
  </div>
{/if}

{#if snapshot}
  {#if editor.readOnly}
    <div class="mt-4 rounded-md border border-warning/40 bg-surface px-4 py-3" role="alert">
      <p class="font-medium text-warning">Saved configuration cannot be edited by this server.</p>
      <p class="mt-1 max-w-[72ch] text-sm wrap-anywhere">{snapshot.error || 'The saved DNS document could not be read.'}</p>
    </div>
  {:else if editor.initialized}
    {#if !snapshot.version}
      <p class="mt-4 text-sm text-muted">No DNS configuration saved. Add the zone and nameservers to create it.</p>
    {/if}

    {#if editor.stale}
      <div class="mt-4 rounded-md border border-warning/40 bg-surface px-4 py-3" role="alert">
        <p class="font-medium text-warning">Saved configuration changed. Your draft is unchanged.</p>
        <p class="mt-1 text-sm">Load the latest saved values before editing again.</p>
      </div>
    {/if}

    <SaveNotice outcome={editor.outcome} saved={editor.outcome?.kind === 'saved' ? `Saved on ${editor.outcome.snapshot.server.address}.` : ''} failed={Object.keys(fields).length ? 'Check the highlighted fields.' : 'Could not save DNS configuration.'} />

    {#if editor.stale}
      <details class="mt-3 rounded-md border border-line px-4 py-2" open>
        <summary class="min-h-8 cursor-pointer py-1 font-medium">Latest saved values</summary>
        {#if snapshot.config}
          <p class="mt-2 text-sm text-muted">Zone <code class="text-foreground">{snapshot.config.zone}</code></p>
          <dl class="my-3 grid gap-x-4 gap-y-2 text-sm sm:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
            {#each snapshot.config.nameservers as ns}
              <dt class="font-mono wrap-anywhere">{ns.host}</dt>
              <dd class="font-mono text-muted wrap-anywhere">{ns.addresses.join(', ')}</dd>
            {/each}
          </dl>
        {:else}
          <p class="my-2 text-sm text-muted">No DNS configuration saved.</p>
        {/if}
        <Button size="sm" onclick={() => editor.accept()} disabled={editor.query.isFetching || editor.saving || Boolean(readError)}>Load latest and discard draft</Button>
      </details>
    {/if}

    <form bind:this={form} onsubmit={save} novalidate class="mt-5" aria-label="DNS configuration">
      <fieldset disabled={editor.saving}>
        <legend class="sr-only">DNS records</legend>
        <div class="max-w-lg">
          <label for="dns-zone" class="mb-1 block text-sm font-medium">Zone</label>
          <input id="dns-zone" class={INPUT_CLASSES + ' font-mono'} value={draft.zone} oninput={(event) => { editor.form.setFieldValue('zone', event.currentTarget.value); editor.edited(); }} autocomplete="off" autocapitalize="none" spellcheck={false} required readonly={Boolean(snapshot.config)} aria-invalid={Boolean(fields.zone)} aria-describedby={fields.zone ? 'dns-zone-error' : undefined} />
          {#if fields.zone}<p id="dns-zone-error" class="mt-1 text-sm text-danger">{fields.zone}</p>{/if}
        </div>

        <div class="mt-5 mb-2 flex flex-wrap items-center justify-between gap-2">
          <h2 class="text-lg font-semibold">Nameservers</h2>
          <Button size="sm" onclick={() => { editor.form.pushFieldValue('nameservers', { host: '', addresses: '' }); editor.edited(); }}>Add nameserver</Button>
        </div>
        {#if fields.nameservers}<p class="mb-2 text-sm text-danger">{fields.nameservers}</p>{/if}
        <div class="border-t border-line">
          {#each draft.nameservers as ns, index}
            {@const hostError = fieldError(`nameservers.${index}.host`)}
            {@const addressError = fieldError(`nameservers.${index}.addresses`)}
            <div class="grid items-start gap-3 border-b border-line py-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_auto]">
              <div class="min-w-0">
                <label for={`dns-host-${index}`} class="mb-1 block text-sm font-medium">Nameserver {index + 1} (NS)</label>
                <input id={`dns-host-${index}`} class={INPUT_CLASSES + ' font-mono'} value={ns.host} oninput={(event) => { editor.form.setFieldValue(`nameservers[${index}].host`, event.currentTarget.value); editor.edited(); }} autocomplete="off" autocapitalize="none" spellcheck={false} required aria-invalid={Boolean(hostError)} aria-describedby={hostError ? `dns-host-error-${index}` : undefined} />
                {#if hostError}<p id={`dns-host-error-${index}`} class="mt-1 text-sm text-danger">{hostError}</p>{/if}
              </div>
              <div class="min-w-0">
                <label for={`dns-address-${index}`} class="mb-1 block text-sm font-medium">IP addresses (A / AAAA)</label>
                <textarea id={`dns-address-${index}`} class={INPUT_CLASSES + ' block resize-y font-mono'} rows="2" value={ns.addresses} oninput={(event) => { editor.form.setFieldValue(`nameservers[${index}].addresses`, event.currentTarget.value); editor.edited(); }} autocomplete="off" autocapitalize="none" spellcheck={false} required aria-invalid={Boolean(addressError)} aria-describedby={`dns-address-help-${index}${addressError ? ` dns-address-error-${index}` : ''}`}></textarea>
                <p id={`dns-address-help-${index}`} class="mt-1 text-xs text-muted">One address per line.</p>
                {#if addressError}<p id={`dns-address-error-${index}`} class="mt-1 text-sm text-danger">{addressError}</p>{/if}
              </div>
              <Button size="sm" variant="ghost" class="justify-self-start sm:mt-6" disabled={draft.nameservers.length === 1} aria-label={`Remove nameserver ${index + 1}`} onclick={() => { editor.form.removeFieldValue('nameservers', index); editor.edited(); }}>Remove</Button>
            </div>
          {/each}
        </div>
      </fieldset>
      <div class="mt-4 flex flex-wrap items-center gap-2">
        <Button variant="primary" type="submit" disabled={!editor.canSave}>{editor.saving ? 'Saving…' : 'Save DNS configuration'}</Button>
        {#if editor.dirty && !editor.stale}<Button variant="ghost" onclick={() => editor.accept()} disabled={editor.saving}>Discard changes</Button>{/if}
        {#if editor.dirty}<span class="text-sm text-muted">Unsaved changes</span>{/if}
      </div>
    </form>
  {/if}
{:else if editor.query.isFetching}
  <div class="mt-5 space-y-3" role="status">
    <span class="sr-only">Loading DNS configuration…</span>
    <div class="h-4 w-48 max-w-full rounded bg-surface" aria-hidden="true"></div>
    <div class="h-36 rounded-md border border-line bg-chrome" aria-hidden="true"></div>
  </div>
{/if}
