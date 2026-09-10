<script lang="ts">
  import { Loader2, Volume2, ArrowRight, Camera } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { apiClient } from '$lib/api'
  import type { AnnounceResponse, CameraSummary } from '$lib/types'
  import Markdown from '$lib/components/Markdown.svelte'
  import CameraSelect from '$lib/components/CameraSelect.svelte'
  import PromptEditor from '$lib/components/PromptEditor.svelte'
  import { formatTimings } from '$lib/utils'

  let { cameras = [] }: { cameras?: CameraSummary[] } = $props()
  let sourceCameras = $derived(cameras.filter(camera => camera.capabilities?.snapshot !== false))
  let targetCameras = $derived(cameras.filter(camera => camera.capabilities?.speak !== false))

  let sourceCamera = $state('')
  let targetCamera = $state('')
  let prompt = $state('')
  let gain = $state<number | undefined>(undefined)
  let busy = $state(false)
  let result = $state<AnnounceResponse | null>(null)
  let error = $state('')

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
        <CameraSelect bind:value={sourceCamera} cameras={sourceCameras} disabled={busy} class="min-w-[160px]" />
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
        {#if result.total_ms}
          <span class="text-xs font-normal text-muted-foreground ml-2">
            {formatTimings({ total_ms: result.total_ms, ...(result.ttfs_ms !== undefined ? { ttfs_ms: result.ttfs_ms } : {}) })}
          </span>
        {/if}
      </div>

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
