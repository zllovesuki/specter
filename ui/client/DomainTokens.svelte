<script lang="ts">
  import { onDestroy, tick } from "svelte";
  import { createQuery, queryOptions } from "@tanstack/svelte-query";
  import Button from "../shared/Button.svelte";
  import ScrollRegion from "../shared/ScrollRegion.svelte";
  import { INPUT_CLASSES } from "../shared/styles";
  import { queryClient } from "../shared/query";
  import { errorMessage, hostnameQueryOptions, requestAPI, type DomainToken, type MintedToken, type RevokeOutcome } from "./api";

  const tokenOptions = queryOptions({
    queryKey: ['client', 'tokens'],
    queryFn: ({ signal }) => requestAPI<DomainToken[]>('/api/tokens', { signal }),
  });
  const tokens = createQuery(() => tokenOptions);
  const hostnames = createQuery(() => hostnameQueryOptions);
  const writes = new AbortController();
  let hostname = $state("");
  let hostnameNotice = $state("");
  let expiresAt = $state("");
  let minted = $state<MintedToken | null>(null);
  let pending = $state<{ kind: 'refresh' | 'mint' } | { kind: 'revoke'; id: string } | null>(null);
  let actionError = $state("");
  let notice = $state("");
  let copyNotice = $state("");
  let confirmation = $state<{ grant: DomainToken; trigger: HTMLButtonElement } | null>(null);
  let cancelButton = $state<HTMLButtonElement>();
  let tokenInput = $state<HTMLInputElement>();
  let heading = $state<HTMLHeadingElement>();
  const grants = $derived(tokens.data);
  const choices = $derived((hostnames.data ?? []).map((item) => item.hostname).filter(Boolean).sort((a, b) => a.localeCompare(b)));
  const hostnamePlaceholder = $derived(!hostnames.data
    ? hostnames.isError && !hostnames.isFetching ? "Hostnames unavailable" : "Loading hostnames…"
    : choices.length ? "Select hostname…" : "No registered hostnames");
  const showHostnameHelp = $derived(Boolean(hostnames.isError || hostnameNotice || (hostnames.data && !choices.length)));
  const refreshing = $derived(pending?.kind === "refresh" || tokens.isFetching);
  const busy = $derived(refreshing || Boolean(pending));
  const canMint = $derived(!busy && choices.includes(hostname));
  const error = $derived(actionError || (tokens.error ? `${grants ? "Showing previous token list. " : ""}${tokens.error.message}` : ""));

  $effect(() => {
    if (hostname && hostnames.data && !choices.includes(hostname)) {
      hostname = "";
      hostnameNotice = "Selected hostname is no longer registered.";
    }
  });

  async function refresh() {
    if (busy) return;
    pending = { kind: "refresh" };
    actionError = "";
    await Promise.allSettled([tokens.refetch(), hostnames.refetch()]);
    pending = null;
    if (confirmation && grants && !grants.some((grant) => grant.id === confirmation?.grant.id)) {
      confirmation = null;
      await tick();
      heading?.focus();
    }
  }

  async function mint() {
    if (!canMint) return;
    pending = { kind: "mint" };
    actionError = "";
    notice = "";
    try {
      const expiry = expiresAt ? new Date(expiresAt).toISOString() : undefined;
      const result = await requestAPI<MintedToken>("/api/tokens", {
        method: "POST",
        signal: writes.signal,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ hostname: hostname.trim(), expiresAt: expiry }),
      });
      minted = result;
      queryClient.setQueryData(tokenOptions.queryKey, (current) => current && [...current, result.grant]);
      copyNotice = "";
      await tick();
      selectToken();
    } catch (cause) {
      actionError = errorMessage(cause);
    } finally {
      pending = null;
    }
  }

  function selectToken() {
    tokenInput?.focus();
    tokenInput?.select();
  }

  async function copy() {
    if (!minted) return;
    try {
      await navigator.clipboard.writeText(minted.token);
      copyNotice = "Token copied.";
    } catch {
      selectToken();
      copyNotice = "Token selected. Copy it using your browser or keyboard.";
    }
  }

  async function askRevoke(grant: DomainToken, trigger: HTMLButtonElement) {
    if (busy) return;
    confirmation = { grant, trigger };
    await tick();
    cancelButton?.focus();
  }

  function cancelRevoke() {
    if (pending) return;
    const trigger = confirmation?.trigger;
    confirmation = null;
    trigger?.focus();
  }

  async function revoke() {
    if (!confirmation || busy) return;
    const grant = confirmation.grant;
    pending = { kind: "revoke", id: grant.id };
    actionError = "";
    notice = "";
    try {
      const result = await requestAPI<RevokeOutcome>(`/api/tokens/${encodeURIComponent(grant.id)}/revoke`, { method: "POST", signal: writes.signal });
      if (result.revoked) {
        if (minted?.grant.id === grant.id) minted = null;
        if (result.indexError) {
          actionError = `Access revoked; list cleanup failed. Revoke again to finish: ${result.indexError}`;
          queryClient.setQueryData(tokenOptions.queryKey, (current) => current?.map((item) => item.id === grant.id ? { ...item, incomplete: true } : item));
        } else {
          queryClient.setQueryData(tokenOptions.queryKey, (current) => current?.filter((item) => item.id !== grant.id));
          notice = "Token revoked.";
        }
        confirmation = null;
        await tick();
        heading?.focus();
      }
    } catch (cause) {
      actionError = errorMessage(cause);
    } finally {
      pending = null;
    }
  }

  onDestroy(() => writes.abort());
