<script lang="ts">
  import { onMount } from 'svelte'
  import { RefreshCw, ArrowUp, ArrowDown, Radio, ArrowUpDown } from 'lucide-svelte'
  import CameraCard from './CameraCard.svelte'
  import Broadcast from './Broadcast.svelte'
  import Modal from '$lib/components/Modal.svelte'
  import { Button } from '$lib/components/ui/button'
  import { apiClient } from '$lib/api'
  import { createPlaybackMonitor } from '$lib/playback.svelte'
  import { createAudioDraft, type AudioDraft } from '$lib/audio-draft'
  import type { CameraSummary, Preset } from '$lib/types'

  let { cameras = [], voices = [], presets = [], onRefresh }: {
    cameras?: CameraSummary[]
    voices?: string[]
    presets?: Preset[]
    onRefresh?: () => void | Promise<void>
  } = $props()

  const monitor = createPlaybackMonitor()
  const serverGains = new Map<string, number>()
  let localCameras = $state<CameraSummary[]>([])
  let drafts = $state<Record<string, AudioDraft>>({})
  let broadcastDraft = $state(createAudioDraft())
  let arrange = $state(false)
  let reordering = $state(false)
  let refreshing = $state(false)
  let error = $state('')
  let broadcastOpen = $state(false)

  $effect(() => {
    if (!reordering) localCameras = cameras
    for (const camera of cameras) {
      if (!drafts[camera.name]) drafts[camera.name] = createAudioDraft(camera.gain ?? 3, camera.vision_prompt ?? '')
      if (camera.gain !== undefined && !drafts[camera.name].gainSaving && serverGains.get(camera.name) !== camera.gain) {
        drafts[camera.name].gain = camera.gain
        drafts[camera.name].savedGain = camera.gain
        serverGains.set(camera.name, camera.gain)
      }
    }
  })
  onMount(() => monitor.start())

  async function refresh() {
    refreshing = true
    error = ''
    try { await onRefresh?.(); await monitor.refresh() }
    catch (cause) { error = cause instanceof Error ? cause.message : String(cause) }
    finally { refreshing = false }
  }

  async function moveCamera(index: number, direction: number) {
    const next = index + direction
    if (next < 0 || next >= localCameras.length || reordering) return
    reordering = true
    error = ''
    const before = localCameras
    const reordered = [...before]
    ;[reordered[index], reordered[next]] = [reordered[next], reordered[index]]
    localCameras = reordered
    try {
      await apiClient.reorderCameras(reordered.map(camera => camera.name))
      await onRefresh?.()
    } catch (cause) {
      localCameras = before
      error = `Camera order could not be saved: ${cause instanceof Error ? cause.message : String(cause)}`
    } finally { reordering = false }
  }
</script>

<section class="camera-dashboard" aria-label="Camera audio">
<div class="mb-4 flex flex-wrap items-center justify-between gap-3">
  <div>
    <h2 class="text-lg font-semibold">Cameras <span class="ml-1 text-sm font-normal text-muted-foreground">{cameras.length}</span></h2>
    <p class="mt-0.5 text-sm text-muted-foreground">Speak, play a preset, or start a stream on your camera.</p>
  </div>
  <div class="flex flex-wrap items-center gap-2">
    <Button size="sm" onclick={() => broadcastOpen = true} disabled={!cameras.length}><Radio class="h-4 w-4" /> Broadcast</Button>
    {#if cameras.length > 1}<Button variant="outline" size="sm" aria-pressed={arrange} onclick={() => arrange = !arrange}><ArrowUpDown class="h-4 w-4" /> {arrange ? 'Done arranging' : 'Arrange'}</Button>{/if}
    <Button variant="ghost" size="icon" onclick={refresh} disabled={refreshing} aria-label="Refresh cameras"><RefreshCw class="h-4 w-4 {refreshing ? 'animate-spin' : ''}" /></Button>
  </div>
</div>

{#if error}<p role="alert" class="mb-4 text-sm text-destructive">{error}</p>{/if}
{#if monitor.state.error}<p role="status" class="mb-4 text-sm text-muted-foreground">{monitor.state.error}</p>{/if}

{#if localCameras.length === 0}
  <div class="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">Add cameras in Config to start sending audio.</div>
{:else}
  <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3">
    {#each localCameras as camera, index (camera.name)}
      {#if drafts[camera.name]}
        <div class="flex min-w-0 flex-col gap-2">
          {#if arrange}
            <div class="flex items-center justify-between rounded-lg bg-muted px-2 py-1">
              <span class="text-xs text-muted-foreground">Position {index + 1}</span>
              <div class="flex gap-1">
                <Button variant="ghost" size="icon" class="h-7 w-7" disabled={reordering || index === 0} aria-label={`Move ${camera.name} earlier`} onclick={() => moveCamera(index, -1)}><ArrowUp class="h-4 w-4" /></Button>
                <Button variant="ghost" size="icon" class="h-7 w-7" disabled={reordering || index === localCameras.length - 1} aria-label={`Move ${camera.name} later`} onclick={() => moveCamera(index, 1)}><ArrowDown class="h-4 w-4" /></Button>
              </div>
            </div>
          {/if}
          <CameraCard {camera} {voices} {presets} bind:draft={drafts[camera.name]} {monitor} />
        </div>
      {/if}
    {/each}
  </div>
{/if}

<Modal bind:open={broadcastOpen} title="Broadcast">
  {#if broadcastOpen}<Broadcast {voices} {presets} bind:draft={broadcastDraft} {monitor} />{/if}
</Modal>
</section>
