<script lang="ts">
  import { onDestroy, type Snippet } from 'svelte'
  import { Eye, Loader2, Play, Radio, Send, Square, Upload } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import VoiceSelect from '$lib/components/VoiceSelect.svelte'
  import GainSlider from '$lib/components/GainSlider.svelte'
  import CopyButton from '$lib/components/CopyButton.svelte'
  import PresetSelect from './PresetSelect.svelte'
  import CameraOutput from './CameraOutput.svelte'
  import AudioPlayer from './AudioPlayer.svelte'
  import { apiClient } from '$lib/api'
  import { saveCameraGain, uploadAudioToCamera } from '$lib/audio-actions'
  import { buildCurl } from '$lib/curl.svelte'
  import { formatTimingSummary } from '$lib/utils'
  import type { CameraSummary, Preset, DescribeResponse } from '$lib/types'
  import { isValidRepeat, type AudioDraft } from '$lib/audio-draft'
  import type { PlaybackMonitor } from '$lib/playback.svelte'

  let { camera, voices = [], presets = [], draft = $bindable(), monitor, active = true, preview, tools }: {
    camera?: CameraSummary
    voices?: string[]
    presets?: Preset[]
    draft: AudioDraft
    monitor: PlaybackMonitor
    active?: boolean
    preview?: Snippet
    tools?: Snippet
  } = $props()

  let status = $state('')
  let failed = $state(false)
  let stopping = $state(false)
  let editingPrompt = $state(false)
  let statusTimer: ReturnType<typeof setTimeout> | undefined
  let uploadController: AbortController | undefined
  const broadcast = $derived(!camera)
  const selected = $derived(presets.find(p => JSON.stringify([p.category, p.name]) === draft.preset))
  const savedStream = $derived(presets.find(p => p.url && JSON.stringify([p.category, p.name]) === draft.streamPreset))
  const finitePresets = $derived(presets.filter(p => !p.url))
  const streamPresets = $derived(presets.filter(p => p.url))
  const stream = $derived(draft.urlMode === 'stream' || (draft.urlMode === 'auto' && looksLikeStream(draft.url)))
  const canStream = $derived(camera?.capabilities?.live_stream !== false)
  const valid = $derived(camera?.capabilities?.speak !== false && (draft.mode === 'speak' ? !!draft.text.trim()
    : draft.mode === 'preset' ? !!selected && (!selected.url || canStream) && (broadcast || !!selected.url || (isValidRepeat(draft.loop) && (draft.loop === 0 || canStream)))
    : draft.mode === 'stream' ? canStream && (draft.streamPreset ? !!savedStream : /^https?:\/\//i.test(draft.url))
    : draft.mode === 'url' ? /^https?:\/\//i.test(draft.url) && (!stream || canStream) : !!camera && camera.capabilities?.snapshot !== false))

  function looksLikeStream(url: string) {
    return /\.(pls|m3u8?)(?:[?#]|$)|liveatc\.net|\/play\/|shoutcast|icecast/i.test(url)
  }

  // Keep the nearby send action visible when a phone keyboard reduces the view.
  function keepActionVisible(form: HTMLFormElement) {
    let frame = 0
    const reveal = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => {
        if (!matchMedia('(max-width: 767px)').matches || !form.contains(document.activeElement)
          || !document.activeElement?.matches('textarea, input[type="url"]')) return
        const button = form.querySelector<HTMLButtonElement>('[data-primary-action]')
        button?.scrollIntoView({ block: 'nearest' })
        const viewport = window.visualViewport
        if (button && viewport) {
          const overflow = button.getBoundingClientRect().bottom - viewport.height - viewport.offsetTop + 8
          if (overflow > 0) window.scrollBy(0, overflow)
        }
      })
    }
    form.addEventListener('focusin', reveal)
    window.visualViewport?.addEventListener('resize', reveal)
    return { destroy() {
      cancelAnimationFrame(frame)
      form.removeEventListener('focusin', reveal)
      window.visualViewport?.removeEventListener('resize', reveal)
    } }
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
    if (draft.mode === 'stream') return savedStream
      ? { ...shared, preset: savedStream.name, category: savedStream.category, loop: 0 }
      : { ...shared, url: draft.url }
    if (draft.mode === 'url') return { ...shared, url: draft.url }
    if (draft.mode === 'describe') return { ...shared, prompt: draft.prompt }
    return { ...shared, text: draft.text, voice: draft.voice }
  }

  const endpoint = $derived(broadcast ? '/api/broadcast' : draft.mode === 'speak' ? '/api/speak'
    : draft.mode === 'preset' ? '/api/play' : draft.mode === 'describe' ? '/api/describe'
    : draft.mode === 'stream' ? savedStream ? '/api/play' : '/api/play-stream'
    : stream ? '/api/play-stream' : '/api/play-url')

  async function submit() {
    if (draft.busy || draft.gainSaving || !valid) return
    draft.busy = true
    clearTimeout(statusTimer)
    status = ''
    failed = false
    const label = draft.mode === 'describe' ? 'Describe' : draft.mode === 'speak' ? 'Speak'
      : draft.mode === 'preset' ? `Preset · ${selected?.name}` : draft.mode === 'stream' ? 'Stream' : 'Audio URL'
    draft.pendingAction = label
    draft.waveformPreset = draft.mode === 'preset' && selected && !selected.url ? selected : null
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
        if (draft.mode === 'stream') result = savedStream
          ? await apiClient.play({ ...target, preset: savedStream.name, category: savedStream.category, loop: 0 })
          : await apiClient.playStream({ ...target, url: draft.url })
        if (draft.mode === 'describe') {
          // This is the exact frame used for inference, not a second snapshot.
          result = await apiClient.describe({ ...target, prompt: draft.prompt })
        }
      }
      const response = result as DescribeResponse | undefined
      draft.lastResult = { ...response, label }
      const timing = response ? formatTimingSummary(response.timings, response.total_ms, response.ttfs_ms) : ''
      feedback(`${broadcast ? 'Broadcast completed' : 'Audio sent'}${timing ? ` (${timing})` : ''}`)
    } catch (cause) {
      feedback(cause instanceof Error ? cause.message : String(cause), true)
    } finally {
      draft.busy = false
      draft.pendingAction = ''
      void monitor.refresh()
    }
  }

  async function upload(file?: File) {
    if (!file || !camera || draft.busy || camera.capabilities?.speak === false) return
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
    } finally {
      draft.busy = false
      draft.pendingAction = ''
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
    const previous = draft.lastResult
    if (!camera || draft.busy || draft.gainSaving || !previous?.description) return
    draft.busy = true
    draft.pendingAction = 'Speak again'
    draft.waveformPreset = null
    clearTimeout(statusTimer)
    status = ''
    try {
      const result = await apiClient.speak({ camera: camera.name, text: previous.description, voice: draft.voice })
      draft.lastResult = { ...result, label: 'Speak again', description: previous.description, image: previous.image }
      feedback('Description sent')
    } catch (cause) { feedback(cause instanceof Error ? cause.message : String(cause), true) }
    finally { draft.busy = false; draft.pendingAction = ''; void monitor.refresh() }
  }

  function describe() {
    if (draft.busy || draft.gainSaving || camera?.capabilities?.snapshot === false) return
    draft.mode = 'describe'
    void submit()
  }

  onDestroy(() => {
    clearTimeout(statusTimer)
    uploadController?.abort()
  })
</script>

<div class="audio-composer {camera ? 'camera-workspace' : ''}">
  {#if camera && active}<div class="workspace-preview min-w-0">{@render preview?.()}</div>{/if}
  <div class="workspace-compose flex min-w-0 flex-col gap-3">
  {#if camera}
    {#if camera.capabilities?.speak === false}<p class="text-sm text-muted-foreground">Audio playback is unavailable for this camera connection. Check its settings in Config.</p>{/if}
  {:else}
    <p class="text-sm text-muted-foreground">Send the same message or preset to all enabled cameras.</p>
    {#if draft.busy || Object.values(monitor.state.cameras).some(camera => camera.state !== 'idle')}
      <Button variant="destructive" size="sm" onclick={stopBroadcast} disabled={stopping}><Square class="h-4 w-4" /> Stop all cameras</Button>
    {/if}
  {/if}

  <div class="grid gap-1 rounded-lg bg-muted p-1 {broadcast ? 'grid-cols-2' : 'grid-cols-3'}" role="group" aria-label="Audio source">
    {#each [...(!broadcast ? [{ key: 'describe', label: 'Describe' }] : []), { key: 'speak', label: 'Speak' }, { key: 'preset', label: 'Presets' }] as mode}
      <Button type="button" size="sm" variant={draft.mode === mode.key ? 'default' : 'ghost'}
        aria-pressed={draft.mode === mode.key} disabled={draft.busy || (mode.key === 'describe' && (draft.gainSaving || camera?.capabilities?.snapshot === false || camera?.capabilities?.speak === false))}
        onclick={() => { if (mode.key === 'describe') describe(); else { draft.mode = mode.key as AudioDraft['mode']; status = '' } }}>{mode.label}</Button>
    {/each}
  </div>

  <form use:keepActionVisible class="flex min-w-0 flex-col gap-3" onsubmit={event => { event.preventDefault(); void submit() }}>
    {#if draft.mode === 'speak'}
      <label class="flex flex-col gap-1.5 text-sm">
        Message
        <textarea bind:value={draft.text} rows="2" placeholder="What would you like to say?" disabled={draft.busy}
          onkeydown={event => { if ((event.ctrlKey || event.metaKey) && event.key === 'Enter') { event.preventDefault(); void submit() } }}
          class="w-full resize-y rounded-md border border-input bg-transparent px-3 py-2 disabled:opacity-50"></textarea>
      </label>
    {:else if draft.mode === 'preset'}
      <PresetSelect bind:value={draft.preset} presets={broadcast ? presets : finitePresets} disabled={draft.busy} />
      {#if !(broadcast ? presets : finitePresets).length}<p class="text-sm text-muted-foreground">Add an audio preset in your Library to use it here.</p>{/if}
    {:else if draft.mode === 'stream'}
      {#if streamPresets.length}
        <PresetSelect bind:value={draft.streamPreset} presets={streamPresets} disabled={draft.busy || !canStream}
          label="Saved stream" placeholder="Paste a stream URL instead" />
      {/if}
      {#if !draft.streamPreset}
        <label class="flex flex-col gap-1.5 text-sm">Stream URL
          <input type="url" bind:value={draft.url} placeholder="https://…" disabled={draft.busy || !canStream} class="min-w-0 rounded-md border border-input bg-transparent px-3 py-2" />
        </label>
      {/if}
      {#if !canStream}<p class="text-sm text-muted-foreground">Continuous streams are unavailable for this camera connection. You can still play finite presets and audio files.</p>{/if}
    {:else if draft.mode === 'url'}
      <label class="flex flex-col gap-1.5 text-sm">Audio file URL
        <input type="url" bind:value={draft.url} placeholder="https://…" disabled={draft.busy} class="min-w-0 rounded-md border border-input bg-transparent px-3 py-2" />
      </label>
    {:else}
      <p class="text-sm text-muted-foreground">Describe this camera's image and speak the result.</p>
      <details bind:open={editingPrompt} class="text-sm">
        <summary class="cursor-pointer text-muted-foreground">Vision prompt · optional</summary>
        <label class="mt-2 flex flex-col gap-1.5">Vision prompt
          <textarea bind:value={draft.prompt} rows="3" placeholder="Use your configured default prompt" disabled={draft.busy} class="w-full resize-y rounded-md border border-input bg-transparent px-3 py-2"></textarea>
        </label>
        <Button type="button" size="sm" variant="ghost" disabled={draft.busy} onclick={() => draft.prompt = camera?.vision_prompt ?? ''}>Reset camera prompt</Button>
      </details>
    {/if}

    <Button data-primary-action size="sm" type="submit" disabled={draft.busy || draft.gainSaving || !valid} class="w-full gap-2">
      {#if draft.busy}<Loader2 class="h-4 w-4 animate-spin" /> Working…
      {:else if broadcast}<Radio class="h-4 w-4" /> Broadcast to all cameras
      {:else if draft.mode === 'describe'}<Eye class="h-4 w-4" /> Describe and speak
      {:else if draft.mode === 'speak'}<Send class="h-4 w-4" /> Speak
      {:else if draft.mode === 'stream'}<Radio class="h-4 w-4" /> Start stream
      {:else}<Play class="h-4 w-4" /> Play on camera{/if}
    </Button>
    {#if status}<p role={failed ? 'alert' : 'status'} class="break-words text-sm {failed ? 'text-destructive' : 'text-primary'}">{status}</p>{/if}

    {#if draft.mode === 'speak' || draft.mode === 'describe'}
      <details class="text-sm">
        <summary class="cursor-pointer text-muted-foreground">Voice & options</summary>
        <div class="mt-2 flex flex-col gap-2">
          <label class="flex flex-col gap-1.5">Voice <VoiceSelect bind:value={draft.voice} {voices} disabled={draft.busy} /></label>
          {#if draft.mode === 'speak'}<p class="hidden text-xs text-muted-foreground md:block">Ctrl+Enter or ⌘+Enter sends your message.</p>{/if}
        </div>
      </details>
    {:else if draft.mode === 'preset' && selected && !selected.url}
      {#if !broadcast}
        <details class="text-sm">
          <summary class="cursor-pointer text-muted-foreground">Repeat: {draft.loop === -1 ? 'until stopped' : draft.loop === 0 ? 'play once' : `${draft.loop + 1} plays`}</summary>
          <label class="mt-2 flex flex-col gap-1.5">Additional repeats
            <input type="number" min="-1" step="1" bind:value={draft.loop} disabled={draft.busy || !canStream}
              class="min-w-0 rounded-md border border-input bg-background px-3 py-2" />
            <span class="text-xs text-muted-foreground">0 plays once; 4 plays five times; -1 repeats until stopped.</span>
          </label>
          {#if !canStream}<p class="text-xs text-muted-foreground">This camera connection supports playing once, not looping.</p>{/if}
        </details>
      {/if}
      <details class="text-sm">
        <summary class="cursor-pointer text-muted-foreground">Preview here · {selected.name}</summary>
        <div class="mt-2">
          {#if active}{#key draft.preset}
            <AudioPlayer peaksUrl={`/api/library/${encodeURIComponent(selected.category)}/${encodeURIComponent(selected.name)}/peaks`}
              audioUrl={`/api/library/${encodeURIComponent(selected.category)}/${encodeURIComponent(selected.name)}/preview`}
              duration={selected.duration} vuOrientation="vertical" subtitle="Preview on this device" />
          {/key}{/if}
        </div>
      </details>
    {:else if draft.mode === 'url'}
      <label class="flex flex-col gap-1.5 text-sm">Playback type
        <select bind:value={draft.urlMode} disabled={draft.busy} class="rounded-md border border-input bg-background px-3 py-2">
          <option value="auto">Detect automatically</option><option value="file">Audio file</option><option value="stream" disabled={!canStream}>Live stream or playlist</option>
        </select>
      </label>
      {#if camera}
        <label class="flex cursor-pointer flex-col gap-2 rounded-lg border border-dashed p-3 text-sm">
          <span class="flex items-center gap-2"><Upload class="h-4 w-4" /> Upload & play an audio file</span>
          <input type="file" accept="audio/*,.wav,.mp3,.m4a,.aac,.flac,.ogg,.opus" disabled={draft.busy || camera.capabilities?.speak === false} class="w-full min-w-0 text-xs" onchange={event => void upload(event.currentTarget.files?.[0])} />
        </label>
      {/if}
    {/if}

    {#if broadcast}<div>
      <GainSlider bind:value={draft.gain} disabled={draft.gainSaving} onchange={saveGain}
        aria-label={camera ? `${camera.name} volume` : 'Broadcast volume'} />
    </div>{/if}
  </form>

  {#if !broadcast}
    <details class="border-t pt-2 text-xs text-muted-foreground">
      <summary class="cursor-pointer">More audio · streams & files</summary>
      <div class="mt-2 flex flex-wrap gap-2">
      <Button type="button" size="sm" variant={draft.mode === 'stream' ? 'secondary' : 'ghost'} aria-pressed={draft.mode === 'stream'}
        disabled={draft.busy} onclick={() => { draft.mode = 'stream'; status = '' }}><Radio class="h-4 w-4" /> Streams</Button>
      <Button type="button" size="sm" variant={draft.mode === 'url' ? 'secondary' : 'ghost'} aria-pressed={draft.mode === 'url'}
        disabled={draft.busy} onclick={() => { draft.mode = 'url'; status = '' }}><Upload class="mr-1.5 h-4 w-4" /> Files</Button>
      <Button type="button" size="sm" variant="ghost" disabled={draft.busy || camera?.capabilities?.snapshot === false}
        onclick={() => { draft.mode = 'describe'; editingPrompt = true; status = '' }}>Edit vision prompt</Button>
      </div>
    </details>
  {/if}
  <details class="text-xs text-muted-foreground">
    <summary class="cursor-pointer">Automation tools</summary>
    <div class="mt-2"><CopyButton text={buildCurl('POST', endpoint, camera ? { ...request(), camera: camera.name } : request())} label="Copy curl command" /></div>
  </details>
  </div>
  {#if camera && active}
    <div class="workspace-output min-w-0"><CameraOutput {camera} bind:draft {monitor} onGain={saveGain} onReplay={replayDescription} /></div>
    <div class="workspace-tools min-w-0">{@render tools?.()}</div>
  {/if}
</div>
