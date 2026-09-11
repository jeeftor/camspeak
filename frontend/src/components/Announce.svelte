<script lang="ts">
  import { onMount } from 'svelte'
  import { Loader2, Volume2, ArrowRight, Camera } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { apiClient } from '$lib/api'
  import type { AnnounceResponse, CameraSummary, Camera as CameraConfig, Settings } from '$lib/types'
  import Markdown from '$lib/components/Markdown.svelte'
  import CameraSelect from '$lib/components/CameraSelect.svelte'
  import PromptEditor from '$lib/components/PromptEditor.svelte'
  import TimingFlow from '$lib/components/TimingFlow.svelte'
  import { describeStages } from '$lib/audio-draft'

  let { cameras = [] }: { cameras?: CameraSummary[] } = $props()
  let configured = $state<CameraConfig[] | null>(null)
  let routing = $state<Partial<Settings>>({})
  let configError = $state('')
  let sourceCameras = $derived((configured ?? cameras).filter(camera => {
    const supported = cameras.find(c => c.name === camera.name)?.capabilities?.snapshot
    if (supported !== undefined) return supported
    return camera.type === 'hikvision' || camera.type === 'reolink' || !!routing.frigate_url
      || (!!routing.go2rtc_url && 'stream' in camera && !!(camera.stream || camera.vision_stream))
  }))
  let targetCameras = $derived(cameras.filter(camera => camera.capabilities?.speak !== false
    && configured?.find(c => c.name === camera.name)?.enabled !== false))
  onMount(() => {
    let mounted = true
    Promise.all([apiClient.getConfig(), apiClient.getSettings()]).then(([config, settings]) => {
      if (mounted) {
        routing = settings
        configured = Object.entries(config.cameras).map(([name, camera]) => ({ ...camera, name }))
      }
    }).catch(() => { if (mounted) configError = 'Could not load disabled capture sources. Showing available cameras.' })
    return () => { mounted = false }
  })

  let sourceCamera = $state('')
  let targetCamera = $state('')
  let prompt = $state('')
  let gain = $state<number | undefined>(undefined)
  let busy = $state(false)
  let result = $state<AnnounceResponse | null>(null)
  let error = $state('')
  let previewSrc = $state('')
  let previewError = $state('')
  let previewBusy = $state(false)
  let previewController: AbortController | undefined
  $effect(() => {
    const source = sourceCamera
    previewError = ''
    previewBusy = false
    return () => {
      previewController?.abort()
      if (previewSrc) URL.revokeObjectURL(previewSrc)
      previewSrc = ''
    }
  })
  async function capturePreview() {
    if (!sourceCamera || previewBusy) return
    const source = sourceCamera
    const controller = new AbortController()
    previewController = controller
    previewBusy = true
    previewError = ''
    let next = ''
    try {
      const response = await apiClient.snapshot(source, undefined, undefined, controller.signal)
      const blob = await response.blob()
      if (controller.signal.aborted) return
      next = URL.createObjectURL(blob)
      const frame = new Image()
      frame.src = next
      await frame.decode()
      if (controller.signal.aborted || source !== sourceCamera) return
      if (previewSrc) URL.revokeObjectURL(previewSrc)
      previewSrc = next
      next = ''
    } catch (e) {
      if (!controller.signal.aborted) previewError = e instanceof Error ? e.message : String(e)
    } finally {
      if (next) URL.revokeObjectURL(next)
      if (!controller.signal.aborted) previewBusy = false
    }
  }

  async function runAnnounce() {
    if (!sourceCamera || !targetCamera) return
    busy = true
    error = ''
    result = null
    try {
      result = await apiClient.announce({
        source_camera: sourceCamera,
        target_camera: targetCamera,
        prompt: prompt.trim() || undefined,
        gain,
      })
    } catch (e) {
      error = e instanceof Error ? e.message : String(e)
    } finally {
      busy = false
    }
  }

  // Auto-select different cameras if possible
  $effect(() => {
    if (!sourceCameras.some(camera => camera.name === sourceCamera)) sourceCamera = sourceCameras[0]?.name || ''
    if (!targetCameras.some(camera => camera.name === targetCamera))
      targetCamera = targetCameras.find(camera => camera.name !== sourceCamera)?.name || targetCameras[0]?.name || ''
  })
</script>

