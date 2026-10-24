<script lang="ts">
  import { onDestroy, tick } from 'svelte';
  import Button from '../shared/Button.svelte';
  import { INPUT_CLASSES } from '../shared/styles';
  import type { WAFMode } from '../shared/waf';
  import { HostWAFError, requestHostWAF, type HostWAFSnapshot } from './waf-api';

  let { hostnames, loadingHostnames, hostnameError, onRefreshHostnames }: {
    hostnames: string[] | null;
    loadingHostnames: boolean;
    hostnameError: string;
    onRefreshHostnames: () => Promise<void>;
  } = $props();
  type ExclusionDraft = { key: number; rule: string; path: string };
  type Notice = { tone: 'success' | 'warning' | 'error'; message: string; detail?: string };
  let hostname = $state('');
  let snapshot = $state<HostWAFSnapshot | null>(null);
  let mode = $state<WAFMode>('INHERIT');
  let exclusions = $state<ExclusionDraft[]>([]);
  let baseline = $state('');
  let expectedVersion = $state('');
  let initialized = $state(false);
  let loading = $state(false);
  let saving = $state(false);
  let conflict = $state(false);
  let readError = $state('');
  let notice = $state<Notice | null>(null);
  let form = $state<HTMLFormElement>();
  let pendingRead: AbortController | undefined;
  let pendingWrite: AbortController | undefined;
  let nextKey = 0;
  let disposed = false;

  const choices = $derived([...(hostnames ?? [])].sort((a, b) => a.localeCompare(b)));
  const registered = $derived(choices.includes(hostname));
  const draftValue = $derived(JSON.stringify({ mode, exclusions: exclusions.map(({ rule, path }) => ({ rule, path })) }));
  const dirty = $derived(initialized && draftValue !== baseline);
  const stale = $derived(conflict || (initialized && snapshot !== null && snapshot.version !== expectedVersion));
  const canSave = $derived(registered && initialized && dirty && !loading && !saving && !stale);
  const placeholder = $derived(hostnames === null
    ? hostnameError && !loadingHostnames ? 'Hostnames unavailable' : 'Loading hostnames…'
    : choices.length ? 'Select hostname…' : 'No registered hostnames');

  function loadSaved() {
    if (!snapshot) return;
    mode = snapshot.policy.mode;
    exclusions = snapshot.policy.exclusions.map(item => ({ key: nextKey++, rule: String(item.rule_id), path: item.path }));
    baseline = JSON.stringify({ mode, exclusions: exclusions.map(({ rule, path }) => ({ rule, path })) });
    expectedVersion = snapshot.version;
    initialized = true;
    conflict = false;
    notice = null;
  }

  function edited() {
    if (notice?.tone !== 'warning') notice = null;
  }

  async function addExclusion() {
    const key = nextKey++;
    exclusions.push({ key, rule: '', path: '' });
    edited();
    await tick();
    form?.querySelector<HTMLInputElement>(`#waf-rule-${key}`)?.focus();
  }

  function removeExclusion(key: number) {
    exclusions = exclusions.filter(item => item.key !== key);
    edited();
  }

  async function loadPolicy() {
    if (!hostname || saving) return;
    pendingRead?.abort();
    const controller = new AbortController();
    pendingRead = controller;
    const target = hostname;
    const timeout = window.setTimeout(() => controller.abort(), 12_000);
    loading = true;
    try {
      const data = await requestHostWAF(target, controller.signal);
      if (disposed || pendingRead !== controller || hostname !== target) return;
      const replaceDraft = !initialized || (!dirty && !conflict && data.version !== expectedVersion);
      snapshot = data;
      readError = '';
      if (replaceDraft) loadSaved();
    } catch (cause) {
      if (disposed || pendingRead !== controller || hostname !== target) return;
      readError = cause instanceof Error ? cause.message : 'Request failed.';
    } finally {
      window.clearTimeout(timeout);
      if (pendingRead === controller) {
        pendingRead = undefined;
        loading = false;
      }
    }
  }

  async function selectHostname(event: Event) {
    hostname = (event.currentTarget as HTMLSelectElement).value;
    snapshot = null;
    initialized = false;
    conflict = false;
    notice = null;
    readError = '';
    await loadPolicy();
  }

  async function refresh() {
    if (saving || loading) return;
    await Promise.allSettled([loadPolicy(), onRefreshHostnames()]);
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
      const policy = { mode, exclusions: exclusions.map(({ rule, path }) => ({ rule_id: Number(rule), path })) };
      const data = await requestHostWAF(hostname, controller.signal, JSON.stringify({ expected_version: expectedVersion, policy }));
      if (disposed) return;
      snapshot = data;
      readError = '';
      loadSaved();
      notice = { tone: 'success', message: 'Hostname policy saved.' };
    } catch (cause) {
      if (disposed) return;
      const error = cause instanceof HostWAFError ? cause : new HostWAFError('Save result unknown.', 'outcome_unknown');
      if (error.code === 'version_conflict') {
        conflict = true;
        reread = true;
      } else {
        notice = error.code === 'outcome_unknown'
          ? { tone: 'warning', message: 'Save result unknown. Your draft is unchanged.', detail: error.message }
          : { tone: 'error', message: 'Could not save hostname protection.', detail: error.message };
      }
    } finally {
      window.clearTimeout(timeout);
      pendingWrite = undefined;
      saving = false;
    }
    if (reread) await loadPolicy();
  }

  onDestroy(() => {
    disposed = true;
    pendingRead?.abort();
    pendingWrite?.abort();
  });
