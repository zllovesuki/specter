<script lang="ts">
  import { onMount } from 'svelte';
  import Button from '../shared/Button.svelte';
  import { INPUT_CLASSES } from '../shared/styles';
  import SnapshotTime from './SnapshotTime.svelte';
  import { requestWAFConfig, WAFRequestError, type WAFConfig, type WAFSnapshot } from './waf-api';

  type Notice = { tone: 'success' | 'warning' | 'error'; message: string; detail?: string };
  let snapshot = $state<WAFSnapshot | null>(null);
  let draft = $state<WAFConfig>({ enabled: false, default_mode: 'BLOCK', channel: 'LTS', pinned_version: '' });
  let baseline = $state('');
  let expectedVersion = $state('');
  let initialized = $state(false);
  let loading = $state(true);
  let saving = $state(false);
  let conflict = $state(false);
  let readError = $state('');
  let notice = $state<Notice | null>(null);
  let pendingRead: AbortController | undefined;
  let pendingWrite: AbortController | undefined;
  let disposed = false;

  const draftValue = $derived(JSON.stringify(draft));
  const dirty = $derived(initialized && draftValue !== baseline);
  const readOnly = $derived(Boolean(snapshot?.error || (snapshot && !snapshot.config)));
  const stale = $derived(conflict || (initialized && snapshot !== null && snapshot.version !== expectedVersion));
  const canSave = $derived(initialized && dirty && !loading && !saving && !readOnly && !stale);
  const pending = $derived(Boolean(snapshot?.version && snapshot.status.applied_version !== snapshot.version));
  const channelResumes = $derived(Boolean(snapshot?.config?.pinned_version
    && draft.channel !== snapshot.config.channel && !draft.pinned_version));
  const pinChanged = $derived(draft.pinned_version !== (snapshot?.config?.pinned_version ?? ''));

  function loadSaved() {
    if (!snapshot?.config || readOnly) return;
    draft = { ...snapshot.config };
    baseline = JSON.stringify(draft);
    expectedVersion = snapshot.version;
    initialized = true;
    conflict = false;
    notice = null;
  }

  function edited() {
    if (notice?.tone !== 'warning') notice = null;
  }

  function changeChannel() {
    draft.pinned_version = '';
    edited();
  }

  function setPin(version = '') {
    draft.pinned_version = version;
    edited();
  }

  async function refresh() {
    if (saving) return;
    pendingRead?.abort();
    const controller = new AbortController();
    pendingRead = controller;
    const timeout = window.setTimeout(() => controller.abort(), 12_000);
    loading = true;
    try {
      const data = await requestWAFConfig(controller.signal, undefined, snapshot?.version);
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

  async function save(event: SubmitEvent) {
    event.preventDefault();
    if (!canSave) return;
    saving = true;
    notice = null;
    const controller = new AbortController();
    pendingWrite = controller;
    const timeout = window.setTimeout(() => controller.abort(), 15_000);
    let reread = false;
    try {
      const data = await requestWAFConfig(controller.signal, JSON.stringify({ expected_version: expectedVersion, config: draft }));
      if (disposed) return;
      snapshot = data;
      readError = '';
      loadSaved();
      notice = { tone: 'success', message: `Saved on ${data.server.address}.` };
    } catch (cause) {
      if (disposed) return;
      const error = cause instanceof WAFRequestError ? cause : new WAFRequestError('Save result unknown.', 'outcome_unknown');
      if (error.code === 'version_conflict') {
        conflict = true;
        reread = true;
      } else {
        notice = error.code === 'outcome_unknown'
          ? { tone: 'warning', message: 'Save result unknown. Your draft is unchanged.', detail: error.message }
          : { tone: 'error', message: 'Could not save web protection.', detail: error.message };
      }
    } finally {
      window.clearTimeout(timeout);
      pendingWrite = undefined;
      saving = false;
    }
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
  <h1 class="text-[22px] leading-tight font-semibold tracking-tight sm:text-2xl">Web protection</h1>
  <Button onclick={refresh} disabled={loading || saving}>{loading ? 'Refreshing…' : 'Refresh'}</Button>
</div>

{#if snapshot}
  <p class="text-sm text-muted wrap-anywhere">
    Server <code class="text-foreground">{snapshot.server.address}</code> · <SnapshotTime value={snapshot.observed_at} />
  </p>
  <div class="mt-4 border-y border-line py-3 text-sm">
    <p class="flex flex-wrap gap-x-4 gap-y-1">
      <span>Local protection: <strong class="font-semibold">{snapshot.status.enabled ? snapshot.status.ready ? 'Enabled' : 'Not ready' : 'Off'}</strong></span>
      <span>Rules: <code>{snapshot.status.active_version || 'Not loaded'}</code></span>
      <span>Engine: <code>{snapshot.status.engine_version || 'Unavailable'}</code></span>
    </p>
    {#if pending}<p class="mt-1 text-warning">Saved settings are pending on this server.</p>{/if}
    {#if snapshot.status.updating}<p class="mt-1 text-muted">Checking rules…</p>{/if}
    {#if snapshot.status.last_error}
      <details class="mt-1 text-warning">
        <summary class="min-h-8 cursor-pointer py-1">Protection update needs attention</summary>
        <p class="mt-1 max-w-[72ch] wrap-anywhere">{snapshot.status.last_error}</p>
      </details>
    {/if}
  </div>
{/if}

{#if readError}
  <div class="my-4 rounded-md border border-danger/40 bg-surface px-4 py-3" role="alert">
    <p class="font-medium text-danger">{snapshot ? 'Refresh failed. Showing previous state.' : 'Could not load web protection.'}</p>
    <details class="mt-1"><summary class="min-h-8 cursor-pointer py-1 text-muted">Request error</summary><p class="mt-1 max-w-[72ch] text-sm wrap-anywhere">{readError}</p></details>
  </div>
{/if}

{#if snapshot}
  {#if readOnly}
    <div class="mt-4 rounded-md border border-warning/40 bg-surface px-4 py-3" role="alert">
      <p class="font-medium text-warning">Saved settings cannot be edited by this server.</p>
      <p class="mt-1 max-w-[72ch] text-sm wrap-anywhere">{snapshot.error || 'Use a compatible Specter version.'}</p>
    </div>
  {:else if initialized}
    {#if stale}
      <div class="mt-4 rounded-md border border-warning/40 bg-surface px-4 py-3" role="alert">
        <p class="font-medium text-warning">Saved settings changed. Your draft is unchanged.</p>
        {#if snapshot.config}
          <p class="mt-1 text-sm">Latest: {snapshot.config.enabled ? 'Enabled' : 'Off'} · {snapshot.config.default_mode === 'BLOCK' ? 'Block' : 'Monitor'} · {snapshot.config.channel === 'LTS' ? 'LTS' : 'Current stable'}{snapshot.config.pinned_version ? ` · Pinned ${snapshot.config.pinned_version}` : ''}</p>
        {/if}
        <Button class="mt-2" size="sm" onclick={loadSaved} disabled={loading || saving || Boolean(readError)}>Load latest and discard draft</Button>
      </div>
    {/if}
    {#if notice}
      <div class={['mt-4 rounded-md border bg-surface px-4 py-3', notice.tone === 'success' ? 'border-success/40' : notice.tone === 'warning' ? 'border-warning/40' : 'border-danger/40']} role={notice.tone === 'success' ? 'status' : 'alert'}>
        <p class="font-medium wrap-anywhere" class:text-success={notice.tone === 'success'} class:text-warning={notice.tone === 'warning'} class:text-danger={notice.tone === 'error'}>{notice.message}</p>
        {#if notice.detail}<details class="mt-1"><summary class="min-h-8 cursor-pointer py-1 text-muted">Request details</summary><p class="mt-1 max-w-[72ch] text-sm wrap-anywhere">{notice.detail}</p></details>{/if}
      </div>
    {/if}

    <form onsubmit={save} class="mt-5 max-w-2xl" aria-label="Web protection settings">
      <fieldset disabled={saving}>
        <legend class="sr-only">Shared web protection</legend>
        <label class="flex min-h-10 w-fit cursor-pointer items-center gap-3 font-medium">
          <input type="checkbox" class="size-4 accent-accent-500" bind:checked={draft.enabled} onchange={edited} aria-describedby="waf-enabled-help" />
          Enabled
        </label>
        <p id="waf-enabled-help" class="mt-1 text-sm text-muted">Turning this off disables protection for every hostname.</p>
        <div class="mt-4 grid gap-4 sm:grid-cols-2">
          <div>
            <label for="waf-mode" class="mb-1 block text-sm font-medium">Default mode</label>
            <select id="waf-mode" class={INPUT_CLASSES} bind:value={draft.default_mode} onchange={edited} disabled={!draft.enabled} aria-describedby="waf-mode-help">
              <option value="BLOCK">Block</option><option value="MONITOR">Monitor</option>
            </select>
            <p id="waf-mode-help" class="mt-1 text-xs text-muted">Hostname owners can override this default.</p>
          </div>
          <div>
            <label for="waf-channel" class="mb-1 block text-sm font-medium">Rules channel</label>
            <select id="waf-channel" class={INPUT_CLASSES} bind:value={draft.channel} onchange={changeChannel}>
              <option value="LTS">LTS</option><option value="STABLE">Current stable</option>
            </select>
          </div>
        </div>
        {#if channelResumes}<p class="mt-3 text-sm text-warning">Saving this channel resumes automatic updates.</p>{/if}

        <details class="mt-5 border-t border-line pt-2">
          <summary class="min-h-10 cursor-pointer py-2 font-medium">Advanced</summary>
          <p class="mt-1 text-sm text-muted">Updates: {pinChanged ? 'after saving, ' : ''}{draft.pinned_version ? `pinned to ${draft.pinned_version}` : 'automatic'}.</p>
          <div class="mt-3 flex flex-wrap gap-2">
            {#if snapshot.status.previous_version}
              <Button size="sm" class="max-w-full whitespace-normal wrap-anywhere" onclick={() => setPin(snapshot?.status.previous_version)} disabled={draft.pinned_version === snapshot.status.previous_version}>Use previous version ({snapshot.status.previous_version})</Button>
            {/if}
            {#if draft.pinned_version}<Button size="sm" onclick={() => setPin()}>Resume automatic updates</Button>{/if}
          </div>
        </details>
      </fieldset>
      <div class="mt-4 flex flex-wrap items-center gap-2">
        <Button variant="primary" type="submit" disabled={!canSave}>{saving ? 'Saving…' : 'Save settings'}</Button>
        {#if dirty && !stale}<Button variant="ghost" onclick={loadSaved} disabled={saving}>Discard changes</Button>{/if}
        {#if dirty}<span class="text-sm text-muted">Unsaved changes</span>{/if}
      </div>
    </form>
  {/if}
{:else if loading}
  <div class="mt-5 space-y-3" role="status">
    <span class="sr-only">Loading web protection…</span>
    <div class="h-4 w-48 max-w-full rounded bg-surface" aria-hidden="true"></div>
    <div class="h-36 rounded-md border border-line bg-chrome" aria-hidden="true"></div>
  </div>
{/if}
