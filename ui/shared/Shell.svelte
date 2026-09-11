<script lang="ts">
  import type { Snippet } from 'svelte';

  type Props = {
    label: string;
    home: string;
    navigation?: { label: string; href: string; active?: boolean }[];
    children: Snippet;
  };

  let { label, home, navigation = [], children }: Props = $props();
</script>

<a
  href="#main-content"
  class="sr-only focus:not-sr-only focus:fixed focus:top-3 focus:left-3 focus:z-50 focus:rounded-md focus:bg-surface focus:px-3 focus:py-2 focus:text-accent-300"
>Skip to content</a>

<header class="border-b border-line bg-chrome">
  <div class="mx-auto flex min-h-14 max-w-6xl flex-wrap items-center gap-x-5 gap-y-1 px-4 py-2 sm:px-6">
    <a href={home} class="flex min-h-10 items-center gap-2.5 rounded-sm text-foreground">
      <span class="text-lg font-semibold tracking-tight">specter</span>
      <span class="border-l border-line pl-2.5 text-sm text-muted">{label}</span>
    </a>
    {#if navigation.length}
      <nav aria-label="Operator" class="flex flex-wrap items-center gap-1">
        {#each navigation as item (item.href)}
          <a
            href={item.href}
            aria-current={item.active ? 'page' : undefined}
            class={[
              'inline-flex min-h-10 items-center rounded-md px-3 text-sm font-medium',
              item.active ? 'bg-surface text-accent-300' : 'text-muted hover:bg-surface hover:text-foreground',
            ]}
          >{item.label}</a>
        {/each}
      </nav>
    {/if}
  </div>
</header>

<main id="main-content" tabindex="-1" class="mx-auto w-full min-w-0 max-w-6xl px-4 py-5 sm:px-6 sm:py-6">
  {@render children()}
</main>
