<script lang="ts">
  import { onMount, tick } from 'svelte';
  import Button from '../shared/Button.svelte';
  import { INPUT_CLASSES } from '../shared/styles';
  import SnapshotTime from './SnapshotTime.svelte';
  import { DNSRequestError, requestDNSConfig } from './dns-api';
  import type { DNSConfig, DNSConfigSnapshot } from './types';

  type Draft = { zone: string; nameservers: { key: number; host: string; addresses: string }[] };
  type Notice = { tone: 'success' | 'warning' | 'error'; message: string; detail?: string };

  let snapshot = $state<DNSConfigSnapshot | null>(null);
  let draft = $state<Draft>({ zone: '', nameservers: [] });
  let baseline = $state('');
  let expectedVersion = $state('');
  let initialized = $state(false);
  let loading = $state(true);
  let saving = $state(false);
  let readError = $state('');
  let notice = $state<Notice | null>(null);
  let fields = $state<Record<string, string>>({});
  let conflict = $state(false);
  let form = $state<HTMLFormElement>();
  let pendingRead: AbortController | undefined;
  let pendingWrite: AbortController | undefined;
  let nextKey = 0;
  let disposed = false;

  const draftValue = $derived(JSON.stringify({ zone: draft.zone, nameservers: draft.nameservers.map(({ host, addresses }) => ({ host, addresses })) }));
  const dirty = $derived(initialized && draftValue !== baseline);
  const readOnly = $derived(Boolean(snapshot?.error || (snapshot?.version && !snapshot.config)));
  const stale = $derived(conflict || (initialized && snapshot !== null && snapshot.version !== expectedVersion));
  const canSave = $derived(initialized && dirty && !saving && !loading && !readOnly && !stale);

  function loadSaved() {
    if (!snapshot || readOnly) return;
    const config = snapshot.config;
    draft = {
      zone: config?.zone ?? '',
      nameservers: (config?.nameservers ?? [{ host: '', addresses: [] }]).map(ns => ({
        key: nextKey++, host: ns.host, addresses: ns.addresses.join('\n'),
      })),
    };
    baseline = JSON.stringify({ zone: draft.zone, nameservers: draft.nameservers.map(({ host, addresses }) => ({ host, addresses })) });
    expectedVersion = snapshot.version;
    initialized = true;
    conflict = false;
    fields = {};
    notice = null;
  }

  async function refresh() {
    if (saving) return;
    pendingRead?.abort();
    const controller = new AbortController();
    pendingRead = controller;
    const timeout = window.setTimeout(() => controller.abort(), 12_000);
    loading = true;
    try {
      const data = await requestDNSConfig(controller.signal, undefined, snapshot?.version);
      if (disposed || pendingRead !== controller) return;
      const replaceDraft = !initialized || (!dirty && !conflict && data.version !== expectedVersion);
      snapshot = data;
      readError = '';
      if (replaceDraft) loadSaved();
    } catch (cause) {
      if (disposed || pendingRead !== controller) return;
      readError = cause instanceof Error ? cause.message : 'Request failed.';
    } finally {
      window.clearTimeout(timeout);
      if (pendingRead === controller) {
        pendingRead = undefined;
        loading = false;
      }
    }
  }

  function configFromDraft(): DNSConfig {
    return {
      zone: draft.zone.trim(),
      nameservers: draft.nameservers.map(ns => ({ host: ns.host.trim(), addresses: ns.addresses.split(/[\s,]+/).filter(Boolean) })),
    };
  }

  function fieldError(path: string): string {
    return Object.entries(fields).filter(([key]) => key === path || key.startsWith(`${path}.`)).map(([, message]) => message).join(' ');
  }

  async function focusError() {
    await tick();
    form?.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus();
  }

  function edited() {
    fields = {};
    if (notice?.tone === 'success' || notice?.tone === 'error') notice = null;
  }

  async function save(event: SubmitEvent) {
    event.preventDefault();
    if (!canSave) return;
    const config = configFromDraft();
    fields = {};
    saving = true;
    notice = null;
    const controller = new AbortController();
    pendingWrite = controller;
    const timeout = window.setTimeout(() => controller.abort(), 15_000);
    let reread = false;
    try {
      const data = await requestDNSConfig(controller.signal, JSON.stringify({ expected_version: expectedVersion, config }));
      if (disposed) return;
      snapshot = data;
      readError = '';
      loadSaved();
      notice = { tone: 'success', message: `Saved on ${data.server.address}.` };
    } catch (cause) {
      if (disposed) return;
      const error = cause instanceof DNSRequestError ? cause : new DNSRequestError('Save result unknown.', 'outcome_unknown');
      fields = error.fields;
      if (error.code === 'version_conflict') {
        conflict = true;
        notice = null;
        reread = true;
      } else if (error.code === 'outcome_unknown') {
        notice = { tone: 'warning', message: 'Save result unknown. Your draft is unchanged.', detail: error.message };
      } else {
        notice = { tone: 'error', message: error.code === 'validation_failed' ? 'Check the highlighted fields.' : 'Could not save DNS configuration.', detail: error.message };
      }
    } finally {
      window.clearTimeout(timeout);
      pendingWrite = undefined;
      saving = false;
    }
    if (Object.keys(fields).length) await focusError();
    if (reread) await refresh();
  }

  onMount(() => {
    void refresh();
    return () => {
      disposed = true;
      pendingRead?.abort();
      pendingWrite?.abort();
    };
  });
