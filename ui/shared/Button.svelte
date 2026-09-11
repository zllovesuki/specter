<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';

  type Props = HTMLButtonAttributes & {
    variant?: 'primary' | 'secondary' | 'danger' | 'ghost';
    size?: 'sm' | 'md';
    element?: HTMLButtonElement;
    children?: Snippet;
  };

  let {
    variant = 'secondary',
    size = 'md',
    element = $bindable(),
    type = 'button',
    class: className,
    children,
    ...attributes
  }: Props = $props();

  const variants = {
    primary: 'border-accent-500 bg-accent-500 text-chrome hover:border-accent-400 hover:bg-accent-400',
    secondary: 'border-zinc-500 bg-surface text-foreground hover:border-zinc-400 hover:bg-zinc-700',
    danger: 'border-danger/70 bg-danger/10 text-danger hover:border-danger hover:bg-danger/20',
    ghost: 'border-transparent bg-transparent text-muted hover:bg-surface hover:text-foreground',
  };
  const sizes = {
    sm: 'min-h-9 px-2.5 py-1 text-sm',
    md: 'min-h-10 px-3 py-1.5 text-sm',
  };
</script>

<button
  bind:this={element}
  {type}
  class={[
    'inline-flex shrink-0 items-center justify-center gap-2 rounded-md border font-semibold leading-5 transition-transform active:scale-[0.98] disabled:pointer-events-none disabled:opacity-50',
    variants[variant],
    sizes[size],
    className,
  ]}
  {...attributes}
>
  {@render children?.()}
</button>
