<script lang="ts">
  import { onDestroy } from "svelte";
  import Button from "../shared/Button.svelte";
  import { INPUT_CLASSES } from "../shared/styles";
  import { errorMessage, requestAPI, type DomainRecord } from "./api";

  type DomainAction = "acme" | "validate";
  type DomainResult = DomainRecord & { domain: string; verified: boolean };

  let domain = $state("");
  let pending = $state<{ action: DomainAction; domain: string } | null>(null);
  let output = $state<DomainResult | null>(null);
  let error = $state("");
  let domainRevision = 0;
  const requests = new AbortController();

  function editDomain() {
    domainRevision += 1;
    output = null;
    error = "";
  }

  async function domainAction(action: DomainAction) {
    if (pending) return;
    const submittedDomain = domain.trim();
    if (!submittedDomain || /[\s/:]/.test(submittedDomain)) {
      error = "Enter a domain without a protocol or path.";
      return;
    }
    const revision = domainRevision;
    pending = { action, domain: submittedDomain };
    error = "";
    output = null;
    try {
      const record = await requestAPI<DomainRecord>(`/api/${action}/${encodeURIComponent(submittedDomain)}`, { signal: requests.signal });
      if (revision === domainRevision)
        output = { ...record, domain: submittedDomain, verified: action === "validate" };
    } catch (cause) {
      if (revision === domainRevision) error = `${submittedDomain}: ${errorMessage(cause)}`;
    } finally {
      pending = null;
    }
  }

  onDestroy(() => requests.abort());
</script>

<section id="domains" class="min-w-0 scroll-mt-6" aria-labelledby="domains-heading">
  <h2 id="domains-heading" class="mb-3.5 text-lg leading-snug font-semibold tracking-tight">Custom domains</h2>
  <form onsubmit={(event) => { event.preventDefault(); void domainAction("acme"); }}>
    <label for="domain" class="mb-1.5 block font-medium">Domain</label>
    <input
      id="domain"
      name="domain"
      type="text"
      class={INPUT_CLASSES}
      bind:value={domain}
      oninput={editDomain}
      placeholder="app.example.com"
      autocomplete="off"
      autocapitalize="none"
      spellcheck="false"
      maxlength="253"
      required
      aria-describedby="domain-hint domain-feedback"
      aria-invalid={Boolean(error)}
    />
    <p id="domain-hint" class="mt-1.5 mb-2.5 text-[0.8125rem] text-muted">Add the record in DNS, then verify.</p>
    <div class="flex flex-wrap items-center gap-2">
      <Button type="submit" disabled={Boolean(pending) || !domain.trim()}>{pending?.action === "acme" ? "Getting record…" : "Get DNS record"}</Button>
      <Button disabled={Boolean(pending) || !domain.trim()} onclick={() => domainAction("validate")}>{pending?.action === "validate" ? "Verifying…" : "Verify ownership"}</Button>
    </div>
  </form>

  <div id="domain-feedback" aria-live="polite" aria-busy={Boolean(pending)}>
    {#if pending}
      <p class="my-2.5 rounded-md border border-line bg-chrome px-3 py-2 wrap-anywhere">
        {pending.action === "validate" ? "Verifying" : "Fetching"} <span class="font-mono text-[0.8125rem]">{pending.domain}</span>…
      </p>
    {/if}
    {#if error}
      <div class="my-2.5 rounded-md border border-danger/40 bg-chrome px-3 py-2 text-danger wrap-anywhere">
        {#if error.length > 120}
          <details><summary class="cursor-pointer py-1 font-medium text-zinc-200">Domain request failed</summary><p class="mt-2">{error}</p></details>
        {:else}
          {error}
        {/if}
      </div>
    {/if}
    {#if output}
      <div class="mt-3.5 border-t border-line pt-3">
        <h3 class={['text-[0.9375rem] font-semibold wrap-anywhere', output.verified && 'text-success']}>
          {output.verified ? "Verified" : "DNS record"} <span class="font-mono text-[0.8125rem]">{output.domain}</span>
        </h3>
        <dl class="mt-3 [&>div]:grid [&>div]:grid-cols-[4rem_minmax(0,1fr)] [&>div]:gap-4 [&>div]:py-1.5 [&_dd]:font-mono [&_dd]:text-[0.8125rem] [&_dd]:wrap-anywhere [&_dt]:text-muted">
          <div><dt>Name</dt><dd>{output.record}</dd></div>
          <div><dt>Type</dt><dd>{output.type}</dd></div>
          <div><dt>Value</dt><dd>{output.content}</dd></div>
        </dl>
        {#if output.verified}<a href="#configuration" class="text-accent-300 underline underline-offset-4 hover:text-accent-200">Add to configuration</a>{/if}
      </div>
    {/if}
  </div>
</section>