</script>

<div class="mb-2 flex flex-wrap items-center justify-between gap-3">
  <h1 class="text-[22px] leading-tight font-semibold tracking-tight sm:text-2xl">ACME DNS</h1>
  <Button variant="secondary" onclick={refresh} disabled={loading || saving}>{loading ? 'Refreshing…' : 'Refresh'}</Button>
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
  {#if readOnly}
    <div class="mt-4 rounded-md border border-warning/40 bg-surface px-4 py-3" role="alert">
      <p class="font-medium text-warning">Saved configuration cannot be edited by this server.</p>
      <p class="mt-1 max-w-[72ch] text-sm wrap-anywhere">{snapshot.error || 'The saved DNS document could not be read.'}</p>
    </div>
  {:else if initialized}
    {#if !snapshot.version}
      <p class="mt-4 text-sm text-muted">No DNS configuration saved. Add the zone and nameservers to create it.</p>
    {/if}

    {#if stale}
      <div class="mt-4 rounded-md border border-warning/40 bg-surface px-4 py-3" role="alert">
        <p class="font-medium text-warning">Saved configuration changed. Your draft is unchanged.</p>
        <p class="mt-1 text-sm">Load the latest saved values before editing again.</p>
      </div>
    {/if}

    {#if notice}
      <div class={['mt-4 rounded-md border bg-surface px-4 py-3', notice.tone === 'success' ? 'border-success/40' : notice.tone === 'warning' ? 'border-warning/40' : 'border-danger/40']} role={notice.tone === 'success' ? 'status' : 'alert'}>
        <p class="font-medium wrap-anywhere" class:text-success={notice.tone === 'success'} class:text-warning={notice.tone === 'warning'} class:text-danger={notice.tone === 'error'}>{notice.message}</p>
        {#if notice.detail}
          <details class="mt-1">
            <summary class="min-h-8 cursor-pointer py-1 text-muted">Request details</summary>
            <p class="mt-1 max-w-[72ch] font-mono text-xs wrap-anywhere">{notice.detail}</p>
          </details>
        {/if}
      </div>
    {/if}

    {#if stale}
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
        <Button size="sm" onclick={loadSaved} disabled={loading || saving || Boolean(readError)}>Load latest and discard draft</Button>
      </details>
    {/if}

    <form bind:this={form} onsubmit={save} novalidate class="mt-5" aria-label="DNS configuration">
      <fieldset disabled={saving}>
        <legend class="sr-only">DNS records</legend>
        <div class="max-w-lg">
          <label for="dns-zone" class="mb-1 block text-sm font-medium">Zone</label>
          <input id="dns-zone" class={INPUT_CLASSES + ' font-mono'} bind:value={draft.zone} oninput={edited} autocomplete="off" autocapitalize="none" spellcheck={false} required readonly={Boolean(snapshot.config)} aria-invalid={Boolean(fields.zone)} aria-describedby={fields.zone ? 'dns-zone-error' : undefined} />
          {#if fields.zone}<p id="dns-zone-error" class="mt-1 text-sm text-danger">{fields.zone}</p>{/if}
        </div>

        <div class="mt-5 mb-2 flex flex-wrap items-center justify-between gap-2">
          <h2 class="text-lg font-semibold">Nameservers</h2>
          <Button size="sm" onclick={() => { draft.nameservers.push({ key: nextKey++, host: '', addresses: '' }); edited(); }}>Add nameserver</Button>
        </div>
        {#if fields.nameservers}<p class="mb-2 text-sm text-danger">{fields.nameservers}</p>{/if}
        <div class="border-t border-line">
          {#each draft.nameservers as ns, index (ns.key)}
            {@const hostError = fieldError(`nameservers.${index}.host`)}
            {@const addressError = fieldError(`nameservers.${index}.addresses`)}
            <div class="grid items-start gap-3 border-b border-line py-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_auto]">
              <div class="min-w-0">
                <label for={`dns-host-${ns.key}`} class="mb-1 block text-sm font-medium">Nameserver {index + 1} (NS)</label>
                <input id={`dns-host-${ns.key}`} class={INPUT_CLASSES + ' font-mono'} bind:value={ns.host} oninput={edited} autocomplete="off" autocapitalize="none" spellcheck={false} required aria-invalid={Boolean(hostError)} aria-describedby={hostError ? `dns-host-error-${ns.key}` : undefined} />
                {#if hostError}<p id={`dns-host-error-${ns.key}`} class="mt-1 text-sm text-danger">{hostError}</p>{/if}
              </div>
              <div class="min-w-0">
                <label for={`dns-address-${ns.key}`} class="mb-1 block text-sm font-medium">IP addresses (A / AAAA)</label>
                <textarea id={`dns-address-${ns.key}`} class={INPUT_CLASSES + ' block resize-y font-mono'} rows="2" bind:value={ns.addresses} oninput={edited} autocomplete="off" autocapitalize="none" spellcheck={false} required aria-invalid={Boolean(addressError)} aria-describedby={`dns-address-help-${ns.key}${addressError ? ` dns-address-error-${ns.key}` : ''}`}></textarea>
                <p id={`dns-address-help-${ns.key}`} class="mt-1 text-xs text-muted">One address per line.</p>
                {#if addressError}<p id={`dns-address-error-${ns.key}`} class="mt-1 text-sm text-danger">{addressError}</p>{/if}
              </div>
              <Button size="sm" variant="ghost" class="justify-self-start sm:mt-6" disabled={draft.nameservers.length === 1} aria-label={`Remove nameserver ${index + 1}`} onclick={() => { draft.nameservers.splice(index, 1); edited(); }}>Remove</Button>
            </div>
          {/each}
        </div>
      </fieldset>
      <div class="mt-4 flex flex-wrap items-center gap-2">
        <Button variant="primary" type="submit" disabled={!canSave}>{saving ? 'Saving…' : 'Save DNS configuration'}</Button>
        {#if dirty && !stale}<Button variant="ghost" onclick={loadSaved} disabled={saving}>Discard changes</Button>{/if}
        {#if dirty}<span class="text-sm text-muted">Unsaved changes</span>{/if}
      </div>
    </form>
  {/if}
{:else if loading}
  <div class="mt-5 space-y-3" role="status">
    <span class="sr-only">Loading DNS configuration…</span>
    <div class="h-4 w-48 max-w-full rounded bg-surface" aria-hidden="true"></div>
    <div class="h-36 rounded-md border border-line bg-chrome" aria-hidden="true"></div>
  </div>
{/if}
