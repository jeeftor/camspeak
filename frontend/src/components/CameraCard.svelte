<script lang="ts">
  import { onDestroy, onMount, untrack } from 'svelte'
  import { Airplay, Bell, Info, Settings, Video, VideoOff } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import CameraInfoModal from './CameraInfoModal.svelte'
  import AudioComposer from './AudioComposer.svelte'
  import { apiClient } from '$lib/api'
  import { uploadAudioToCamera } from '$lib/audio-actions'
  import type { CameraSummary, Preset } from '$lib/types'
  import { cameraSwipe, type AudioDraft } from '$lib/audio-draft'
  import type { PlaybackMonitor } from '$lib/playback.svelte'

  let { camera, voices, presets, draft = $bindable(), monitor, active, onMove, onOpenSettings }: {
    camera: CameraSummary
    voices: string[]
    presets: Preset[]
    draft: AudioDraft
    monitor: PlaybackMonitor
    active: boolean
    onMove: (direction: number) => void
    onOpenSettings?: (camera: string) => void
  } = $props()
  let preview = $state(false)
  let previewSrc = $state('')
  let previewError = $state('')
  let previewLoading = $state(false)
  let previewSlow = $state(false)
  let previewTime = $state('')
  let previewTimer: ReturnType<typeof setTimeout> | undefined
  let previewController: AbortController | undefined
  let uploadController: AbortController | undefined
  let showInfo = $state(false)
  let status = $state('')
  let failed = $state(false)
  let dragOver = $state(false)
  let statusTimer: ReturnType<typeof setTimeout> | undefined
  let visible = $state(true)

  onMount(() => {
    const update = () => visible = !document.hidden
    update()
    document.addEventListener('visibilitychange', update)
    return () => document.removeEventListener('visibilitychange', update)
  })

  // Only the visible camera fetches frames; drafts and in-flight audio stay mounted.
  const previewEnabled = $derived(active && visible && draft.previewEnabled && camera.capabilities?.snapshot !== false)
  $effect(() => {
    const enabled = previewEnabled
    untrack(() => { if (enabled) { preview = true; void snapshot() } })
    return stopPreview
  })

  function swipe(node: HTMLElement) {
    let start: { x: number; y: number; id: number } | undefined
    const down = (event: PointerEvent) => {
      if (event.pointerType !== 'touch' || !event.isPrimary || !matchMedia('(max-width: 1023px)').matches
        || (event.target as HTMLElement).closest('button, a, input, select, textarea, summary')) return
      start = { x: event.clientX, y: event.clientY, id: event.pointerId }
      node.setPointerCapture(event.pointerId)
    }
    const up = (event: PointerEvent) => {
      if (!start || start.id !== event.pointerId) return
      const direction = cameraSwipe(event.clientX - start.x, event.clientY - start.y)
      start = undefined
      if (direction) onMove(direction)
    }
    const cancel = () => start = undefined
    node.addEventListener('pointerdown', down)
    node.addEventListener('pointerup', up)
    node.addEventListener('pointercancel', cancel)
    return { destroy() {
      node.removeEventListener('pointerdown', down)
      node.removeEventListener('pointerup', up)
      node.removeEventListener('pointercancel', cancel)
    } }
  }

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
    const slowTimer = setTimeout(() => { if (!controller.signal.aborted) previewSlow = true }, 750)
    let nextSrc = ''
    try {
      const response = await apiClient.snapshot(camera.name, undefined, undefined, controller.signal)
      const blob = await response.blob()
      if (!preview || controller.signal.aborted) return
      nextSrc = URL.createObjectURL(blob)
      const frame = new Image()
      frame.src = nextSrc
      await frame.decode()
      if (!preview || controller.signal.aborted) return
      if (previewSrc) URL.revokeObjectURL(previewSrc)
      previewSrc = nextSrc
      nextSrc = ''
      previewTime = new Date().toLocaleTimeString()
      previewError = ''
    } catch (cause) {
      if (!controller.signal.aborted) previewError = cause instanceof Error ? cause.message : String(cause)
    } finally {
      clearTimeout(slowTimer)
      if (nextSrc) URL.revokeObjectURL(nextSrc)
      if (!controller.signal.aborted) {
        previewLoading = false
        previewSlow = false
        // Schedule after completion so slow cameras never overlap requests.
        if (preview) previewTimer = setTimeout(snapshot, 2000)
      }
    }
  }

  function stopPreview() {
    preview = false
    clearTimeout(previewTimer)
    previewController?.abort()
    previewLoading = false
    previewSlow = false
  }

  function togglePreview() {
    draft.previewEnabled = !draft.previewEnabled
  }

  async function beep() {
    if (draft.busy) return
    clearTimeout(statusTimer)
    status = ''
    draft.busy = true
    draft.pendingAction = 'Test speaker'
    draft.waveformPreset = null
    try {
      await apiClient.beep({ camera: camera.name })
      draft.lastResult = { label: 'Test speaker' }
      feedback('Test tone sent')
    }
    catch (cause) { feedback(cause instanceof Error ? cause.message : String(cause), true) }
    finally { draft.busy = false; draft.pendingAction = ''; void monitor.refresh() }
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
    draft.pendingAction = 'Upload'
    draft.waveformPreset = null
    clearTimeout(statusTimer)
    failed = false
    status = 'Uploading your audio…'
    uploadController = new AbortController()
    try {
      const result = await uploadAudioToCamera(camera.name, file, progress => {
        status = `${progress.step} (${Math.round(progress.percent)}%)`
      }, uploadController.signal)
      draft.lastResult = { ...result, label: `File · ${file.name}` }
      feedback(`Audio sent: ${file.name}`)
    } catch (cause) {
      if (!uploadController.signal.aborted) {
        if (cause instanceof DOMException && cause.name === 'AbortError') feedback('Audio canceled')
        else feedback(cause instanceof Error ? cause.message : String(cause), true)
      }
    } finally { draft.busy = false; draft.pendingAction = ''; void monitor.refresh() }
  }

  onDestroy(() => {
    stopPreview()
    if (previewSrc) URL.revokeObjectURL(previewSrc)
    uploadController?.abort()
    clearTimeout(statusTimer)
  })
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<article aria-label={camera.name} ondragover={drag} ondragleave={() => dragOver = false} ondrop={drop}
  class="flex min-w-0 flex-col gap-3 rounded-xl border bg-card p-3 shadow-sm md:p-4 {dragOver ? 'border-dashed border-primary bg-primary/5' : ''}">
  <div use:swipe class="camera-swipe flex min-w-0 items-start justify-between gap-3">
    <div class="min-w-0">
      <h3 class="break-words text-base font-semibold leading-snug">{camera.name}</h3>
      <p class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
        <span class="flex items-center gap-1.5"><span class="h-1.5 w-1.5 rounded-full {camera.online ? 'bg-green-500' : 'bg-muted-foreground'}"></span>{camera.online ? 'Online' : 'Offline'}</span>
        <span>{camera.type}</span>
        {#if camera.airplay_enabled}<span class="flex items-center gap-1" title={camera.airplay_name}><Airplay class="h-3 w-3" /> AirPlay</span>{/if}
      </p>
    </div>
  </div>

  {#snippet previewPanel()}
    <div use:swipe class="camera-swipe flex flex-col gap-1.5" aria-label={`${camera.name} preview area`}>
      <div class="flex items-center justify-between gap-2"><span class="text-xs text-muted-foreground">Camera preview</span>
        {#if camera.capabilities?.snapshot !== false}<Button size="sm" variant="ghost" class="gap-1.5" onclick={togglePreview} aria-pressed={draft.previewEnabled} aria-label={`${draft.previewEnabled ? 'Hide' : 'Show'} ${camera.name} preview`}>
          {#if draft.previewEnabled}<VideoOff class="h-4 w-4" /> Hide{:else}<Video class="h-4 w-4" /> Show{/if}
        </Button>{/if}
      </div>
      {#if camera.capabilities?.snapshot === false}<p class="text-xs text-muted-foreground">Preview is unavailable for this camera connection.</p>
      {:else if preview}
        {#if previewSrc}<img src={previewSrc} alt={`${camera.name} preview`} draggable="false" class:opacity-60={!!previewError || previewSlow} class="camera-preview-image aspect-video w-full rounded-lg object-contain bg-muted transition-opacity" />
        {:else}<div class="camera-preview-image flex aspect-video items-center justify-center rounded-lg bg-muted text-sm text-muted-foreground">{previewLoading ? 'Loading preview…' : 'Preview unavailable'}</div>{/if}
        <p class="break-words text-xs {previewError || previewSlow ? 'text-warning' : 'text-muted-foreground'}">{previewError ? previewSrc ? `Last frame ${previewTime} · Retrying preview: ${previewError}` : `Preview unavailable: ${previewError}` : previewSlow && previewSrc ? `Last frame ${previewTime} · Refreshing preview…` : `Updated ${previewTime || '…'}`}</p>
      {/if}
    </div>
  {/snippet}
  {#snippet toolsPanel()}
    <div class="flex flex-wrap gap-2 border-t pt-2" role="group" aria-label="Camera tools">
      <Button size="sm" variant="ghost" class="gap-1.5" onclick={() => showInfo = true}><Info class="h-4 w-4" /> Device info</Button>
      <Button size="sm" variant="ghost" class="gap-1.5" onclick={beep} disabled={draft.busy || camera.capabilities?.speak === false}><Bell class="h-4 w-4" /> Test speaker</Button>
      <Button size="sm" variant="ghost" class="gap-1.5" onclick={() => onOpenSettings?.(camera.name)}><Settings class="h-4 w-4" /> Settings</Button>
    </div>
    {#if camera.note}<p class="mt-2 break-words text-xs text-muted-foreground">{camera.note}</p>{/if}
    {#if status}<p role={failed ? 'alert' : 'status'} class="mt-2 break-words text-xs {failed ? 'text-destructive' : 'text-primary'}">{status}</p>{/if}
  {/snippet}
  <AudioComposer {camera} {voices} {presets} bind:draft {monitor} {active} preview={previewPanel} tools={toolsPanel} />
</article>

<CameraInfoModal cameraName={camera.name} cameraType={camera.type} show={showInfo} onClose={() => showInfo = false} />
