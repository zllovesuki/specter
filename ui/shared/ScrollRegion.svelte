<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';

  type Props = {
    label: string;
    class?: HTMLAttributes<HTMLDivElement>['class'];
    children: Snippet;
  };

  let { label, class: className, children }: Props = $props();
</script>

<!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard users need to focus overflowing tables to scroll them.) -->
<div
  class={['relative w-full max-w-full overflow-x-auto rounded-md border border-line', className]}
  role="region"
  aria-label={label}
  tabindex="0"
>
  {@render children()}
</div>
