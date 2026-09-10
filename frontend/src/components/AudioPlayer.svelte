<script lang="ts">
  /**
   * AudioPlayer — unified audio player widget.
   *
   * Combines:
   *   - MiniWaveform (canvas peaks + play/pause + click-to-seek)
   *   - Time / duration display
   *   - VU meter (always visible, driven by audioLevel prop or Web Audio API)
   *   - Optional metadata line (title, subtitle, extra fields)
   *
   * Two modes:
   *   - Interactive (default): play button plays audio in the browser via HTML5 Audio.
   *     VU meter uses Web Audio AnalyserNode for real levels.
   *   - Visual (visualMode=true): no play button, progress driven by externalProgress prop.
   *     VU meter uses the audioLevel prop (e.g. from SSE stream-levels).
   *
   * Props:
   *   peaksUrl        - URL to fetch peaks JSON { peaks: number[], duration: number }
   *   audioUrl        - URL to the audio file for browser playback (optional in visual mode)
   *   duration        - initial duration in seconds (overridden by peaks/audio metadata)
   *   visualMode      - hide play button, use externalProgress
   *   externalProgress - 0..1 playhead position (visual mode only)
   *   audioLevel      - 0..1 audio level for VU meter (visual mode only)
   *   title           - metadata title (e.g. preset name)
   *   subtitle        - metadata subtitle (e.g. category)
   *   metadata        - array of extra metadata strings (e.g. codec, bitrate, size)
   *   vuOrientation   - "horizontal" (default) or "vertical"
   */

  import MiniWaveform from './MiniWaveform.svelte'

  let {
    peaksUrl,
    audioUrl = '',
    duration = 0,
    visualMode = false,
    externalProgress = -1,
    audioLevel = 0,
    title = '',
    subtitle = '',
    metadata = [],
    vuOrientation = 'horizontal',
  }: {
    peaksUrl: string
    audioUrl?: string
    duration?: number
    visualMode?: boolean
    externalProgress?: number
    audioLevel?: number
    title?: string
    subtitle?: string
    metadata?: string[]
    vuOrientation?: 'horizontal' | 'vertical'
  } = $props()

  // Build the metadata line: title · subtitle · meta1 · meta2 ...
  let metaLine = $derived(
    [title, subtitle, ...metadata].filter(Boolean).join(' · ')
  )

  // --- Web Audio API for interactive mode VU levels ---
  // In interactive mode, we tap into the MiniWaveform's HTML5 Audio element
  // via an AnalyserNode to get real-time audio levels for the VU meter.
  let analyser: AnalyserNode | null = null
  let audioCtx: AudioContext | null = null
  let sourceNode: MediaElementAudioSourceNode | null = null
  let levelRaf = 0
  let localLevel = $state(0)

  // The effective level: external in visual mode, local in interactive mode
  let effectiveLevel = $derived(visualMode ? audioLevel : localLevel)

  function setupAnalyser(audioEl: HTMLAudioElement) {
    if (sourceNode) return // already connected
    try {
      audioCtx = new AudioContext()
      sourceNode = audioCtx.createMediaElementSource(audioEl)
      analyser = audioCtx.createAnalyser()
      analyser.fftSize = 256
      analyser.smoothingTimeConstant = 0.8
      sourceNode.connect(analyser)
      analyser.connect(audioCtx.destination)
    } catch {
      // AudioContext not available or source already connected
    }
  }

  function startLevelMonitoring() {
    if (levelRaf) return
    const dataArray = new Uint8Array(analyser ? analyser.frequencyBinCount : 0)
    function tick() {
      if (analyser) {
        analyser.getByteTimeDomainData(dataArray)
        // Compute RMS level from time-domain data
        let sum = 0
        for (let i = 0; i < dataArray.length; i++) {
          const v = (dataArray[i] - 128) / 128
          sum += v * v
        }
        const rms = Math.sqrt(sum / dataArray.length)
        localLevel = Math.min(1, rms * 3) // scale up for visibility
      }
      levelRaf = requestAnimationFrame(tick)
    }
    tick()
  }

  function stopLevelMonitoring() {
    if (levelRaf) {
      cancelAnimationFrame(levelRaf)
      levelRaf = 0
    }
    localLevel = 0
  }

  // Listen for play/pause events from the MiniWaveform's audio element.
  // We use a MutationObserver-free approach: poll for the audio element
  // after mount and attach listeners.
  let waveContainer: HTMLDivElement | null = $state(null)
  let monitoredAudio: HTMLAudioElement | null = null

  $effect(() => {
    const container = waveContainer
    if (!container || visualMode) return

    // Find the audio element created by MiniWaveform
    // It's created lazily on first play, so we poll
    const checkInterval = setInterval(() => {
      const audio = container.querySelector('audio')
      if (audio && audio !== monitoredAudio) {
        monitoredAudio = audio
        setupAnalyser(audio)
        audio.addEventListener('play', () => {
          if (audioCtx?.state === 'suspended') audioCtx.resume()
          startLevelMonitoring()
        })
        audio.addEventListener('pause', stopLevelMonitoring)
        audio.addEventListener('ended', stopLevelMonitoring)
      }
    }, 500)

    return () => {
      clearInterval(checkInterval)
      stopLevelMonitoring()
      if (sourceNode) sourceNode.disconnect()
      if (analyser) analyser.disconnect()
      if (audioCtx) audioCtx.close()
      sourceNode = null
      analyser = null
      audioCtx = null
      monitoredAudio = null
    }
  })
</script>

<div class="flex flex-col gap-1" bind:this={waveContainer}>
  <div class="flex gap-2">
    <!-- Waveform + play button + time -->
    <div class="flex-1 min-w-0">
      <MiniWaveform
        {peaksUrl}
        {audioUrl}
        {duration}
        {visualMode}
        {externalProgress}
      />
    </div>

    <!-- VU meter -->
    {#if vuOrientation === 'vertical'}
      <div class="flex flex-col items-center gap-1 flex-shrink-0">
        <div class="flex flex-col h-12 w-3 overflow-hidden rounded-full bg-muted gap-px justify-end" title="Audio level">
          {#each Array(20) as _, i}
            {@const idx = 19 - i}
            {@const lit = (idx + 1) / 20 <= effectiveLevel}
            {@const segClass = lit ? (idx < 12 ? 'bg-green-500' : idx < 17 ? 'bg-yellow-500' : 'bg-red-500') : 'bg-muted-foreground/20'}
            <div class="flex-1 transition-colors duration-75 {segClass}"></div>
          {/each}
        </div>
        <span class="text-[9px] tabular-nums text-muted-foreground">{Math.round(effectiveLevel * 100)}</span>
      </div>
    {:else}
      <div class="flex items-center gap-1.5 flex-shrink-0">
        <div class="flex h-2 w-32 overflow-hidden rounded-full bg-muted gap-px" title="Audio level">
          {#each Array(20) as _, i}
            {@const lit = (i + 1) / 20 <= effectiveLevel}
            {@const segClass = lit ? (i < 12 ? 'bg-green-500' : i < 17 ? 'bg-yellow-500' : 'bg-red-500') : 'bg-muted-foreground/20'}
            <div class="flex-1 transition-colors duration-75 {segClass}"></div>
          {/each}
        </div>
        <span class="text-[10px] tabular-nums text-muted-foreground">{Math.round(effectiveLevel * 100)}%</span>
      </div>
    {/if}
  </div>

  <!-- Metadata line -->
  {#if metaLine}
    <div class="text-xs text-muted-foreground truncate" title={metaLine}>
      {metaLine}
    </div>
  {/if}
</div>
