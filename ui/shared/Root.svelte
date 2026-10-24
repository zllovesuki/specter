<script lang="ts">
  import type { Component } from 'svelte';
  import { QueryClientProvider } from '@tanstack/svelte-query';
  import { queryClient } from './query';

  let { app: App }: { app: Component } = $props();
</script>

<QueryClientProvider client={queryClient}><App /></QueryClientProvider>
