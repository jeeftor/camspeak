<script lang="ts">
  import { onDestroy } from 'svelte'
  import { Eye, Loader2, Play, Radio, Send, Square, Upload } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import VoiceSelect from '$lib/components/VoiceSelect.svelte'
  import GainSlider from '$lib/components/GainSlider.svelte'
  import CopyButton from '$lib/components/CopyButton.svelte'
  import Markdown from '$lib/components/Markdown.svelte'
  import PresetSelect from './PresetSelect.svelte'
  import PlaybackStrip from './PlaybackStrip.svelte'
  import AudioPlayer from './AudioPlayer.svelte'
  import { apiClient } from '$lib/api'
  import { saveCameraGain, uploadAudioToCamera } from '$lib/audio-actions'
  import { buildCurl } from '$lib/curl.svelte'
  import { formatTimingSummary } from '$lib/utils'
  import type { CameraSummary, Preset, SpeakResponse } from '$lib/types'
  import type { AudioDraft } from '$lib/audio-draft'
  import type { PlaybackMonitor } from '$lib/playback.svelte'

  let { camera, voices = [], presets = [], draft = $bindable(), monitor }: {
    camera?: CameraSummary
    voices?: string[]
    presets?: Preset[]
    draft: AudioDraft
    monitor: PlaybackMonitor
  } = $props()

  let status = $state('')
  let failed = $state(false)
  let stopping = $state(false)
  let statusTimer: ReturnType<typeof setTimeout> | undefined
  let uploadController: AbortController | undefined
  const broadcast = $derived(!camera)
  const selected = $derived(presets.find(p => JSON.stringify([p.category, p.name]) === draft.preset))
  const stream = $derived(draft.urlMode === 'stream' || (draft.urlMode === 'auto' && looksLikeStream(draft.url)))
  const canStream = $derived(camera?.capabilities?.live_stream !== false)
  const valid = $derived(camera?.capabilities?.speak !== false && (draft.mode === 'speak' ? !!draft.text.trim()
    : draft.mode === 'preset' ? !!selected && (!selected.url || canStream)
    : draft.mode === 'url' ? /^https?:\/\//i.test(draft.url) && (!stream || canStream) : !!camera))

  function looksLikeStream(url: string) {
    return /\.(pls|m3u8?)(?:[?#]|$)|liveatc\.net|\/play\/|shoutcast|icecast/i.test(url)
  }

  function feedback(message: string, error = false) {
    clearTimeout(statusTimer)
    status = message
    failed = error
    if (!error) statusTimer = setTimeout(() => { status = '' }, 5000)
  }

  function request() {
    const shared = { gain: draft.gain }
    if (draft.mode === 'preset') return { ...shared, preset: selected?.name ?? '', category: selected?.category ?? '', loop: selected?.url ? 0 : draft.loop }
    if (draft.mode === 'url') return { ...shared, url: draft.url }
    if (draft.mode === 'describe') return { ...shared, prompt: draft.prompt }
    return { ...shared, text: draft.text, voice: draft.voice }
  }

  const endpoint = $derived(broadcast ? '/api/broadcast' : draft.mode === 'speak' ? '/api/speak'
    : draft.mode === 'preset' ? '/api/play' : draft.mode === 'describe' ? '/api/describe'
    : stream ? '/api/play-stream' : '/api/play-url')

  async function submit() {
    if (draft.busy || draft.gainSaving || !valid) return
    draft.busy = true
    clearTimeout(statusTimer)
    status = ''
    failed = false
    try {
      let result
      if (broadcast) {
        result = await apiClient.broadcast(draft.mode === 'preset'
          ? { preset: selected!.name, category: selected!.category, gain: draft.gain }
          : { text: draft.text, voice: draft.voice, gain: draft.gain })
        if (result.errors?.length) {
          feedback(`Broadcast reached ${result.succeeded?.length ?? 0} cameras. ${result.errors.join('; ')}`, true)
          return
        }
      } else if (camera) {
        // Use the shared runtime gain controller so later slider changes apply.
        const target = { camera: camera.name }
        if (draft.mode === 'speak') result = await apiClient.speak({ ...target, text: draft.text, voice: draft.voice })
        if (draft.mode === 'preset') result = await apiClient.play({ ...target, preset: selected!.name, category: selected!.category, loop: selected!.url ? 0 : draft.loop })
        if (draft.mode === 'url') result = await (stream ? apiClient.playStream : apiClient.playURL)({ ...target, url: draft.url })
        if (draft.mode === 'describe') {
          draft.description = ''
          draft.image = ''
          const described = await apiClient.describe({ ...target, prompt: draft.prompt })
          draft.description = described.description ?? ''
          // This is the exact frame used for inference, not a second snapshot.
          draft.image = described.image ?? ''
          result = described
        }
      }
      const response = result as SpeakResponse | undefined
      const timing = response ? formatTimingSummary(response.timings, response.total_ms, response.ttfs_ms) : ''
      feedback(`${broadcast ? 'Broadcast completed' : 'Audio sent'}${timing ? ` (${timing})` : ''}`)
    } catch (cause) {
      feedback(cause instanceof Error ? cause.message : String(cause), true)
    } finally {
      draft.busy = false
      void monitor.refresh()
    }
  }

  async function upload(file?: File) {
    if (!file || !camera || draft.busy || camera.capabilities?.speak === false) return
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
    } finally {
      draft.busy = false
      void monitor.refresh()
    }
  }

  async function saveGain() {
    if (!camera) return
    try { await saveCameraGain(camera.name, draft) }
    catch (cause) { feedback(`Volume could not be saved: ${cause instanceof Error ? cause.message : String(cause)}`, true) }
  }

  async function stopBroadcast() {
    stopping = true
    try { await apiClient.stopAll(); await monitor.refresh(); feedback('All cameras stopped') }
    catch (cause) { feedback(cause instanceof Error ? cause.message : String(cause), true) }
    finally { stopping = false }
  }

  async function replayDescription() {
    if (!camera || draft.busy || !draft.description) return
    draft.busy = true
    clearTimeout(statusTimer)
    status = ''
    try {
      await apiClient.speak({ camera: camera.name, text: draft.description, voice: draft.voice })
      feedback('Description sent')
    } catch (cause) { feedback(cause instanceof Error ? cause.message : String(cause), true) }
    finally { draft.busy = false; void monitor.refresh() }
  }

  onDestroy(() => {
    clearTimeout(statusTimer)
    uploadController?.abort()
  })
</script>

<div class="flex min-w-0 flex-col gap-5">
  {#if camera}
    <PlaybackStrip cameraName={camera.name} playback={monitor.state.cameras[camera.name]} preparing={draft.busy} level={monitor.state.levels[camera.name]} onRefresh={monitor.refresh} />
    {#if camera.capabilities?.speak === false}<p class="text-sm text-muted-foreground">Audio playback is unavailable for this camera connection. Check its settings in Config.</p>{/if}
  {:else}
    <p class="text-sm text-muted-foreground">Send the same message or preset to all enabled cameras.</p>
    {#if draft.busy || Object.values(monitor.state.cameras).some(camera => camera.state !== 'idle')}
      <Button variant="destructive" size="sm" onclick={stopBroadcast} disabled={stopping}><Square class="h-4 w-4" /> Stop all cameras</Button>
    {/if}
  {/if}

  <div class="flex flex-wrap gap-1 rounded-lg bg-muted p-1" role="group" aria-label="Audio source">
    {#each [{ key: 'speak', label: 'Speak' }, { key: 'preset', label: 'Preset' }, ...(!broadcast ? [{ key: 'url', label: 'URL' }, { key: 'describe', label: 'Describe' }] : [])] as mode}
      <Button size="sm" variant={draft.mode === mode.key ? 'default' : 'ghost'}
        aria-pressed={draft.mode === mode.key} disabled={draft.busy || (mode.key === 'describe' && camera?.capabilities?.snapshot === false)}
        onclick={() => { draft.mode = mode.key as AudioDraft['mode']; status = '' }}>{mode.label}</Button>
    {/each}
  </div>

  <form class="flex min-w-0 flex-col gap-4" onsubmit={event => { event.preventDefault(); void submit() }}>
    {#if draft.mode === 'speak'}
      <label class="flex flex-col gap-1.5 text-sm">
        Message
        <textarea bind:value={draft.text} rows="4" placeholder="What would you like to say?" disabled={draft.busy}
          onkeydown={event => { if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') { event.preventDefault(); void submit() } }}
          class="w-full resize-y rounded-md border border-input bg-transparent px-3 py-2 disabled:opacity-50"></textarea>
      </label>
      <label class="flex flex-col gap-1.5 text-sm">Voice <VoiceSelect bind:value={draft.voice} {voices} disabled={draft.busy} /></label>
      <p class="text-xs text-muted-foreground">Ctrl+Enter or ⌘+Enter sends your message.</p>
    {:else if draft.mode === 'preset'}
      <PresetSelect bind:value={draft.preset} {presets} disabled={draft.busy} />
      {#if selected && !selected.url}
        {#key draft.preset}
          <AudioPlayer peaksUrl={`/api/library/${encodeURIComponent(selected.category)}/${encodeURIComponent(selected.name)}/peaks`}
            audioUrl={`/api/library/${encodeURIComponent(selected.category)}/${encodeURIComponent(selected.name)}/preview`}
            duration={selected.duration} subtitle="Preview on this device" />
        {/key}
        {#if !broadcast}
          <label class="flex flex-col gap-1.5 text-sm">Repeat
            <select bind:value={draft.loop} disabled={draft.busy} class="rounded-md border border-input bg-background px-3 py-2">
              <option value={0}>Play once</option><option value={1}>Play twice</option><option value={2}>Play three times</option><option value={-1}>Repeat until stopped</option>
            </select>
          </label>
        {/if}
      {:else if !presets.length}<p class="text-sm text-muted-foreground">Add a preset in your Library to use it here.</p>{/if}
    {:else if draft.mode === 'url'}
      <label class="flex flex-col gap-1.5 text-sm">Audio URL
        <input type="url" bind:value={draft.url} placeholder="https://…" disabled={draft.busy} class="min-w-0 rounded-md border border-input bg-transparent px-3 py-2" />
      </label>
      <label class="flex flex-col gap-1.5 text-sm">Playback type
        <select bind:value={draft.urlMode} disabled={draft.busy} class="rounded-md border border-input bg-background px-3 py-2">
          <option value="auto">Detect automatically</option><option value="file">Audio file</option><option value="stream" disabled={!canStream}>Live stream or playlist</option>
        </select>
      </label>
      {#if !canStream}<p class="text-xs text-muted-foreground">This camera supports audio files. Continuous streams are unavailable for its connection.</p>{/if}
      {#if camera}
        <label class="flex cursor-pointer flex-col gap-2 rounded-lg border border-dashed p-4 text-sm">
          <span class="flex items-center gap-2"><Upload class="h-4 w-4" /> Or upload an audio file</span>
          <input type="file" accept="audio/*,.wav,.mp3,.m4a,.aac,.flac,.ogg,.opus" disabled={draft.busy || camera.capabilities?.speak === false} class="w-full min-w-0 text-xs" onchange={event => void upload(event.currentTarget.files?.[0])} />
        </label>
      {/if}
    {:else}
      <p class="text-sm text-muted-foreground">Capture this camera, describe the image, and speak the result through its speaker.</p>
      <label class="flex flex-col gap-1.5 text-sm">Vision prompt
        <textarea bind:value={draft.prompt} rows="3" placeholder="Use your configured default prompt" disabled={draft.busy} class="w-full resize-y rounded-md border border-input bg-transparent px-3 py-2"></textarea>
      </label>
      {#if draft.image}<img src={draft.image} alt="Camera frame used for this description" class="w-full rounded-lg" />{/if}
      {#if draft.description}
        <Markdown content={draft.description} />
        <div class="flex gap-2">
          <Button type="button" size="sm" variant="outline" disabled={draft.busy} onclick={replayDescription}><Play class="h-4 w-4" /> Speak again</Button>
          <Button type="button" size="sm" variant="ghost" disabled={draft.busy} onclick={() => { draft.description = ''; draft.image = '' }}>Clear result</Button>
        </div>
      {/if}
    {/if}

    <div class="flex flex-col gap-1.5">
      <span class="text-sm">{broadcast ? 'Broadcast volume' : 'Camera volume'}</span>
      <GainSlider bind:value={draft.gain} disabled={draft.busy} onchange={saveGain} />
    </div>
    <Button type="submit" disabled={draft.busy || draft.gainSaving || !valid} class="w-full">
      {#if draft.busy}<Loader2 class="h-4 w-4 animate-spin" /> Working…
      {:else if broadcast}<Radio class="h-4 w-4" /> Broadcast to all cameras
      {:else if draft.mode === 'describe'}<Eye class="h-4 w-4" /> Describe and speak
      {:else if draft.mode === 'speak'}<Send class="h-4 w-4" /> Speak
      {:else}<Play class="h-4 w-4" /> Play on camera{/if}
    </Button>
  </form>
  {#if status}<p role={failed ? 'alert' : 'status'} class="break-words text-sm {failed ? 'text-destructive' : 'text-primary'}">{status}</p>{/if}
  <details class="text-xs text-muted-foreground">
    <summary class="cursor-pointer">Automation tools</summary>
    <div class="mt-2"><CopyButton text={buildCurl('POST', endpoint, camera ? { ...request(), camera: camera.name } : request())} label="Copy curl command" /></div>
  </details>
</div>
