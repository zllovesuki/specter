<script lang="ts">
  import { tick } from 'svelte';
  import { createQuery } from '@tanstack/svelte-query';
  import Button from '../shared/Button.svelte';
  import SaveNotice from '../shared/SaveNotice.svelte';
  import { createEditor } from '../shared/editor.svelte';
  import { INPUT_CLASSES } from '../shared/styles';
  import type { WAFMode } from '../shared/waf';
  import { hostnameQueryOptions } from './api';
  import { requestHostWAF } from './waf-api';

  type Draft = { mode: WAFMode; exclusions: { rule: string; path: string }[] };
  let hostname = $state('');
  let form = $state<HTMLFormElement>();
  const hostnamesQuery = createQuery(() => hostnameQueryOptions);
  const hostnames = $derived(hostnamesQuery.data?.map(item => item.hostname) ?? null);
  const loadingHostnames = $derived(hostnamesQuery.isFetching);
  const hostnameError = $derived(hostnamesQuery.error?.message ?? '');
  const choices = $derived([...(hostnames ?? [])].sort((a, b) => a.localeCompare(b)));
  const registered = $derived(choices.includes(hostname));
  const editor = createEditor({
    key: () => hostname ? `/api/waf?hostname=${encodeURIComponent(hostname)}` : '',
    defaults: { mode: 'INHERIT', exclusions: [] } as Draft,
    load: signal => requestHostWAF(hostname, signal),
    save: (value, version, signal) => requestHostWAF(hostname, signal, JSON.stringify({
      expected_version: version,
      policy: { mode: value.mode, exclusions: value.exclusions.map(({ rule, path }) => ({ rule_id: Number(rule), path })) },
    })),
    draft: snapshot => ({ mode: snapshot.policy.mode, exclusions: snapshot.policy.exclusions.map(item => ({ rule: String(item.rule_id), path: item.path })) }),
  });
  const snapshot = $derived(editor.query.data);
  const draft = $derived(editor.values);
  const readError = $derived(editor.query.error?.message ?? '');
  const placeholder = $derived(hostnames === null
    ? hostnameError && !loadingHostnames ? 'Hostnames unavailable' : 'Loading hostnames…'
    : choices.length ? 'Select hostname…' : 'No registered hostnames');

  async function refresh() {
    if (!editor.saving) await Promise.allSettled([hostname ? editor.refresh() : Promise.resolve(), hostnamesQuery.refetch()]);
  }

  async function addExclusion() {
    const index = draft.exclusions.length;
    editor.form.pushFieldValue('exclusions', { rule: '', path: '' });
    editor.edited();
    await tick();
    form?.querySelector<HTMLInputElement>(`#waf-rule-${index}`)?.focus();
  }
</script>

