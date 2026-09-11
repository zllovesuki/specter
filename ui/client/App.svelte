<script lang="ts">
  import { onMount, tick } from "svelte";
  import Button from "../shared/Button.svelte";
  import Shell from "../shared/Shell.svelte";
  import ScrollRegion from "../shared/ScrollRegion.svelte";
  import CustomDomains from "./CustomDomains.svelte";
  import {
    APIError,
    createAPI,
    errorMessage,
    type ClientStatus,
    type RegisteredTunnel,
    type TunnelSyncResult,
  } from "./api";

  interface TunnelRow extends Partial<TunnelSyncResult> {
    hostname: string;
    target: string;
    headerHost?: string;
    registered: boolean;
  }

  type RemovalAction = "unpublish" | "release";
  type Confirmation = { action: RemovalAction; tunnel: TunnelRow; trigger: HTMLButtonElement };
  type Notice = {
    tone: "success" | "warning" | "error";
    title: string;
    detail?: string;
    saveFailed?: boolean;
    restore?: string | null;
  };

  let status = $state<ClientStatus | null>(null);
  let registered = $state<RegisteredTunnel[] | null>(null);
  let statusError = $state("");
  let listError = $state("");
  let statusCheckedAt = $state<Date | null>(null);
  let listCheckedAt = $state<Date | null>(null);
  let refreshing = $state(false);
  let checkingStatus = $state(false);
  let pendingAction = $state<RemovalAction | "synchronize" | "">("");
  let confirmation = $state<Confirmation | null>(null);
  let confirmButton = $state<HTMLButtonElement>();
  let tunnelsHeading = $state<HTMLHeadingElement>();
  let tunnelNotice = $state<Notice | null>(null);
  let statusRequestID = 0;
  const api = createAPI();

  const synchronization = $derived(status?.synchronization);
  const connectedNodes = $derived(status?.connectedNodes ?? []);
  const actionBusy = $derived(Boolean(pendingAction));
  const unsavedChanges = $derived(Boolean(synchronization?.attemptedAt && !synchronization.saved));
  const tunnelRows = $derived(mergeTunnels(registered, synchronization?.tunnels));
  const configuredCount = $derived(synchronization?.tunnels?.length ?? 0);
  const syncIssue = $derived<Notice | null>(
    synchronization?.error || status?.pending || unsavedChanges
      ? {
          tone: "warning",
          title: unsavedChanges ? "Changes not saved." : "Synchronization incomplete.",
          detail: synchronization?.error,
        }
      : null,
  );
  const syncNotice = $derived(
    syncIssue && tunnelNotice?.tone === "success" ? syncIssue : tunnelNotice || syncIssue,
  );

  function mergeTunnels(hostnames: RegisteredTunnel[] | null, outcomes?: TunnelSyncResult[]): TunnelRow[] {
    const rows = new Map<string, TunnelRow>();
    for (const [index, tunnel] of (outcomes ?? []).entries()) {
      rows.set(tunnel.hostname || `unassigned:${index}`, { ...tunnel, registered: false });
    }
    for (const tunnel of hostnames ?? []) {
      const outcome = rows.get(tunnel.hostname);
      rows.set(tunnel.hostname, {
        ...tunnel,
        target: outcomes ? "" : tunnel.target === "(unused)" ? "" : tunnel.target,
        headerHost: outcome && outcome.target !== tunnel.target ? "" : tunnel.headerHost,
        ...outcome,
        registered: true,
      });
    }
    return [...rows.values()];
  }

  function timeLabel(value: string | Date | null | undefined): string {
    return value
      ? new Date(value).toLocaleTimeString([], {
          hour: "2-digit",
          minute: "2-digit",
          second: "2-digit",
        })
      : "Not checked";
  }

  function publicationLabel(tunnel: TunnelRow): string {
    if (!tunnel.target) return tunnel.registered ? "Reserved" : "No target";
    if (tunnel.published && tunnel.error) return "Partially published";
    if (tunnel.published) return "Published";
    if (tunnel.error) return "Not published";
    return "Not checked";
  }

  function restorationSnippet(tunnel: TunnelRow): string {
    return `  - hostname: ${JSON.stringify(tunnel.hostname)}\n    target: ${JSON.stringify(tunnel.target || "http://127.0.0.1:8080")}${tunnel.headerHost ? `\n    headerHost: ${JSON.stringify(tunnel.headerHost)}` : ""}`;
  }

  async function loadStatus(force = false) {
    if (checkingStatus && !force) return;
    const requestID = ++statusRequestID;
    checkingStatus = true;
    try {
      const nextStatus = await api.request<ClientStatus>("/api/status");
      if (requestID !== statusRequestID) return;
      status = nextStatus;
      statusCheckedAt = new Date();
      statusError = "";
    } catch (error) {
      if (requestID === statusRequestID) statusError = errorMessage(error);
    } finally {
      if (requestID === statusRequestID) checkingStatus = false;
    }
  }

  async function refresh() {
    if (refreshing || pendingAction) return;
    refreshing = true;
    await Promise.allSettled([
      loadStatus(true),
      (async () => {
        try {
          registered = await api.request<RegisteredTunnel[]>("/api/ls");
          listCheckedAt = new Date();
          listError = "";
        } catch (error) {
          listError = errorMessage(error);
        }
      })(),
    ]);
    refreshing = false;
  }

  async function synchronize() {
    if (pendingAction || refreshing) return;
    pendingAction = "synchronize";
    tunnelNotice = null;
    try {
      await api.request("/api/reload", { method: "POST" });
      tunnelNotice = { tone: "success", title: "Synchronized and saved." };
    } catch (error) {
      const outcome = error instanceof APIError ? error.outcome : undefined;
      if (outcome?.applied && outcome.tunnels && status) {
        status = {
          ...status,
          synchronization: {
            applied: true,
            saved: outcome.saved ?? false,
            error: outcome.error,
            attemptedAt: outcome.attemptedAt,
            tunnels: outcome.tunnels,
          },
        };
      }
      tunnelNotice = {
        tone: outcome?.applied ? "warning" : "error",
        title: outcome?.applied
          ? outcome.saved
            ? "Saved; publication incomplete."
            : "Applied; save failed."
          : outcome
            ? "Configuration not applied."
            : "Synchronization result unknown.",
        detail: errorMessage(error),
      };
    } finally {
      pendingAction = "";
      await refresh();
    }
  }

  async function askRemoval(action: RemovalAction, tunnel: TunnelRow, trigger: HTMLButtonElement) {
    if (pendingAction || refreshing) return;
    confirmation = { action, tunnel, trigger };
    await tick();
    confirmButton?.focus();
  }

  function cancelRemoval() {
    if (pendingAction) return;
    const trigger = confirmation?.trigger;
    confirmation = null;
    trigger?.focus();
  }

  async function removeTunnel() {
    if (!confirmation || pendingAction || refreshing) return;
    const { action, tunnel } = confirmation;
    pendingAction = action;
    tunnelNotice = null;
    try {
      await api.request(`/api/${action}/${encodeURIComponent(tunnel.hostname)}`, { method: "POST" });
      tunnelNotice = {
        tone: "success",
        title: `${tunnel.hostname} ${action === "unpublish" ? "unpublished" : "released"}.`,
        restore: action === "unpublish" ? restorationSnippet(tunnel) : null,
      };
      confirmation = null;
    } catch (error) {
      const outcome = error instanceof APIError ? error.outcome : undefined;
      const applied = outcome?.applied;
      tunnelNotice = {
        tone: applied ? "warning" : "error",
        title: applied
          ? `${tunnel.hostname} ${action === "unpublish" ? "unpublished" : "released"}; saving failed.`
          : outcome
            ? `Could not ${action} ${tunnel.hostname}.`
            : `Could not confirm ${action} for ${tunnel.hostname}.`,
        detail: errorMessage(error),
        saveFailed: applied,
        restore: applied && action === "unpublish" ? restorationSnippet(tunnel) : null,
      };
      if (applied) confirmation = null;
    } finally {
      pendingAction = "";
      await refresh();
      if (!confirmation) tunnelsHeading?.focus();
    }
  }

  onMount(() => {
    void refresh();
    const timer = setInterval(() => {
      if (!refreshing && !pendingAction && !document.hidden) void loadStatus();
    }, 15_000);
    return () => {
      clearInterval(timer);
      api.abort();
    };
  });