</script>

<svelte:window onkeydown={(event) => { if (event.key === "Escape" && confirmation) cancelRevoke(); }} />

<section class="mt-6 min-w-0 border-t border-line pt-5" aria-labelledby="tokens-heading">
  <div class="mb-3.5 flex items-center justify-between gap-3">
    <h2 id="tokens-heading" bind:this={heading} tabindex="-1" class="text-lg leading-snug font-semibold tracking-tight">Domain tokens</h2>
    <Button size="sm" disabled={busy} onclick={refresh}>{refreshing ? "Refreshing…" : "Refresh"}</Button>
  </div>
  <form class={['flex flex-wrap items-end gap-3', showHostnameHelp ? 'mb-1.5' : 'mb-3.5']} onsubmit={(event) => { event.preventDefault(); void mint(); }}>
    <div class="min-w-0 grow basis-64">
      <label for="token-hostname" class="mb-1.5 block font-medium">Hostname</label>
      <select id="token-hostname" class={INPUT_CLASSES} bind:value={hostname} onchange={() => { hostnameNotice = ""; }} disabled={Boolean(pending) || !choices.length} aria-describedby="token-hostname-help" aria-busy={hostnames.isFetching} required>
        <option value="" disabled>{hostnamePlaceholder}</option>
        {#each choices as host (host)}<option value={host}>{host}</option>{/each}
      </select>
    </div>
    <div class="min-w-0 grow basis-56">
      <label for="token-expiry" class="mb-1.5 block font-medium">Expiry <span class="font-normal text-muted">(optional)</span></label>
      <input id="token-expiry" type="datetime-local" class={INPUT_CLASSES} bind:value={expiresAt} disabled={Boolean(pending)} />
    </div>
    <Button type="submit" variant="primary" disabled={!canMint}>{pending?.kind === "mint" ? "Minting…" : "Mint token"}</Button>
  </form>
  <p id="token-hostname-help" class={['text-[0.8125rem] text-muted', showHostnameHelp && 'mb-3.5']} aria-live="polite">
    {#if hostnames.isError}
      {!hostnames.data ? "Refresh to retry." : "Using previous hostname list."}
    {:else if hostnameNotice}
      {hostnameNotice}
    {:else if hostnames.data && !choices.length}
      <a href="#configuration" class="text-accent-300 underline underline-offset-4 hover:text-accent-200">Add a tunnel</a> to register a hostname.
    {/if}
  </p>

  {#if minted}
    <div class="my-3.5">
      <label for="minted-token" class="mb-1.5 block font-medium wrap-anywhere">New token for <span class="font-mono text-[0.8125rem]">{minted.grant.hostname}</span></label>
      <div class="flex min-w-0 gap-2">
        <input id="minted-token" bind:this={tokenInput} class={[INPUT_CLASSES, 'min-w-0 font-mono text-[0.8125rem]']} readonly value={minted.token} aria-describedby="token-once" onfocus={(event) => event.currentTarget.select()} onclick={(event) => event.currentTarget.select()} />
        <Button disabled={pending?.kind === "mint"} onclick={copy}>Copy</Button>
      </div>
      <p id="token-once" class="mt-1.5 text-[0.8125rem] text-muted">Copy now; you cannot retrieve it later.</p>
      <p class={['text-[0.8125rem] text-muted', copyNotice && 'mt-1.5']} aria-live="polite">{copyNotice}</p>
    </div>
  {/if}

  <div aria-live="polite" aria-busy={busy}>
    {#if error}
      <div class="my-2.5 rounded-md border border-danger/40 bg-chrome px-3 py-2 text-danger wrap-anywhere">
        {#if error.length > 180}<details open><summary class="cursor-pointer font-medium">Token request needs attention</summary><p class="mt-2">{error}</p></details>{:else}{error}{/if}
      </div>
    {/if}
    {#if notice}<p class="my-2.5 text-success">{notice}</p>{/if}
  </div>

  {#if grants?.length}
    <ScrollRegion label="Domain tokens" class="bg-chrome">
      <table class="w-full min-w-[640px] table-fixed border-collapse text-start [&_td]:px-3.5 [&_td]:py-2.5 [&_td]:align-top [&_th]:px-3.5 [&_th]:py-2.5 [&_th]:text-start">
        <thead class="text-xs text-muted [&_th]:font-medium"><tr><th scope="col" class="w-[28%]">Hostname</th><th scope="col" class="w-[30%]">Grant ID</th><th scope="col" class="w-[26%]">Expiry</th><th scope="col" class="w-[16%]">Action</th></tr></thead>
        <tbody>
          {#each grants as grant (grant.id)}
            <tr class={['border-t border-line hover:bg-surface/50', confirmation?.grant.id === grant.id && 'bg-surface']}>
              <th scope="row" class="font-mono text-[0.8125rem] font-medium wrap-anywhere">{grant.hostname || "Unknown"}</th>
              <td><span class="block truncate font-mono text-[0.8125rem]" title={grant.id}>{grant.id}</span></td>
              <td class="text-[0.8125rem]">{#if grant.incomplete}<span class="text-warning">Incomplete</span>{:else}{grant.expiresAt ? new Date(grant.expiresAt).toLocaleString() : "No expiry"}{/if}</td>
              <td><Button size="sm" variant="danger" disabled={busy} aria-label={`Revoke ${grant.id}`} aria-expanded={confirmation?.grant.id === grant.id} onclick={(event) => askRevoke(grant, event.currentTarget)}>Revoke</Button></td>
            </tr>
          {/each}
        </tbody>
      </table>
    </ScrollRegion>
  {:else}
    <p class="rounded-md border border-line bg-chrome px-3.5 py-3 text-muted">{grants ? "No domain tokens." : refreshing ? "Loading tokens…" : "Token list unavailable."}</p>
  {/if}
  {#if confirmation}
    <div class="mt-2.5 flex min-w-0 flex-wrap items-center justify-between gap-3 rounded-md border border-line bg-surface px-3.5 py-3" role="group" aria-label="Confirm token revocation">
      <div class="min-w-0 max-w-[68ch] text-zinc-300">
        <p class="wrap-anywhere">Revoke token{#if confirmation.grant.hostname} for <span class="font-mono text-[0.8125rem]">{confirmation.grant.hostname}</span>{/if}?</p>
        <p class="mt-1 text-[0.8125rem] text-muted">Ends this token's access; running sidecars may take up to 2 minutes to disconnect.</p>
      </div>
      <div class="flex flex-wrap gap-2"><Button bind:element={cancelButton} disabled={Boolean(pending)} onclick={cancelRevoke}>Cancel</Button><Button variant="danger" disabled={busy} onclick={revoke}>{pending?.kind === "revoke" && pending.id === confirmation.grant.id ? "Revoking…" : "Confirm revoke"}</Button></div>
    </div>
  {/if}
</section>
