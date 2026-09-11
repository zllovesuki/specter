<script lang="ts">
  import type { Snippet } from 'svelte';
  import ScrollRegion from '../shared/ScrollRegion.svelte';

  let {
    label,
    tableClass = 'min-w-[42rem]',
    children,
  }: {
    label: string;
    tableClass?: string;
    children: Snippet;
  } = $props();
</script>

<ScrollRegion {label}>
  <table
    class="w-full border-collapse text-left [&_th]:px-3 [&_td]:px-3 [&_th]:py-2.5 [&_td]:py-2.5 [&_th]:align-top [&_td]:align-top [&_th]:wrap-anywhere [&_td]:wrap-anywhere [&_th]:border-b [&_td]:border-b [&_th]:border-line [&_td]:border-line [&_thead]:bg-chrome [&_thead_th]:text-xs [&_thead_th]:font-semibold [&_thead_th]:text-muted [&_thead_th]:whitespace-nowrap [&_tbody_th]:font-normal [&_tbody_tr:last-child>*]:border-b-0 [&_tbody_tr:hover]:bg-surface [&_td]:tabular-nums {tableClass}"
  >
    {@render children()}
  </table>
</ScrollRegion>