</script>

<svelte:head>
  <title>Specter · Client manager</title>
  <meta name="description" content="Manage Specter tunnels and custom domains." />
</svelte:head>

<svelte:window onkeydown={(event) => { if (event.key === "Escape") cancelRemoval(); }} />

<Shell label="Client manager" home="#main-content">
  <section id="tunnels" class="min-w-0 scroll-mt-6" aria-labelledby="tunnels-heading">
    <div class="mb-3.5 flex flex-col items-start justify-between gap-3.5 min-[600px]:flex-row min-[600px]:items-center min-[600px]:gap-4">
      <h1 id="tunnels-heading" class="text-2xl leading-tight font-bold tracking-tight" bind:this={tunnelsHeading} tabindex="-1">
        Tunnels <span class="ms-1.5 text-sm font-medium text-muted">{tunnelRows.length}</span>
      </h1>
      <div class="flex flex-wrap items-center gap-2">
        <Button onclick={refresh} disabled={refreshing || actionBusy}>{refreshing ? "Refreshing…" : "Refresh"}</Button>
        <Button variant="primary" onclick={synchronize} disabled={actionBusy || refreshing}>{pendingAction === "synchronize" ? "Synchronizing…" : "Synchronize"}</Button>
      </div>
    </div>

    <div class="mb-4 flex flex-col gap-2.5 border-y border-line py-2.5 min-[600px]:flex-row min-[600px]:flex-wrap min-[600px]:gap-x-8 min-[600px]:gap-y-4 [&>div]:flex [&>div]:min-w-0 [&>div]:flex-wrap [&>div]:items-center [&>div]:gap-2.5" aria-busy={checkingStatus}>
      <div><span class="min-w-[7.625rem] text-[0.8125rem] text-muted min-[600px]:min-w-0">Server</span><span class="font-mono text-[0.8125rem] wrap-anywhere">{status?.apex || (statusError ? "Unavailable" : "Loading…")}</span></div>
      <div>
        <span class="min-w-[7.625rem] text-[0.8125rem] text-muted min-[600px]:min-w-0">Connected</span>
        <span class={status && !connectedNodes.length ? 'text-warning' : ''}>
          {#if status}<span class={['me-1.5 inline-block size-1.5 rounded-full align-[1px]', connectedNodes.length ? 'bg-success' : 'bg-warning']} aria-hidden="true"></span>{connectedNodes.length} {connectedNodes.length === 1 ? "node" : "nodes"}{:else}Not checked{/if}
        </span>
      </div>
      <div><span class="min-w-[7.625rem] text-[0.8125rem] text-muted min-[600px]:min-w-0">Last sync</span><span>{synchronization?.attemptedAt ? timeLabel(synchronization.attemptedAt) : "Not attempted"}</span></div>
    </div>

    {#if syncNotice}
      <div class={[
        'my-2.5 rounded-md border bg-chrome px-3 py-2 wrap-anywhere',
        syncNotice.tone === 'success' && 'border-success/40 text-success',
        syncNotice.tone === 'warning' && 'border-warning/40 text-warning',
        syncNotice.tone === 'error' && 'border-danger/40 text-danger',
      ]} role="status">
        <strong class="font-semibold">{syncNotice.title}</strong>
        {#if status?.pending}<span> Retry {status.retryAt ? timeLabel(status.retryAt) : "pending"}.</span>{/if}
        {#if unsavedChanges || (syncNotice.saveFailed && statusError)}<p class="mt-1">Review the YAML before reloading or restarting.</p>{/if}
        {#if syncNotice.detail}<details class="mt-1"><summary class="cursor-pointer text-[0.8125rem] text-zinc-200">Error details</summary><p class="mt-2">{syncNotice.detail}</p></details>{/if}
        {#if syncNotice.restore}
          <details class="my-2"><summary class="cursor-pointer py-1 font-medium text-zinc-200">Restore tunnel</summary><p class="mt-2">Add under <code>tunnels:</code>, restore other options, then synchronize.</p><pre class="my-3 overflow-auto rounded-md border border-line bg-chrome px-3.5 py-3 font-mono text-[0.8125rem] leading-relaxed whitespace-pre-wrap">{syncNotice.restore}</pre></details>
        {/if}
      </div>
    {/if}
    {#if statusError || listError}
      <div class="my-2.5 rounded-md border border-danger/40 bg-chrome px-3 py-2 text-danger wrap-anywhere" role="status">
        {#if statusError}<div><strong class="font-semibold">Status unavailable.</strong>{#if status} Snapshot {timeLabel(statusCheckedAt)}.{/if}<details class="mt-1"><summary class="cursor-pointer text-[0.8125rem] text-zinc-200">Status error</summary><p class="mt-2">{statusError}</p></details></div>{/if}
        {#if listError}<div><strong class="font-semibold">Hostnames unavailable.</strong>{#if registered} Snapshot {timeLabel(listCheckedAt)}.{/if}<details class="mt-1"><summary class="cursor-pointer text-[0.8125rem] text-zinc-200">Hostname error</summary><p class="mt-2">{listError}</p></details></div>{/if}
      </div>
    {/if}

    <div aria-busy={refreshing}>
      {#if registered === null && refreshing && !tunnelRows.length}
        <div class="rounded-md border border-line bg-chrome px-4 py-4.5" role="status">Loading tunnels…</div>
      {:else if tunnelRows.length === 0}
        <div class="rounded-md border border-line bg-chrome px-4 py-4.5">{#if statusError || listError}No data. Refresh to retry.{:else}No tunnels. <a href="#configuration" class="text-accent-300 underline underline-offset-4 hover:text-accent-200">Add a target</a>, then synchronize.{/if}</div>
      {:else}
        <ScrollRegion label="Tunnels" class="bg-chrome">
          <table class="w-full min-w-[760px] table-fixed border-collapse text-start [&_td]:px-3.5 [&_td]:py-2.5 [&_td]:align-top [&_td]:wrap-anywhere [&_th]:px-3.5 [&_th]:py-2.5 [&_th]:text-start [&_th]:align-top [&_th]:wrap-anywhere">
            <thead class="text-xs text-muted [&_th]:font-medium"><tr><th class="w-1/4" scope="col">Hostname</th><th class="w-[27%]" scope="col">Local target</th><th class="w-1/4" scope="col">Publication</th><th class="w-[23%]" scope="col">Actions</th></tr></thead>
            <tbody>
              {#each tunnelRows as tunnel, index (tunnel.hostname || `unassigned:${index}`)}
                <tr class="border-t border-line hover:bg-surface/50">
                  <th scope="row" class="font-medium"><span class="font-mono text-[0.8125rem]">{tunnel.hostname || "Awaiting hostname"}</span></th>
                  <td><span class={['font-mono text-[0.8125rem]', !tunnel.target && 'text-muted']}>{tunnel.target || "None"}</span>{#if tunnel.headerHost}<span class="mt-1 block text-xs text-muted">Host: <span class="font-mono">{tunnel.headerHost}</span></span>{/if}</td>
                  <td>
                    <span class={['text-[0.8125rem]', tunnel.published && !tunnel.error && 'text-success', tunnel.error && 'text-warning']}>{publicationLabel(tunnel)}</span>
                    {#if tunnel.published}<span class="mt-1 block text-xs text-muted">{tunnel.publishedEndpoints} {tunnel.publishedEndpoints === 1 ? "endpoint" : "endpoints"}</span>{/if}
                    {#if tunnel.error}<details class="mt-1"><summary class="cursor-pointer text-[0.8125rem] text-zinc-200">Error</summary><p class="mt-2 text-xs text-danger">{tunnel.error}</p></details>{/if}
                  </td>
                  <td>
                    <div class="flex flex-wrap items-center justify-end gap-1.5">
                      <Button size="sm" disabled={actionBusy || refreshing || !tunnel.hostname || !tunnel.target} aria-label={`Unpublish ${tunnel.hostname || "unassigned tunnel"}`} onclick={(event) => askRemoval("unpublish", tunnel, event.currentTarget)}>Unpublish</Button>
                      <Button size="sm" variant="danger" disabled={actionBusy || refreshing || !tunnel.hostname} aria-label={`Release ${tunnel.hostname || "unassigned tunnel"}`} onclick={(event) => askRemoval("release", tunnel, event.currentTarget)}>Release</Button>
                    </div>
                  </td>
                </tr>
                {#if confirmation?.tunnel.hostname === tunnel.hostname}
                  <tr class="border-t border-line bg-surface"><td colspan="4">
                    <div class="flex flex-wrap items-center justify-between gap-4" role="group" aria-label={`${confirmation.action === "unpublish" ? "Unpublish" : "Release"} ${tunnel.hostname}`}>
                      <p class="max-w-[68ch] text-zinc-300">{confirmation.action === "unpublish" ? "Remove target from YAML; keep hostname." : "Remove target from YAML; release hostname for others to claim."}</p>
                      <div class="flex flex-wrap items-center gap-2"><Button onclick={cancelRemoval} disabled={Boolean(pendingAction)}>Cancel</Button><Button bind:element={confirmButton} variant="danger" onclick={removeTunnel} disabled={actionBusy || refreshing}>{pendingAction ? "Applying…" : confirmation.action === "unpublish" ? "Confirm unpublish" : "Confirm release"}</Button></div>
                    </div>
                  </td></tr>
                {/if}
              {/each}
            </tbody>
          </table>
        </ScrollRegion>
      {/if}
    </div>
    <p class="mt-2.5 text-[0.8125rem] text-muted min-[600px]:text-xs">Publication: last acknowledgement; target health unchecked.</p>
  </section>

  <div class="mt-6 grid grid-cols-1 gap-7 border-t border-line pt-5 min-[600px]:grid-cols-2 min-[800px]:gap-8">
    <CustomDomains />
    <section id="configuration" class="min-w-0 scroll-mt-6 border-t border-line pt-5 min-[600px]:border-0 min-[600px]:pt-0" aria-labelledby="configuration-heading">
      <h2 id="configuration-heading" class="mb-3.5 text-lg leading-snug font-semibold tracking-tight">Configuration</h2>
      <dl class="mb-2.5 [&>div]:grid [&>div]:grid-cols-[9.375rem_minmax(0,1fr)] [&>div]:gap-4 [&>div]:py-0.5 [&_dd]:wrap-anywhere [&_dt]:text-muted">
        <div><dt>Configured tunnels</dt><dd>{status ? configuredCount : "Unavailable"}</dd></div>
        <div><dt>Disk state</dt><dd class={synchronization?.attemptedAt && !synchronization.saved ? 'text-warning' : ''}>{!synchronization?.attemptedAt ? "Not checked" : synchronization.saved ? "Saved" : "Unsaved"}</dd></div>
      </dl>
      <details class="my-2">
        <summary class="cursor-pointer py-1 font-medium text-zinc-200">Edit tunnels</summary>
        <pre class="my-3 overflow-auto rounded-md border border-line bg-chrome px-3.5 py-3 font-mono text-[0.8125rem] leading-relaxed whitespace-pre-wrap" aria-label="Example tunnel configuration">{"tunnels:\n  - target: http://127.0.0.1:8080\n    # hostname: app.example.com"}</pre>
        <p class="mt-1.5 mb-2.5 max-w-[72ch] text-[0.8125rem] leading-relaxed text-muted">Edit your existing YAML, then synchronize. Keep credentials intact. Omit hostname for automatic assignment; otherwise use a reserved or verified hostname.</p>
      </details>
    </section>
  </div>

  <footer class="mt-6 flex justify-end border-t border-line py-3 text-xs text-muted">{statusCheckedAt ? `Checked ${timeLabel(statusCheckedAt)}` : statusError ? "Status unavailable" : "Connecting…"}</footer>
</Shell>
