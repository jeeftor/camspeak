<script>
  import { onDestroy } from 'svelte'
  import { Eye, Bell, Play, Pause, Loader2, FileAudio, X, MessageSquare, Square, Info, Airplay } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { Textarea } from '$lib/components/ui/textarea'
  import { Card } from '$lib/components/ui/card'
  import { Badge } from '$lib/components/ui/badge'
  import CopyButton from '$lib/components/CopyButton.svelte'
  import VoiceSelect from '$lib/components/VoiceSelect.svelte'
  import GainSlider from '$lib/components/GainSlider.svelte'
  import CameraInfoModal from './CameraInfoModal.svelte'
  import { buildCurl } from '$lib/curl.svelte'
  import { apiClient } from '$lib/api'
  import { Tooltip } from '$lib/components/ui/tooltip'
  import { formatTimingSummary, timingTooltipContent, isMobile, formatSeconds } from '$lib/utils'

  let { camera, voices = [], presets = [] } = $props()

  let text = $state('')
  let voice = $state('')
  let preset = $state('')
  let loopPreset = $state(false)
  let url = $state('')
  let gain = $state(camera.gain ?? 3.0)
  let busy = $state(false)
  let streaming = $state(false)
  let paused = $state(false)
  let playbackDetail = $state('')
  let status = $state('')
  let statusType = $state('ok')
  let snapshot = $state('')
  let description = $state('')
  let describeTiming = $state('')
  let describeTimingsRaw = $state(undefined)
  let describeTotalMs = $state(undefined)
  let describeTtfsMs = $state(undefined)
  let desktop = $state(!isMobile())
  // Pre-fill from saved camera default; user can override per-session
  const savedPrompt = camera.vision_prompt ?? ''
  let visionPrompt = $state(savedPrompt)
  let showPromptPopup = $state(false)
  let showInfoModal = $state(false)
  let isDragOver = $state(false)
  let statusTimeout

  onDestroy(() => {
    if (snapshot) URL.revokeObjectURL(snapshot)
    clearTimeout(statusTimeout)
  })

  // Poll the server for playback state so the UI stays in sync even when
  // playback is started/stopped from another client (MQTT, REST, etc.).
  // Skipped while busy to avoid racing with in-flight requests.
  $effect(() => {
    const interval = setInterval(async () => {
      if (busy) return
      try {
        const states = await apiClient.getPlayback()
        const ps = states[camera.name]
        if (ps) {
          streaming = ps.state !== 'idle'
          paused = ps.state === 'paused'
          playbackDetail = ps.detail ?? ''
        } else {
          streaming = false
          paused = false
          playbackDetail = ''
        }
      } catch { /* server unreachable — keep last known state */ }
    }, 3000)
    return () => clearInterval(interval)
  })

  function setStatus(msg, type = 'ok') {
    status = msg
    statusType = type
    if (!busy) {
      clearTimeout(statusTimeout)
      statusTimeout = setTimeout(() => (status = ''), 4000)
    }
  }

  async function speak() {
    if (!text) return
    busy = true; status = ''
    try {
      const data = await apiClient.speak({ camera: camera.name, text, voice, gain })
      const timing = formatTimingSummary(data.timings, data.total_ms, data.ttfs_ms)
      setStatus(timing ? `✓ sent (${timing})` : '✓ sent')
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    } finally {
      busy = false
    }
  }

  async function play() {
    if (!preset) {
      setStatus('select a preset first', 'warn')
      return
    }
    busy = true; status = ''
    try {
      const data = await apiClient.play({ camera: camera.name, preset, gain, loop: loopPreset })
      if (loopPreset) {
        setStatus('✓ looping')
      } else {
        const timing = formatTimingSummary(data.timings, data.total_ms, data.ttfs_ms)
        setStatus(timing ? `✓ playing (${timing})` : '✓ playing')
      }
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    } finally {
      busy = false
    }
  }

  let gainSaveTimeout

  async function saveGain() {
    try {
      await apiClient.setVolume(camera.name, gain)
    } catch (e) {
      setStatus('✗ gain save failed: ' + e.message, 'err')
    }
  }

  function onGainChange() {
    clearTimeout(gainSaveTimeout)
    gainSaveTimeout = setTimeout(saveGain, 500)
  }

  function looksLikeStream(u) {
    if (!u) return false
    const s = u.toLowerCase()
    return s.endsWith('.pls') || s.endsWith('.m3u') || s.endsWith('.m3u8') ||
           s.includes('liveatc.net') || s.includes('/play/') ||
           s.includes('shoutcast') || s.includes('icecast')
  }

  async function playUrl() {
    if (!url) return
    busy = true; status = ''
    try {
      await apiClient.playURL({ camera: camera.name, url, gain })
      setStatus('✓ playing')
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    } finally {
      busy = false
    }
  }

  async function playStream() {
    if (!url) return
    streaming = true; paused = false; status = ''
    try {
      await apiClient.playStream({ camera: camera.name, url, gain })
      setStatus('✓ streaming')
    } catch (e) {
      streaming = false
      setStatus('✗ ' + e.message, 'err')
    }
  }

  async function playFromUrl() {
    if (!url) return
    if (looksLikeStream(url)) {
      await playStream()
    } else {
      await playUrl()
    }
  }

  async function pauseStream() {
    busy = true; status = ''
    try {
      await apiClient.pause(camera.name)
      paused = true
      setStatus('⏸ paused')
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    } finally {
      busy = false
    }
  }

  async function resumeStream() {
    busy = true; status = ''
    try {
      await apiClient.resume(camera.name)
      paused = false
      setStatus('✓ streaming')
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    } finally {
      busy = false
    }
  }

  async function stopStream() {
    busy = true; status = ''
    try {
      await apiClient.stop(camera.name)
      streaming = false
      paused = false
      setStatus('⏹ stopped')
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    } finally {
      busy = false
    }
  }

  async function beep() {
    busy = true; status = ''
    try {
      await apiClient.beep({ camera: camera.name })
      setStatus('✓ beep')
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    } finally {
      busy = false
    }
  }

  async function stop() {
    try {
      await apiClient.stop(camera.name)
      setStatus('⏹ stopped')
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    }
  }

  async function describe() {
    busy = true; status = ''
    if (snapshot) URL.revokeObjectURL(snapshot)
    snapshot = ''; description = ''; describeTiming = ''
    describeTimingsRaw = undefined; describeTotalMs = undefined; describeTtfsMs = undefined
    try {
      setStatus('Capturing screenshot…')
      const snapRes = await apiClient.snapshot(camera.name)
      const snapBlob = await snapRes.blob()
      snapshot = URL.createObjectURL(snapBlob)

      setStatus('Describing → speaking…')
      const body = { camera: camera.name, gain }
      if (visionPrompt) body.prompt = visionPrompt
      const data = await apiClient.describe(body)
      description = data.description || ''
      describeTimingsRaw = data.timings
      describeTotalMs = data.total_ms
      describeTtfsMs = data.ttfs_ms
      describeTiming = formatTimingSummary(data.timings, data.total_ms, data.ttfs_ms)
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    } finally {
      busy = false
      if (description) {
        setStatus(describeTiming ? `✓ described (${describeTiming})` : '✓ described & spoken')
      }
    }
  }

  async function replayDescription() {
    if (!description) return
    busy = true; status = ''
    try {
      await apiClient.speak({ camera: camera.name, text: description, voice, gain })
      setStatus('✓ replaying')
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    } finally {
      busy = false
    }
  }

  function clearSnapshot() {
    if (snapshot) URL.revokeObjectURL(snapshot)
    snapshot = ''
    description = ''
    describeTiming = ''
    describeTimingsRaw = undefined
    describeTotalMs = undefined
    describeTtfsMs = undefined
    status = ''
  }

  // WAV / audio file drag-and-drop
  function onDragOver(e) {
    const hasAudio = [...(e.dataTransfer?.items ?? [])].some(
      it => it.kind === 'file' && (it.type.startsWith('audio/') || it.type === 'application/octet-stream')
    )
    if (hasAudio) {
      e.preventDefault()
      isDragOver = true
    }
  }

  function onDragLeave() {
    isDragOver = false
  }

  async function onDrop(e) {
    e.preventDefault()
    isDragOver = false
    const file = e.dataTransfer?.files?.[0]
    if (!file) return
    if (!file.name.match(/\.(wav|mp3|m4a|aac|flac|ogg|opus)$/i) && !file.type.startsWith('audio/')) {
      setStatus('Drop an audio file', 'warn')
      return
    }

    busy = true
    setStatus('Uploading…')
    try {
      const dropName = `drop_${Date.now()}`
      const fd = new FormData()
      fd.append('name', dropName)
      fd.append('category', 'drops')
      fd.append('file', file)

      await apiClient.uploadPreset(fd)

      setStatus('Playing…')
      await apiClient.play({ camera: camera.name, preset: dropName, category: 'drops', gain })
      setStatus(`✓ playing ${file.name}`)
    } catch (err) {
      setStatus('✗ ' + err.message, 'err')
    } finally {
      busy = false
    }
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  ondragover={onDragOver}
  ondragleave={onDragLeave}
  ondrop={onDrop}
>
  <Card class="flex flex-col gap-2.5 p-4 transition-colors
    {!camera.online ? 'opacity-50' : ''}
    {isDragOver ? 'border-primary border-dashed bg-primary/5 scale-[1.01]' : 'hover:border-primary/50'}">

    <!-- Camera header -->
    <div class="flex items-center justify-between gap-2">
      <div class="flex items-center gap-2 min-w-0">
        {#if camera.airplay_enabled}
          <Tooltip content={'AirPlay active' + (camera.airplay_name ? ` — "${camera.airplay_name}"` : '') + (camera.online ? '' : ' (camera offline)')}>
            <Airplay class="h-4 w-4 flex-shrink-0 {camera.online ? 'text-violet-500' : 'text-violet-500/40'}" />
          </Tooltip>
        {:else}
          <span class="h-2.5 w-2.5 rounded-full flex-shrink-0 {camera.online
            ? 'bg-green-500 shadow-[0_0_6px_rgba(34,197,94,0.5)]'
            : 'bg-muted-foreground/40'}"></span>
        {/if}
        <span class="font-semibold">{camera.name}</span>
        <Badge variant="secondary" class="text-xs flex-shrink-0">{camera.type}</Badge>
        {#if camera.note}
          <Badge variant="outline" class="text-xs flex-shrink-0 text-amber-500 border-amber-500/30" title={camera.note}>
            ⚠ Limited
          </Badge>
        {/if}
      </div>
      <div class="flex gap-1 flex-shrink-0">
        <Button
          variant="outline" size="icon"
          onclick={() => showInfoModal = true}
          title="Camera settings — device info, streams, codecs" aria-label="Camera info"
          class="h-8 w-8"
        >
          <Info class="h-4 w-4" />
        </Button>
        <Button
          variant="outline" size="icon"
          onclick={describe} disabled={busy}
          title="Describe — snapshot → vision → TTS → speak" aria-label="Describe"
          class="h-8 w-8"
        >
          {#if busy && status.toLowerCase().includes('describ')}
            <Loader2 class="h-4 w-4 animate-spin" />
          {:else}
            <Eye class="h-4 w-4" />
          {/if}
        </Button>
        <Button
          variant={visionPrompt ? 'default' : 'outline'} size="icon"
          onclick={() => showPromptPopup = true}
          title="Edit vision prompt" aria-label="Vision prompt"
          class="h-8 w-8"
        >
          <MessageSquare class="h-4 w-4" />
        </Button>
        <Button
          variant="outline" size="icon"
          onclick={beep} disabled={busy}
          title="Test beep (800 Hz)" aria-label="Test beep"
          class="h-8 w-8"
        >
          <Bell class="h-4 w-4" />
        </Button>
        <Button
          variant="outline" size="icon"
          onclick={stop}
          title="Stop audio on this camera" aria-label="Stop"
          class="h-8 w-8 hover:bg-destructive/10 hover:text-destructive hover:border-destructive/50"
        >
          <Square class="h-4 w-4 fill-current" />
        </Button>
      </div>
    </div>

    <!-- TTS row -->
    <div class="flex flex-col gap-1.5">
      <Textarea
        bind:value={text}
        placeholder="Say something..."
        onkeydown={e => e.key === 'Enter' && !e.shiftKey && (e.preventDefault(), speak())}
        disabled={busy}
        rows="2"
        class="flex-1 text-sm min-w-0 resize-y"
      />
      <div class="flex gap-1.5">
        <VoiceSelect bind:value={voice} {voices} {busy} class="w-[100px] flex-shrink-0" />
        <Button size="sm" onclick={speak} disabled={busy || !text} aria-label="Speak" title="Send TTS to camera" class="flex-shrink-0">
          <Play class="h-4 w-4" />
        </Button>
        <CopyButton
          text={buildCurl('POST', '/api/speak', { camera: camera.name, text, voice: voice || undefined, gain })}
          disabled={!text} label="Copy curl — speak endpoint"
          preview={!!text} previewType="curl"
          class="flex-shrink-0"
        />
      </div>
    </div>

    <!-- Volume row -->
    <GainSlider bind:value={gain} {busy} onchange={onGainChange} class="px-1" />

    <!-- Preset row -->
    {#if presets.length > 0}
      <div class="flex gap-1.5 items-center">
        <select bind:value={preset} disabled={busy}
          class="flex-1 min-w-0 rounded-md border border-input bg-transparent px-3 py-1 text-sm disabled:opacity-50">
          <option value="">— play preset —</option>
          {#each presets as p}
            <option value={p.name}>{p.category}/{p.name} ({formatSeconds(p.duration)})</option>
          {/each}
        </select>
        <label class="flex items-center gap-1 text-xs text-muted-foreground whitespace-nowrap flex-shrink-0" title="Loop the preset infinitely (pausable)">
          <input type="checkbox" bind:checked={loopPreset} disabled={busy} class="h-3.5 w-3.5" />
          loop
        </label>
        {#if streaming && playbackDetail?.endsWith('(loop)')}
          {#if paused}
            <Button size="sm" onclick={resumeStream} disabled={busy} aria-label="Resume" title="Resume looped preset" class="flex-shrink-0">
              <Play class="h-4 w-4" />
            </Button>
          {:else}
            <Button size="sm" onclick={pauseStream} disabled={busy} aria-label="Pause" title="Pause looped preset" class="flex-shrink-0">
              <Pause class="h-4 w-4" />
            </Button>
          {/if}
          <Button size="sm" onclick={stopStream} disabled={busy} aria-label="Stop" title="Stop looped preset" class="flex-shrink-0">
            <Square class="h-4 w-4" />
          </Button>
        {:else}
          <Button size="sm" onclick={play} disabled={busy || !preset} aria-label="Play preset" title="Play preset on camera" class="flex-shrink-0">
            <Play class="h-4 w-4" />
          </Button>
        {/if}
        <CopyButton
          text={buildCurl('POST', '/api/play', { camera: camera.name, preset, category: presets.find(x => x.name === preset)?.category, gain, loop: loopPreset })}
          disabled={!preset} label="Copy curl — play preset endpoint"
          preview={!!preset} previewType="curl"
          class="flex-shrink-0"
        />
      </div>
    {/if}

    <!-- URL row -->
    <div class="flex gap-1.5">
      <Input
        bind:value={url}
        placeholder="Play from URL or stream..."
        onkeydown={e => e.key === 'Enter' && (streaming ? stopStream() : playFromUrl())}
        disabled={busy}
        class="flex-1 text-sm min-w-0"
      />
      {#if streaming}
        {#if paused}
          <Button size="sm" onclick={resumeStream} disabled={busy} aria-label="Resume stream" title="Resume paused stream" class="flex-shrink-0">
            <Play class="h-4 w-4" />
          </Button>
        {:else}
          <Button size="sm" onclick={pauseStream} disabled={busy} aria-label="Pause stream" title="Pause live stream" class="flex-shrink-0">
            <Pause class="h-4 w-4" />
          </Button>
        {/if}
        <Button size="sm" onclick={stopStream} disabled={busy} aria-label="Stop stream" title="Stop live stream" class="flex-shrink-0">
          <Square class="h-4 w-4" />
        </Button>
      {:else}
        <Button size="sm" onclick={playFromUrl} disabled={busy || !url} aria-label="Play from URL" title={looksLikeStream(url) ? 'Stream live audio from URL' : 'Download and play audio from URL'} class="flex-shrink-0">
          <Play class="h-4 w-4" />
        </Button>
      {/if}
      <CopyButton
        text={buildCurl('POST', looksLikeStream(url) ? '/api/play-stream' : '/api/play-url', { camera: camera.name, url, gain })}
        disabled={!url} label={looksLikeStream(url) ? 'Copy curl — play stream endpoint' : 'Copy curl — play URL endpoint'}
        preview={!!url} previewType="curl"
        class="flex-shrink-0"
      />
    </div>

    <!-- Drag overlay hint -->
    {#if isDragOver}
      <div class="flex items-center justify-center gap-2 rounded-lg border border-dashed border-primary py-3 text-sm text-primary">
        <FileAudio class="h-4 w-4" />
        Drop to play on {camera.name}
      </div>
    {/if}

    <!-- Status -->
    {#if status}
      <div class="text-sm {statusType === 'err' ? 'text-destructive' : statusType === 'warn' ? 'text-yellow-500' : 'text-primary'}">
        {status}
      </div>
    {/if}

    <!-- Playback indicator (from server state, survives page refresh) -->
    {#if streaming && playbackDetail && !status}
      <div class="flex items-center gap-1.5 text-xs text-muted-foreground">
        {#if paused}
          <Pause class="h-3 w-3" />
        {:else}
          <Play class="h-3 w-3" />
        {/if}
        <span class="truncate">{playbackDetail}</span>
      </div>
    {/if}

    <!-- Snapshot + description -->
    {#if snapshot}
      <div class="rounded-lg border border-primary/30 overflow-hidden relative">
        <img src={snapshot} alt="Camera snapshot" class="w-full" />
        <Button
          variant="outline" size="icon"
          onclick={clearSnapshot} disabled={busy}
          title="Clear snapshot and description" aria-label="Clear"
          class="absolute top-2 right-2 h-7 w-7 bg-background/80 backdrop-blur"
        >
          <X class="h-4 w-4" />
        </Button>
        {#if description}
          <div class="flex flex-col gap-2 p-2 bg-muted/30">
            <p class="text-xs text-muted-foreground">{description}</p>
            {#if describeTiming}
              {#if desktop}
                <Tooltip
                  content={timingTooltipContent(describeTimingsRaw, describeTotalMs, describeTtfsMs)}
                  multiline
                  side="bottom"
                  class="text-xs"
                >
                  <span class="text-xs text-primary/70 cursor-help inline-flex items-center gap-1 w-fit">
                    ⏱ {describeTiming}
                  </span>
                </Tooltip>
              {:else}
                <span class="text-xs text-primary/70 inline-flex items-center gap-1 w-fit">
                  ⏱ {describeTiming}
                </span>
              {/if}
            {/if}
            <div class="flex items-center gap-1.5">
              <Button
                variant="outline" size="icon"
                onclick={replayDescription} disabled={busy}
                title="Re-play description via TTS" aria-label="Re-play"
                class="h-7 w-7 flex-shrink-0"
              >
                <Play class="h-3.5 w-3.5" />
              </Button>
              <CopyButton
                text={buildCurl('POST', '/api/speak', { camera: camera.name, text: description, voice: voice || undefined, gain })}
                label="Copy curl — re-play description as TTS"
                preview previewType="curl"
                class="h-7 w-7 flex-shrink-0"
              />
              <CopyButton
                text={buildCurl('POST', '/api/describe', { camera: camera.name, gain, ...(visionPrompt ? { prompt: visionPrompt } : {}) })}
                label="Copy curl — describe endpoint"
                preview previewType="curl"
                class="h-7 w-7 flex-shrink-0"
              />
            </div>
          </div>
        {/if}
        <!-- Re-describe button (prompt is edited via the popup) -->
        <div class="flex justify-end p-2 border-t bg-muted/20">
          <Button
            variant="secondary" size="sm"
            onclick={describe} disabled={busy}
            title="Re-describe with current prompt" class="text-xs"
          >
            {#if busy && status.toLowerCase().includes('describ')}
              <Loader2 class="h-3.5 w-3.5 animate-spin" />
            {:else}
              <Eye class="h-3.5 w-3.5" />
            {/if}
            Re-describe
          </Button>
        </div>
      </div>
    {/if}
  </Card>

  <!-- Vision prompt popup -->
  {#if showPromptPopup}
    <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
    <!-- Backdrop: dims page and closes popup on click -->
    <div class="fixed inset-0 z-[100] bg-black/60" onclick={() => showPromptPopup = false}></div>
    <div class="fixed z-[101] top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 rounded-lg border border-border bg-card shadow-2xl p-4 flex flex-col gap-3 w-[400px] max-w-[calc(100vw-2rem)]">
      <div class="flex items-center justify-between">
        <h4 class="text-sm font-semibold text-foreground">Vision Prompt</h4>
        <Button variant="ghost" size="icon" class="h-6 w-6" onclick={() => showPromptPopup = false} title="Close">
          <X class="h-3.5 w-3.5" />
        </Button>
      </div>
      <p class="text-xs text-muted-foreground">
        Custom prompt for this camera's describe action. Overrides the global default.
        Leave empty to use the global vision prompt.
      </p>
      <Textarea
        bind:value={visionPrompt}
        rows="4"
        placeholder="e.g. How many people do you see? Describe any vehicles."
        class="text-xs"
        disabled={busy}
      />
      <div class="flex gap-2 justify-end">
        {#if visionPrompt !== savedPrompt}
          <Button variant="ghost" size="sm" onclick={() => visionPrompt = savedPrompt} title="Reset to saved camera prompt">
            Reset
          </Button>
        {/if}
        <Button variant="secondary" size="sm" onclick={() => { showPromptPopup = false; describe() }} disabled={busy}>
          <Eye class="h-3.5 w-3.5" />
          Apply & Describe
        </Button>
        <Button variant="default" size="sm" onclick={() => showPromptPopup = false}>
          Done
        </Button>
      </div>
    </div>
  {/if}

  <!-- Camera info modal -->
  <CameraInfoModal
    cameraName={camera.name}
    cameraType={camera.type}
    show={showInfoModal}
    onClose={() => showInfoModal = false}
  />
</div>
