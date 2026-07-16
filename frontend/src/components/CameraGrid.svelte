<script>
  import CameraCard from './CameraCard.svelte'
  import { Button } from '$lib/components/ui/button'

  let { cameras = [], voices = [], presets = [], onRefresh } = $props()
</script>

<div class="mb-4 flex items-center justify-between">
  <h2 class="text-lg font-semibold text-primary">Cameras</h2>
  <Button variant="outline" size="sm" onclick={onRefresh}>↻ Refresh</Button>
</div>

{#if cameras.length === 0}
  <p class="italic text-muted-foreground">No cameras configured.</p>
{:else}
  <div class="grid grid-cols-[repeat(auto-fill,minmax(300px,1fr))] gap-4">
    {#each cameras as cam (cam.name)}
      <CameraCard camera={cam} {voices} {presets} />
    {/each}
  </div>
{/if}
