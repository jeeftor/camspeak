<script lang="ts">
  import { onDestroy } from 'svelte'
  import { Airplay, Bell, Info, Loader2, SlidersHorizontal, Video, VideoOff } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import GainSlider from '$lib/components/GainSlider.svelte'
  import CameraInfoModal from './CameraInfoModal.svelte'
  import PlaybackStrip from './PlaybackStrip.svelte'
  import { apiClient } from '$lib/api'
  import { saveCameraGain, uploadAudioToCamera } from '$lib/audio-actions'
  import type { CameraSummary } from '$lib/types'
  import type { AudioDraft } from '$lib/audio-draft'
  import type { PlaybackMonitor } from '$lib/playback.svelte'

  let { camera, draft = $bindable(), monitor, onOpen }: {
    camera: CameraSummary
    draft: AudioDraft
    monitor: PlaybackMonitor
    onOpen: () => void
  } = $props()
  let preview = $state(false)
  let previewSrc = $state('')
  let previewError = $state('')
  let previewLoading = $state(false)
  let previewTime = $state('')
  let previewTimer: ReturnType<typeof setTimeout> | undefined
  let previewController: AbortController | undefined
  let uploadController: AbortController | undefined
  let showInfo = $state(false)
  let status = $state('')
  let failed = $state(false)
  let dragOver = $state(false)
  let statusTimer: ReturnType<typeof setTimeout> | undefined

  function feedback(message: string, error = false) {
    clearTimeout(statusTimer)
    status = message
    failed = error
    if (!error) statusTimer = setTimeout(() => { status = '' }, 5000)
  }

  async function snapshot() {
    if (!preview) return
    const controller = new AbortController()
    previewController = controller
    previewLoading = true
    try {
      const response = await apiClient.snapshot(camera.name, undefined, undefined, controller.signal)
      const blob = await response.blob()
      if (!preview || controller.signal.aborted) return
      if (previewSrc) URL.revokeObjectURL(previewSrc)
      previewSrc = URL.createObjectURL(blob)
      previewTime = new Date().toLocaleTimeString()
      previewError = ''
    } catch (cause) {
      if (!controller.signal.aborted) previewError = cause instanceof Error ? cause.message : String(cause)
    } finally {
      if (!controller.signal.aborted) {
        previewLoading = false
        // Schedule after completion so slow cameras never overlap requests.
        if (preview) previewTimer = setTimeout(snapshot, 2000)
      }
    }
  }

  function stopPreview() {
    preview = false
    clearTimeout(previewTimer)
    previewController?.abort()
    if (previewSrc) URL.revokeObjectURL(previewSrc)
    previewSrc = ''
    previewError = ''
    previewLoading = false
  }

  function togglePreview() {
    if (preview) stopPreview()
    else { preview = true; void snapshot() }
  }

  async function saveGain() {
    try { await saveCameraGain(camera.name, draft) }
    catch (cause) { feedback(`Volume could not be saved: ${cause instanceof Error ? cause.message : String(cause)}`, true) }
  }

  async function beep() {
    if (draft.busy) return
    clearTimeout(statusTimer)
    status = ''
    draft.busy = true
    try { await apiClient.beep({ camera: camera.name }); feedback('Test tone sent') }
    catch (cause) { feedback(cause instanceof Error ? cause.message : String(cause), true) }
    finally { draft.busy = false; void monitor.refresh() }
  }

  function drag(event: DragEvent) {
    if (!draft.busy && Array.from(event.dataTransfer?.items ?? []).some(item => item.kind === 'file')) {
      event.preventDefault()
      dragOver = true
    }
  }

  async function drop(event: DragEvent) {
    event.preventDefault()
    dragOver = false
    const file = event.dataTransfer?.files[0]
    if (!file || draft.busy || camera.capabilities?.speak === false) return
    if (!file.type.startsWith('audio/') && !/\.(wav|mp3|m4a|aac|flac|ogg|opus)$/i.test(file.name)) {
      feedback('Choose an audio file to play on your camera.', true)
      return
    }
    draft.busy = true
    clearTimeout(statusTimer)
    failed = false
    status = 'Uploading your audio…'
    uploadController = new AbortController()
    try {
      await uploadAudioToCamera(camera.name, file, progress => {
        status = `${progress.step} (${Math.round(progress.percent)}%)`
      }, uploadController.signal)
      feedback(`Audio sent: ${file.name}`)
    } catch (cause) {
      if (!uploadController.signal.aborted) {
        if (cause instanceof DOMException && cause.name === 'AbortError') feedback('Audio canceled')
        else feedback(cause instanceof Error ? cause.message : String(cause), true)
      }
    } finally { draft.busy = false; void monitor.refresh() }
  }

  onDestroy(() => {
    stopPreview()
    uploadController?.abort()
    clearTimeout(statusTimer)
  })
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<article ondragover={drag} ondragleave={() => dragOver = false} ondrop={drop}
  class="flex h-full min-w-0 flex-col gap-4 rounded-xl border bg-card p-4 shadow-sm transition-colors {dragOver ? 'border-dashed border-primary bg-primary/5' : 'hover:border-primary/40'}">
  <div class="flex min-w-0 items-start justify-between gap-3">
    <div class="min-w-0">
      <h3 class="break-words text-base font-semibold leading-snug">{camera.name}</h3>
      <p class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
        <span class="flex items-center gap-1.5"><span class="h-1.5 w-1.5 rounded-full {camera.online ? 'bg-green-500' : 'bg-muted-foreground'}"></span>{camera.online ? 'Online' : 'Offline'}</span>
        <span>{camera.type}</span>
        {#if camera.airplay_enabled}<span class="flex items-center gap-1" title={camera.airplay_name}><Airplay class="h-3 w-3" /> AirPlay</span>{/if}
      </p>
    </div>
    <details class="relative shrink-0">
      <summary aria-label={`More actions for ${camera.name}`} class="cursor-pointer rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-accent">More</summary>
      <div class="absolute right-0 z-20 mt-2 flex w-44 flex-col gap-1 rounded-lg border bg-popover p-2 shadow-lg">
        <Button size="sm" variant="ghost" class="justify-start" onclick={() => showInfo = true}><Info class="h-4 w-4" /> Device information</Button>
        <Button size="sm" variant="ghost" class="justify-start" onclick={beep} disabled={draft.busy || camera.capabilities?.speak === false}><Bell class="h-4 w-4" /> Test speaker</Button>
        {#if camera.note}<p class="break-words px-2 py-1 text-xs text-muted-foreground">{camera.note}</p>{/if}
      </div>
    </details>
  </div>

  {#if preview}
    <div class="flex flex-col gap-1.5">
      {#if previewSrc}<img src={previewSrc} alt={`${camera.name} preview`} class="aspect-video w-full rounded-lg object-contain bg-muted" />
      {:else}<div class="flex aspect-video items-center justify-center rounded-lg bg-muted text-sm text-muted-foreground">{previewLoading ? 'Loading preview…' : 'Preview unavailable'}</div>{/if}
      <p class="text-xs text-muted-foreground">{previewError ? `Preview unavailable: ${previewError}` : `Updated ${previewTime || '…'}`}</p>
    </div>
  {/if}

  <PlaybackStrip cameraName={camera.name} playback={monitor.state.cameras[camera.name]} preparing={draft.busy} level={monitor.state.levels[camera.name]} onRefresh={monitor.refresh} />

  <div class="mt-auto flex flex-col gap-3">
    <GainSlider bind:value={draft.gain} onchange={saveGain} aria-label={`${camera.name} volume`} />
    <div class="flex flex-wrap gap-2">
      <Button class="flex-1" size="sm" onclick={onOpen}><SlidersHorizontal class="h-4 w-4" /> Audio controls</Button>
      {#if camera.capabilities?.snapshot !== false}
        <Button size="sm" variant="outline" onclick={togglePreview} aria-pressed={preview} aria-label={`${preview ? 'Hide' : 'Show'} ${camera.name} preview`}>
          {#if preview}<VideoOff class="h-4 w-4" />{:else}<Video class="h-4 w-4" />{/if} Preview
        </Button>
      {/if}
    </div>
    {#if draft.busy}<p class="flex items-center gap-2 text-xs text-muted-foreground"><Loader2 class="h-3 w-3 animate-spin" /> Sending audio…</p>{/if}
    {#if status}<p role={failed ? 'alert' : 'status'} class="break-words text-xs {failed ? 'text-destructive' : 'text-primary'}">{status}</p>{/if}
  </div>
</article>

<CameraInfoModal cameraName={camera.name} cameraType={camera.type} show={showInfo} onClose={() => showInfo = false} />
