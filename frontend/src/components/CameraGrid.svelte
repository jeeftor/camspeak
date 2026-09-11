<script lang="ts">
  import ErrorNotice from '$lib/components/ErrorNotice.svelte'
  import { onMount } from 'svelte'
  import { RefreshCw, Radio, ChevronLeft, ChevronRight } from 'lucide-svelte'
  import CameraCard from './CameraCard.svelte'
  import Broadcast from './Broadcast.svelte'
  import Modal from '$lib/components/Modal.svelte'
  import { Button } from '$lib/components/ui/button'
  import { createPlaybackMonitor } from '$lib/playback.svelte'
  import { createAudioDraft, type AudioDraft } from '$lib/audio-draft'
  import type { CameraSummary, Preset } from '$lib/types'

  let { cameras = [], voices = [], presets = [], onRefresh, onOpenSettings }: {
    cameras?: CameraSummary[]
    voices?: string[]
    presets?: Preset[]
    onRefresh?: () => void | Promise<void>
    onOpenSettings?: (camera: string) => void
  } = $props()

  const monitor = createPlaybackMonitor()
  const serverGains = new Map<string, number>()
  let localCameras = $state<CameraSummary[]>([])
  let drafts = $state<Record<string, AudioDraft>>({})
  let broadcastDraft = $state(createAudioDraft())
  let refreshing = $state(false)
  let error = $state('')
  let broadcastOpen = $state(false)
  let selectedName = $state('')
  const selected = $derived(localCameras.find(camera => camera.name === selectedName) ?? localCameras[0])
  const selectedIndex = $derived(localCameras.findIndex(camera => camera.name === selected?.name))

  function selectNext(direction: number) {
    const next = localCameras[selectedIndex + direction]
    if (next) selectedName = next.name
  }

  $effect(() => {
    localCameras = cameras
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

</script>

<section class="camera-dashboard" aria-label="Camera audio">
<div class="mb-4 flex flex-wrap items-center justify-between gap-3">
  <div>
    <h2 class="text-lg font-semibold">Cameras <span class="ml-1 text-sm font-normal text-muted-foreground">{cameras.length}</span></h2>
    <p class="mt-0.5 text-sm text-muted-foreground">Describe, speak, or play a preset on your camera.</p>
  </div>
  <div class="flex flex-wrap items-center gap-2">
    <Button size="sm" onclick={() => broadcastOpen = true} disabled={!cameras.length}><Radio class="h-4 w-4" /> Broadcast</Button>
    <Button variant="ghost" size="icon" onclick={refresh} disabled={refreshing} aria-label="Refresh cameras"><RefreshCw class="h-4 w-4 {refreshing ? 'animate-spin' : ''}" /></Button>
  </div>
</div>

<ErrorNotice message={error} />
{#if monitor.state.error}<p role="status" class="mb-4 text-sm text-muted-foreground">{monitor.state.error}</p>{/if}

{#if localCameras.length === 0}
  <div class="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">Add cameras in Config to start sending audio.</div>
{:else}
  <div class="mb-3 flex items-end gap-2 lg:hidden">
    <Button size="icon" variant="outline" aria-label="Previous camera" disabled={selectedIndex <= 0} onclick={() => selectNext(-1)}><ChevronLeft class="h-4 w-4" /></Button>
    <label class="flex min-w-0 flex-1 flex-col gap-1 text-xs text-muted-foreground">Camera · {selectedIndex + 1} of {localCameras.length}
      <select aria-label="Select camera" value={selected?.name} onchange={event => selectedName = event.currentTarget.value}
        class="min-w-0 rounded-md border bg-card px-2 py-2 text-foreground">
        {#each localCameras as camera}<option value={camera.name}>{camera.name}</option>{/each}
      </select>
    </label>
    <Button size="icon" variant="outline" aria-label="Next camera" disabled={selectedIndex >= localCameras.length - 1} onclick={() => selectNext(1)}><ChevronRight class="h-4 w-4" /></Button>
  </div>
  <p class="mb-3 text-xs text-muted-foreground lg:hidden">Swipe the camera header or preview to switch cameras.</p>
  <div class="grid min-w-0 gap-4 lg:grid-cols-[11rem_minmax(0,1fr)]">
    <nav aria-label="Camera selection" class="min-w-0 hidden lg:block">
      {#each localCameras as camera (camera.name)}
        <div class="mb-2">
          <button class="w-full rounded-lg border p-3 text-left text-sm {selected?.name === camera.name ? 'border-primary bg-primary/10' : 'bg-card hover:bg-muted'}"
            aria-pressed={selected?.name === camera.name} onclick={() => selectedName = camera.name}>
            <span class="block break-words font-medium">{camera.name}</span>
            <span class="text-xs text-muted-foreground">{drafts[camera.name]?.busy ? 'Working…' : monitor.state.cameras[camera.name]?.state || (camera.online ? 'Online' : 'Offline')}</span>
          </button>
        </div>
      {/each}
    </nav>
    <div class="min-w-0">
      {#each localCameras as camera (camera.name)}
        {#if drafts[camera.name]}
          <div hidden={camera.name !== selected?.name}>
            <CameraCard {camera} {voices} {presets} bind:draft={drafts[camera.name]} {monitor}
              active={camera.name === selected?.name} onMove={selectNext} {onOpenSettings} />
          </div>
        {/if}
      {/each}
    </div>
  </div>
{/if}

<Modal bind:open={broadcastOpen} title="Broadcast">
  {#if broadcastOpen}<Broadcast {voices} {presets} bind:draft={broadcastDraft} {monitor} />{/if}
</Modal>
</section>
