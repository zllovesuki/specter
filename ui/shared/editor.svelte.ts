import { onDestroy, untrack } from 'svelte';
import { createQuery } from '@tanstack/svelte-query';
import { createForm } from '@tanstack/svelte-form';
import { RequestError, errorMessage } from './http';
import { queryClient } from './query';

export type SaveOutcome<S> = { kind: 'saved'; snapshot: S }
  | { kind: 'conflict' | 'unknown' | 'rejected'; error: RequestError };

// Query owns the observed snapshot, Form owns the draft, and the version belongs
// to the snapshot explicitly accepted as that draft's base.
export function createEditor<S extends { version: string }, D>(options: {
  key: () => string;
  defaults: D;
  load: (signal: AbortSignal, after?: string) => Promise<S>;
  save: (value: D, version: string, signal: AbortSignal) => Promise<S>;
  draft: (snapshot: S) => D;
  editable?: (snapshot: S) => boolean;
}) {
  let base = $state<{ key: string; version: string } | null>(null);
  let outcome = $state<SaveOutcome<S> | null>(null);
  const lifetime = new AbortController();
  const key = $derived(options.key());
  const query = createQuery(() => ({
    queryKey: [key], enabled: key !== '', staleTime: 0,
    queryFn: ({ signal, queryKey }) => options.load(signal, queryClient.getQueryData<S>(queryKey)?.version),
    refetchOnMount: 'always',
  }));
  const form = createForm(() => ({
    defaultValues: options.defaults,
    onSubmit: async ({ value }) => {
      if (!base || base.key !== key || stale || readOnly || lifetime.signal.aborted) return;
      const submitted = base;
      outcome = null;
      await queryClient.cancelQueries({ queryKey: [submitted.key] });
      try {
        const saved = await options.save(value, submitted.version, lifetime.signal);
        if (lifetime.signal.aborted || submitted.key !== key) return;
        queryClient.setQueryData([submitted.key], saved);
        accept(saved);
        outcome = { kind: 'saved', snapshot: saved };
      } catch (cause) {
        if (lifetime.signal.aborted || submitted.key !== key) return;
        const error = cause instanceof RequestError ? cause : new RequestError(errorMessage(cause), 'outcome_unknown');
        outcome = { kind: error.status === 409 || error.code === 'version_conflict' ? 'conflict'
          : error.code === 'outcome_unknown' ? 'unknown' : 'rejected', error };
        if (outcome.kind === 'conflict') await query.refetch();
        throw error;
      }
    },
  }));
  const state = form.useSelector(s => ({ values: s.values, dirty: !s.isDefaultValue, saving: s.isSubmitting }));
  const readOnly = $derived(Boolean(query.data && options.editable && !options.editable(query.data)));
  const stale = $derived(base?.key === key && (outcome?.kind === 'conflict' || (query.data !== undefined && base.version !== query.data.version)));
  const dirty = $derived(base?.key === key && state.current.dirty);
  const canSave = $derived(dirty && !state.current.saving && !query.isFetching && !readOnly && !stale);

  function accept(snapshot = query.data) {
    if (!snapshot || (options.editable && !options.editable(snapshot))) return;
    form.reset(options.draft(snapshot));
    base = { key, version: snapshot.version };
    outcome = null;
  }

  $effect(() => {
    const currentKey = key;
    const snapshot = query.data;
    untrack(() => {
      if (base?.key !== currentKey) {
        base = null;
        outcome = null;
        form.reset(options.defaults);
      }
      if (snapshot && (!base || (!dirty && outcome?.kind !== 'conflict' && outcome?.kind !== 'unknown' && base.version !== snapshot.version))) accept(snapshot);
    });
  });
  onDestroy(() => lifetime.abort());

  return {
    form, query, accept,
    get values() { return state.current.values; },
    get initialized() { return base?.key === key; },
    get outcome() { return outcome; },
    get dirty() { return dirty; },
    get stale() { return stale; },
    get readOnly() { return readOnly; },
    get saving() { return state.current.saving; },
    get canSave() { return canSave; },
    get fields() { return outcome?.kind === 'rejected' ? outcome.error.fields : {}; },
    edited() { if (outcome?.kind === 'saved' || outcome?.kind === 'rejected') outcome = null; },
    async refresh() { if (!form.state.isSubmitting) await query.refetch(); },
    async submit(event: SubmitEvent) {
      event.preventDefault();
      if (canSave && !form.state.isSubmitting) await form.handleSubmit().catch(() => {});
    },
  };
}