<section class="mt-6 min-w-0 border-t border-line pt-5" aria-labelledby="protection-heading">
  <div class="mb-3.5 flex flex-wrap items-center justify-between gap-3">
    <h2 id="protection-heading" class="text-lg leading-snug font-semibold tracking-tight">Web protection</h2>
    <Button size="sm" onclick={refresh} disabled={editor.query.isFetching || editor.saving || loadingHostnames}>{editor.query.isFetching || loadingHostnames ? 'Refreshing…' : 'Refresh'}</Button>
  </div>
  <div class="max-w-lg">
    <label for="protected-hostname" class="mb-1.5 block text-sm font-medium">Hostname</label>
    <select id="protected-hostname" class={INPUT_CLASSES} bind:value={hostname} disabled={editor.query.isFetching || editor.saving || editor.dirty || !choices.length}>
      <option value="" disabled>{placeholder}</option>
      {#if hostname && !registered}<option value={hostname}>{hostname} (no longer registered)</option>{/if}
      {#each choices as host (host)}<option value={host}>{host}</option>{/each}
    </select>
    {#if editor.dirty}<p class="mt-1 text-xs text-muted">Save or discard changes before choosing another hostname.</p>{/if}
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
  {#if editor.stale}
    <div class="mt-3 rounded-md border border-warning/40 bg-chrome px-3.5 py-3" role="alert">
      <p class="font-medium text-warning">Saved policy changed. Your draft is unchanged.</p>
      <Button class="mt-2" size="sm" onclick={() => editor.accept()} disabled={editor.query.isFetching || editor.saving || Boolean(readError)}>Load latest and discard draft</Button>
    </div>
  {/if}
  <SaveNotice outcome={editor.outcome} saved="Hostname policy saved." failed="Could not save hostname protection." class="mt-3 bg-chrome px-3.5 py-3" />

  {#if editor.initialized}
    <form bind:this={form} onsubmit={(event) => { if (registered) return editor.submit(event); event.preventDefault(); }} class="mt-4" aria-label={`Protection for ${hostname}`}>
      <fieldset disabled={editor.saving || !registered}>
        <legend class="sr-only">Hostname policy</legend>
        <div class="max-w-lg">
          <label for="hostname-waf-mode" class="mb-1.5 block text-sm font-medium">Mode</label>
          <select id="hostname-waf-mode" class={INPUT_CLASSES} value={draft.mode} onchange={(event) => { editor.form.setFieldValue('mode', event.currentTarget.value as WAFMode); editor.edited(); }}>
            <option value="INHERIT">Server default</option><option value="BLOCK">Block</option><option value="MONITOR">Monitor</option><option value="OFF">Off</option>
          </select>
        </div>
        <div class="mt-4 mb-2 flex flex-wrap items-center justify-between gap-2">
          <h3 class="font-semibold">Rule exceptions</h3>
          <Button size="sm" onclick={addExclusion}>Add exception</Button>
        </div>
        {#if draft.exclusions.length}
          <div class="border-t border-line">
            {#each draft.exclusions as item, index}
              <div class="grid items-start gap-3 border-b border-line py-3 sm:grid-cols-[9rem_minmax(0,1fr)_auto]">
                <div>
                  <label for={`waf-rule-${index}`} class="mb-1 block text-sm font-medium">Rule ID</label>
                  <input id={`waf-rule-${index}`} class={INPUT_CLASSES + ' font-mono'} value={item.rule} oninput={(event) => { editor.form.setFieldValue(`exclusions[${index}].rule`, event.currentTarget.value); editor.edited(); }} inputmode="numeric" pattern={'[0-9]{6}'} maxlength="6" required placeholder="941100" title="Enter the six-digit rule ID from the blocked request." aria-label={`Rule ID for exception ${index + 1}`} />
                </div>
                <div class="min-w-0">
                  <label for={`waf-path-${index}`} class="mb-1 block text-sm font-medium">Exact path <span class="font-normal text-muted">(optional)</span></label>
                  <input id={`waf-path-${index}`} class={INPUT_CLASSES + ' font-mono'} value={item.path} oninput={(event) => { editor.form.setFieldValue(`exclusions[${index}].path`, event.currentTarget.value); editor.edited(); }} autocomplete="off" autocapitalize="none" spellcheck={false} placeholder="/api/save" aria-describedby="waf-path-help" />
                </div>
                <Button size="sm" variant="ghost" class="justify-self-start sm:mt-6" onclick={() => { editor.form.removeFieldValue('exclusions', index); editor.edited(); }} aria-label={`Remove exception ${item.rule || index + 1} and enable the rule again`}>{item.rule ? 'Enable again' : 'Remove'}</Button>
              </div>
            {/each}
          </div>
          <p id="waf-path-help" class="mt-2 text-xs text-muted">Use the escaped path without a query string. An empty path applies to every path on this hostname.</p>
        {:else}
          <p class="text-sm text-muted">No rule exceptions.</p>
        {/if}
      </fieldset>
      <div class="mt-4 flex flex-wrap items-center gap-2">
        <Button variant="primary" type="submit" disabled={!editor.canSave || !registered}>{editor.saving ? 'Saving…' : 'Save policy'}</Button>
        {#if editor.dirty && !editor.stale}<Button variant="ghost" onclick={() => editor.accept()} disabled={editor.saving}>Discard changes</Button>{/if}
        {#if editor.dirty}<span class="text-sm text-muted">Unsaved changes</span>{/if}
      </div>
    </form>
  {:else if editor.query.isFetching}
    <div class="mt-4 h-24 rounded-md border border-line bg-chrome" role="status"><span class="sr-only">Loading hostname protection…</span></div>
  {/if}
</section>