<div class="flex flex-col gap-5 max-w-4xl">
  <!-- Header -->
  <div>
    <h2 class="text-lg font-semibold text-primary mb-1">Cross-Camera Announce</h2>
    <p class="text-sm text-muted-foreground">
      Capture an image from one camera, describe it with the vision model, then play the description
      on another camera's speaker. Useful for doorbells or cameras without speakers.
    </p>
  </div>

  <!-- Controls -->
  <div class="rounded-lg border p-4 flex flex-col gap-3">
    <div class="flex flex-wrap items-end gap-3">
      <label class="flex flex-col gap-1 text-sm text-muted-foreground">
        Source (capture + vision)
        <CameraSelect bind:value={sourceCamera} cameras={sourceCameras} showDisabledLabel disabled={busy} class="w-full min-w-0" />
      </label>

      <ArrowRight class="h-5 w-5 text-muted-foreground self-center pb-2" />

      <label class="flex flex-col gap-1 text-sm text-muted-foreground">
        Target (speaker)
        <CameraSelect bind:value={targetCamera} cameras={targetCameras} disabled={busy} class="min-w-[160px]" />
      </label>

      <label class="flex flex-col gap-1 text-sm text-muted-foreground">
        Volume override (optional)
        <Input type="number" min="0" max="10" step="0.5" bind:value={gain} disabled={busy}
          placeholder="default" class="w-24" />
      </label>

      <Button onclick={runAnnounce} disabled={busy || !sourceCamera || !targetCamera} class="ml-auto">
        {#if busy}
          <Loader2 class="h-4 w-4 animate-spin" />
        {:else}
          <Volume2 class="h-4 w-4" />
        {/if}
        Announce
      </Button>
    </div>

    {#if configError}<p role="alert" class="text-xs text-warning">{configError}</p>{/if}
    <div class="flex flex-col gap-2">
      <Button variant="outline" class="self-start" onclick={capturePreview} disabled={!sourceCamera || previewBusy || busy}>
        <Camera class="h-4 w-4" />{previewBusy ? 'Capturing…' : previewSrc ? 'Refresh source preview' : 'Preview source'}
      </Button>
      <p class="text-xs text-muted-foreground">Preview is silent. Announce captures a fresh image before speaking.</p>
      {#if previewSrc}<img src={previewSrc} alt={`${sourceCamera} source preview`} class="w-full max-h-[300px] rounded-lg object-contain border" />{/if}
      {#if previewError}<p role="alert" class="break-words text-xs text-warning">{previewError}</p>{/if}
    </div>

    <label class="flex flex-col gap-1 text-sm text-muted-foreground">
      Prompt <span class="font-normal text-xs">(optional — overrides camera/global prompt)</span>
      <PromptEditor bind:value={prompt} disabled={busy}
        placeholder="Describe who is at the door in one sentence." />
    </label>
  </div>

  <!-- Error -->
  {#if error}
    <div role="alert" class="rounded-lg border border-destructive/50 bg-destructive/10 p-3 text-sm text-destructive">
      {error}
    </div>
  {/if}

  <!-- Result -->
  {#if result}
    <div class="rounded-lg border p-4 flex flex-col gap-3">
      <div class="flex items-center gap-2 text-sm font-semibold text-primary">
        <Camera class="h-4 w-4" />
        {result.source_camera} <ArrowRight class="h-4 w-4" /> {result.target_camera}
      </div>
      <TimingFlow label="Announce timing flow" steps={describeStages.map(step => ({ ...step,
        duration: result?.timings?.[step.timing] ?? (step.stage === 'snapshot' ? result?.timings?.snap_ms : step.stage === 'playing' ? result?.timings?.send_playback_ms : undefined),
      }))} firstAudioMs={result.ttfs_ms} totalMs={result.total_ms} />

      {#if result.image}
        <img src={result.image} alt="captured frame" class="rounded-lg max-h-[300px] object-contain border" />
      {/if}

      {#if result.description}
        <div class="text-sm">
          <Markdown content={result.description} />
        </div>
      {/if}
    </div>
  {:else if !busy && !error}
    <div class="rounded-lg border border-dashed p-8 text-center text-muted-foreground">
      <Volume2 class="h-8 w-8 mx-auto mb-2 opacity-50" />
      <p>Select source and target cameras, then click "Announce" to capture, describe, and play.</p>
    </div>
  {/if}
</div>
