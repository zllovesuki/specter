<script lang="ts">
  import Button from '../shared/Button.svelte';
  import SaveNotice from '../shared/SaveNotice.svelte';
  import { createEditor } from '../shared/editor.svelte';
  import { INPUT_CLASSES } from '../shared/styles';
  import type { WAFChannel, WAFDefaultMode } from '../shared/waf';
  import SnapshotTime from './SnapshotTime.svelte';
  import { requestWAFConfig, type WAFConfig } from './waf-api';

  const editor = createEditor({
    key: () => '/_internal/api/config/waf',
    defaults: { enabled: false, default_mode: 'BLOCK', channel: 'LTS', pinned_version: '' } as WAFConfig,
    load: (signal, after) => requestWAFConfig(signal, undefined, after),
    save: (value, version, signal) => requestWAFConfig(signal, JSON.stringify({ expected_version: version, config: value })),
    draft: snapshot => ({ ...snapshot.config! }),
    editable: snapshot => !snapshot.error && snapshot.config !== null,
  });
  const snapshot = $derived(editor.query.data);
  const draft = $derived(editor.values);
  const readError = $derived(editor.query.error?.message ?? '');
  const pending = $derived(Boolean(snapshot?.version && snapshot.status.applied_version !== snapshot.version));
  const channelResumes = $derived(Boolean(snapshot?.config?.pinned_version && draft.channel !== snapshot.config.channel && !draft.pinned_version));
  const pinChanged = $derived(draft.pinned_version !== (snapshot?.config?.pinned_version ?? ''));

  function setPin(version = '') {
    editor.form.setFieldValue('pinned_version', version);
    editor.edited();
  }
</script>

<div class="mb-2 flex flex-wrap items-center justify-between gap-3">
  <h1 class="text-[22px] leading-tight font-semibold tracking-tight sm:text-2xl">Web protection</h1>
  <Button onclick={() => editor.refresh()} disabled={editor.query.isFetching || editor.saving}>{editor.query.isFetching ? 'Refreshing…' : 'Refresh'}</Button>
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
  {#if editor.readOnly}
    <div class="mt-4 rounded-md border border-warning/40 bg-surface px-4 py-3" role="alert">
      <p class="font-medium text-warning">Saved settings cannot be edited by this server.</p>
      <p class="mt-1 max-w-[72ch] text-sm wrap-anywhere">{snapshot.error || 'Use a compatible Specter version.'}</p>
    </div>
  {:else if editor.initialized}
    {#if editor.stale}
      <div class="mt-4 rounded-md border border-warning/40 bg-surface px-4 py-3" role="alert">
        <p class="font-medium text-warning">Saved settings changed. Your draft is unchanged.</p>
        {#if snapshot.config}
          <p class="mt-1 text-sm">Latest: {snapshot.config.enabled ? 'Enabled' : 'Off'} · {snapshot.config.default_mode === 'BLOCK' ? 'Block' : 'Monitor'} · {snapshot.config.channel === 'LTS' ? 'LTS' : 'Current stable'}{snapshot.config.pinned_version ? ` · Pinned ${snapshot.config.pinned_version}` : ''}</p>
        {/if}
        <Button class="mt-2" size="sm" onclick={() => editor.accept()} disabled={editor.query.isFetching || editor.saving || Boolean(readError)}>Load latest and discard draft</Button>
      </div>
    {/if}
    <SaveNotice outcome={editor.outcome} saved={editor.outcome?.kind === 'saved' ? `Saved on ${editor.outcome.snapshot.server.address}.` : ''} failed="Could not save web protection." />

    <form onsubmit={editor.submit} class="mt-5 max-w-2xl" aria-label="Web protection settings">
      <fieldset disabled={editor.saving}>
        <legend class="sr-only">Shared web protection</legend>
        <label class="flex min-h-10 w-fit cursor-pointer items-center gap-3 font-medium">
          <input type="checkbox" class="size-4 accent-accent-500" checked={draft.enabled} onchange={(event) => { editor.form.setFieldValue('enabled', event.currentTarget.checked); editor.edited(); }} aria-describedby="waf-enabled-help" />
          Enabled
        </label>
        <p id="waf-enabled-help" class="mt-1 text-sm text-muted">Turning this off disables protection for every hostname.</p>
        <div class="mt-4 grid gap-4 sm:grid-cols-2">
          <div>
            <label for="waf-mode" class="mb-1 block text-sm font-medium">Default mode</label>
            <select id="waf-mode" class={INPUT_CLASSES} value={draft.default_mode} onchange={(event) => { editor.form.setFieldValue('default_mode', event.currentTarget.value as WAFDefaultMode); editor.edited(); }} disabled={!draft.enabled} aria-describedby="waf-mode-help">
              <option value="BLOCK">Block</option><option value="MONITOR">Monitor</option>
            </select>
            <p id="waf-mode-help" class="mt-1 text-xs text-muted">Hostname owners can override this default.</p>
          </div>
          <div>
            <label for="waf-channel" class="mb-1 block text-sm font-medium">Rules channel</label>
            <select id="waf-channel" class={INPUT_CLASSES} value={draft.channel} onchange={(event) => { editor.form.setFieldValue('channel', event.currentTarget.value as WAFChannel); setPin(); }}>
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
        <Button variant="primary" type="submit" disabled={!editor.canSave}>{editor.saving ? 'Saving…' : 'Save settings'}</Button>
        {#if editor.dirty && !editor.stale}<Button variant="ghost" onclick={() => editor.accept()} disabled={editor.saving}>Discard changes</Button>{/if}
        {#if editor.dirty}<span class="text-sm text-muted">Unsaved changes</span>{/if}
      </div>
    </form>
  {/if}
{:else if editor.query.isFetching}
  <div class="mt-5 space-y-3" role="status">
    <span class="sr-only">Loading web protection…</span>
    <div class="h-4 w-48 max-w-full rounded bg-surface" aria-hidden="true"></div>
    <div class="h-36 rounded-md border border-line bg-chrome" aria-hidden="true"></div>
  </div>
{/if}