</script>

<section class="mt-6 min-w-0 border-t border-line pt-5" aria-labelledby="protection-heading">
  <div class="mb-3.5 flex flex-wrap items-center justify-between gap-3">
    <h2 id="protection-heading" class="text-lg leading-snug font-semibold tracking-tight">Web protection</h2>
    <Button size="sm" onclick={refresh} disabled={loading || saving || loadingHostnames}>{loading || loadingHostnames ? 'Refreshing…' : 'Refresh'}</Button>
  </div>
  <div class="max-w-lg">
    <label for="protected-hostname" class="mb-1.5 block text-sm font-medium">Hostname</label>
    <select id="protected-hostname" class={INPUT_CLASSES} value={hostname} onchange={selectHostname} disabled={loading || saving || dirty || !choices.length}>
      <option value="" disabled>{placeholder}</option>
      {#if hostname && !registered}<option value={hostname}>{hostname} (no longer registered)</option>{/if}
      {#each choices as host (host)}<option value={host}>{host}</option>{/each}
    </select>
    {#if dirty}<p class="mt-1 text-xs text-muted">Save or discard changes before choosing another hostname.</p>{/if}
  </div>
  {#if hostnameError}<p class="mt-2 text-sm text-warning">{hostnames === null ? 'Could not load hostnames. Refresh to retry.' : 'Using the previous hostname list.'}</p>
  {:else if hostnames !== null && !choices.length}<p class="mt-2 text-sm text-muted"><a href="#configuration" class="text-accent-300 underline underline-offset-4 hover:text-accent-200">Add a tunnel</a> to register a hostname.</p>{/if}
  {#if hostname && !registered && hostnames !== null}<p class="mt-2 text-sm text-warning">This hostname is no longer registered to this client.</p>{/if}

  {#if readError}
    <div class="mt-3 rounded-md border border-danger/40 bg-chrome px-3.5 py-3" role="alert">
      <p class="font-medium text-danger">{snapshot ? 'Refresh failed. Showing previous policy.' : 'Could not load hostname protection.'}</p>
      <details class="mt-1"><summary class="min-h-8 cursor-pointer py-1 text-muted">Request error</summary><p class="mt-1 max-w-[72ch] text-sm wrap-anywhere">{readError}</p></details>
    </div>
  {/if}
  {#if stale}
    <div class="mt-3 rounded-md border border-warning/40 bg-chrome px-3.5 py-3" role="alert">
      <p class="font-medium text-warning">Saved policy changed. Your draft is unchanged.</p>
      <Button class="mt-2" size="sm" onclick={loadSaved} disabled={loading || saving || Boolean(readError)}>Load latest and discard draft</Button>
    </div>
  {/if}
  {#if notice}
    <div class={['mt-3 rounded-md border bg-chrome px-3.5 py-3', notice.tone === 'success' ? 'border-success/40' : notice.tone === 'warning' ? 'border-warning/40' : 'border-danger/40']} role={notice.tone === 'success' ? 'status' : 'alert'}>
      <p class="font-medium wrap-anywhere" class:text-success={notice.tone === 'success'} class:text-warning={notice.tone === 'warning'} class:text-danger={notice.tone === 'error'}>{notice.message}</p>
      {#if notice.detail}<details class="mt-1"><summary class="min-h-8 cursor-pointer py-1 text-muted">Request details</summary><p class="mt-1 max-w-[72ch] text-sm wrap-anywhere">{notice.detail}</p></details>{/if}
    </div>
  {/if}

  {#if initialized}
    <form bind:this={form} onsubmit={save} class="mt-4" aria-label={`Protection for ${hostname}`}>
      <fieldset disabled={saving || !registered}>
        <legend class="sr-only">Hostname policy</legend>
        <div class="max-w-lg">
          <label for="hostname-waf-mode" class="mb-1.5 block text-sm font-medium">Mode</label>
          <select id="hostname-waf-mode" class={INPUT_CLASSES} bind:value={mode} onchange={edited}>
            <option value="INHERIT">Server default</option><option value="BLOCK">Block</option><option value="MONITOR">Monitor</option><option value="OFF">Off</option>
          </select>
        </div>
        <div class="mt-4 mb-2 flex flex-wrap items-center justify-between gap-2">
          <h3 class="font-semibold">Rule exceptions</h3>
          <Button size="sm" onclick={addExclusion}>Add exception</Button>
        </div>
        {#if exclusions.length}
          <div class="border-t border-line">
            {#each exclusions as item, index (item.key)}
              <div class="grid items-start gap-3 border-b border-line py-3 sm:grid-cols-[9rem_minmax(0,1fr)_auto]">
                <div>
                  <label for={`waf-rule-${item.key}`} class="mb-1 block text-sm font-medium">Rule ID</label>
                  <input id={`waf-rule-${item.key}`} class={INPUT_CLASSES + ' font-mono'} bind:value={item.rule} oninput={edited} inputmode="numeric" pattern={'[0-9]{6}'} maxlength="6" required placeholder="941100" title="Enter the six-digit rule ID from the blocked request." aria-label={`Rule ID for exception ${index + 1}`} />
                </div>
                <div class="min-w-0">
                  <label for={`waf-path-${item.key}`} class="mb-1 block text-sm font-medium">Exact path <span class="font-normal text-muted">(optional)</span></label>
                  <input id={`waf-path-${item.key}`} class={INPUT_CLASSES + ' font-mono'} bind:value={item.path} oninput={edited} autocomplete="off" autocapitalize="none" spellcheck={false} placeholder="/api/save" aria-describedby="waf-path-help" />
                </div>
                <Button size="sm" variant="ghost" class="justify-self-start sm:mt-6" onclick={() => removeExclusion(item.key)} aria-label={`Remove exception ${item.rule || index + 1} and enable the rule again`}>{item.rule ? 'Enable again' : 'Remove'}</Button>
              </div>
            {/each}
          </div>
          <p id="waf-path-help" class="mt-2 text-xs text-muted">Use the escaped path without a query string. An empty path applies to every path on this hostname.</p>
        {:else}
          <p class="text-sm text-muted">No rule exceptions.</p>
        {/if}
      </fieldset>
      <div class="mt-4 flex flex-wrap items-center gap-2">
        <Button variant="primary" type="submit" disabled={!canSave}>{saving ? 'Saving…' : 'Save policy'}</Button>
        {#if dirty && !stale}<Button variant="ghost" onclick={loadSaved} disabled={saving}>Discard changes</Button>{/if}
        {#if dirty}<span class="text-sm text-muted">Unsaved changes</span>{/if}
      </div>
    </form>
  {:else if loading}
    <div class="mt-4 h-24 rounded-md border border-line bg-chrome" role="status"><span class="sr-only">Loading hostname protection…</span></div>
  {/if}
</section>
