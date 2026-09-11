<script lang="ts">
  let { value }: { value: string } = $props();
  const formatTime = (timestamp: string) => {
    const date = new Date(timestamp);
    return Number.isNaN(date.getTime())
      ? timestamp
      : date.toISOString().replace('T', ' ').replace(/\.\d{3}Z$/, ' UTC');
  };
</script>

<time datetime={value} class="tabular-nums">{formatTime(value)}</time>
