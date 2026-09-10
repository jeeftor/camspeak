<script lang="ts">
  let { level = 0, vertical = false }: { level?: number; vertical?: boolean } = $props()
  const value = $derived(Number.isFinite(level) ? Math.max(0, Math.min(1, level)) : 0)
</script>

<div class="flex shrink-0 gap-1.5 {vertical ? 'flex-col items-center' : 'items-center'}">
  <div role="meter" aria-label="Audio level" aria-valuemin="0" aria-valuemax="100" aria-valuenow={Math.round(value * 100)}
    class="flex gap-px overflow-hidden rounded bg-muted {vertical ? 'h-12 w-3 flex-col justify-end' : 'h-2 w-32'}">
    {#each Array(20) as _, i}
      {@const index = vertical ? 19 - i : i}
      {@const lit = (index + 1) / 20 <= value}
      <div class="flex-1 transition-colors duration-75 {lit ? (index < 12 ? 'bg-green-500' : index < 17 ? 'bg-yellow-500' : 'bg-red-500') : 'bg-muted-foreground/20'}"></div>
    {/each}
  </div>
  <span class="text-xs tabular-nums text-muted-foreground">{Math.round(value * 100)}%</span>
</div>
