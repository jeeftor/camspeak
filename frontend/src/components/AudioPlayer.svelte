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
  import VuMeter from './VuMeter.svelte'

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

  let monitoredAudio: HTMLAudioElement | null = $state(null)

  $effect(() => {
    const audio = monitoredAudio
    if (!audio || visualMode) return
    setupAnalyser(audio)
    const onPlay = () => {
      if (audioCtx?.state === 'suspended') void audioCtx.resume()
      startLevelMonitoring()
    }
    audio.addEventListener('play', onPlay)
    audio.addEventListener('pause', stopLevelMonitoring)
    audio.addEventListener('ended', stopLevelMonitoring)
    if (!audio.paused) onPlay()

    return () => {
      audio.removeEventListener('play', onPlay)
      audio.removeEventListener('pause', stopLevelMonitoring)
      audio.removeEventListener('ended', stopLevelMonitoring)
      stopLevelMonitoring()
      if (sourceNode) sourceNode.disconnect()
      if (analyser) analyser.disconnect()
      if (audioCtx) audioCtx.close()
      sourceNode = null
      analyser = null
      audioCtx = null
    }
  })
</script>

<div class="flex flex-col gap-1">
  <div class="flex gap-2">
    <!-- Waveform + play button + time -->
    <div class="flex-1 min-w-0">
      <MiniWaveform
        {peaksUrl}
        {audioUrl}
        {duration}
        {visualMode}
        {externalProgress}
        onAudio={(audio) => { monitoredAudio = audio }}
      />
    </div>

    <!-- VU meter -->
    <VuMeter level={effectiveLevel} vertical={vuOrientation === 'vertical'} />
  </div>

  <!-- Metadata line -->
  {#if metaLine}
    <div class="text-xs text-muted-foreground truncate" title={metaLine}>
      {metaLine}
    </div>
  {/if}
</div>
