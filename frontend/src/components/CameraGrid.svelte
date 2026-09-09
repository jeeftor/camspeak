<script>
  import { RefreshCw, ArrowUp, ArrowDown } from 'lucide-svelte'
  import CameraCard from './CameraCard.svelte'
  import Broadcast from './Broadcast.svelte'
  import { Button } from '$lib/components/ui/button'
  import { apiClient } from '$lib/api'

  let { cameras = [], voices = [], presets = [], onRefresh } = $props()

  let localCameras = $state(cameras)
  let reordering = $state(false)

  // Sync external cameras prop into local state when it changes
  // (e.g. after refresh), but only if we're not in the middle of reordering.
  $effect(() => {
    if (!reordering) {
      localCameras = cameras
    }
  })

  async function moveCamera(index, direction) {
    const newIndex = index + direction
    if (newIndex < 0 || newIndex >= localCameras.length) return
    reordering = true
    // Swap locally for immediate feedback
    const cams = [...localCameras]
    ;[cams[index], cams[newIndex]] = [cams[newIndex], cams[index]]
    localCameras = cams
    // Persist to backend
    try {
      await apiClient.reorderCameras(cams.map(c => c.name))
      // Trigger refresh so the parent picks up the new sort_order
      onRefresh?.()
    } catch (e) {
      // Revert on failure
      localCameras = cameras
    } finally {
      reordering = false
    }
  }
</script>

<div class="mb-4 flex items-center justify-between">
  <h2 class="text-lg font-semibold text-primary">Cameras</h2>
  <Button variant="outline" size="sm" onclick={onRefresh}>
    <RefreshCw class="h-4 w-4" />
    Refresh
  </Button>
</div>

{#if localCameras.length === 0}
  <p class="italic text-muted-foreground">No cameras configured. Run <code class="rounded bg-muted px-1.5 py-0.5 text-sm">camspeak discover</code> or add cameras in Config.</p>
{:else}
  <div class="grid grid-cols-1 sm:grid-cols-[repeat(auto-fill,minmax(320px,1fr))] gap-4">
    {#each localCameras as cam, i (cam.name)}
      <div class="flex flex-col gap-1">
        <div class="flex justify-end gap-0.5">
          <Button
            variant="ghost"
            size="icon"
            class="h-6 w-6 opacity-40 hover:opacity-100"
            disabled={reordering || i === 0}
            onclick={() => moveCamera(i, -1)}
            title="Move up"
          >
            <ArrowUp class="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            class="h-6 w-6 opacity-40 hover:opacity-100"
            disabled={reordering || i === localCameras.length - 1}
            onclick={() => moveCamera(i, 1)}
            title="Move down"
          >
            <ArrowDown class="h-3.5 w-3.5" />
          </Button>
        </div>
        <CameraCard camera={cam} {voices} {presets} />
      </div>
    {/each}
  </div>

  <div class="mt-6 border-t pt-6">
    <Broadcast {voices} {presets} />
  </div>
{/if}
