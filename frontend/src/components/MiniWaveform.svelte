<script lang="ts">
  /**
   * MiniWaveform — canvas-based audio waveform for library presets.
   *
   * Renders a canvas waveform (peaks fetched lazily from
   * /api/library/:category/:name/peaks — ~2KB JSON vs full WAV download),
   * with a play/pause button, progress overlay, time display, and
   * click-to-seek. Audio is streamed via the existing /preview endpoint.
   *
   * Adapted from musicgen's MiniWaveform.svelte.
   */

  import { Play, Pause } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'

  // Module-level cache: key -> { peaks, duration }. Shared across all
  // MiniWaveform instances so repeated mounts don't re-fetch.
  const peaksCache = new Map<string, { peaks: number[]; duration: number }>()

  let {
    category,
    name,
    duration: initialDuration = 0,
  }: {
    category: string
    name: string
    duration?: number
  } = $props()

  const key = `${category}/${name}`
  const previewUrl = `/api/library/${encodeURIComponent(category)}/${encodeURIComponent(name)}/preview`
  const peaksUrl = `/api/library/${encodeURIComponent(category)}/${encodeURIComponent(name)}/peaks`

  // --- DOM refs ---
  let canvasEl: HTMLCanvasElement | null = $state(null)
  let containerEl: HTMLDivElement | null = $state(null)

  // --- State ---
  let peaks: number[] | null = $state(null)
  let duration = $state(initialDuration)
  let playing = $state(false)
  let progress = $state(0) // 0..1 playback position
  let currentTime = $state(0) // seconds

  // --- Audio + animation handles (non-reactive) ---
  let audio: HTMLAudioElement | null = null
  let raf = 0
  let decoded = false
  let io: IntersectionObserver | null = null

  // --- Peaks fetching ---
  async function loadPeaks(): Promise<void> {
    if (decoded) return
    decoded = true
    const cached = peaksCache.get(key)
    if (cached) {
      peaks = cached.peaks
      duration = cached.duration
      return
    }
    try {
      const res = await fetch(peaksUrl)
      if (!res.ok) return
      const data = await res.json()
      if (data.peaks) {
        peaksCache.set(key, { peaks: data.peaks, duration: data.duration })
        peaks = data.peaks
        duration = data.duration
      }
    } catch {
      // leave peaks null — canvas draws a flat placeholder line
    }
  }

  // --- Drawing ---
  function draw(): void {
    const canvas = canvasEl
    if (!canvas) return
    const ctx = canvas.getContext('2d')
    if (!ctx) return
    const w = (canvas.width = canvas.offsetWidth || 200)
    const h = (canvas.height = canvas.offsetHeight || 40)
    ctx.clearRect(0, 0, w, h)

    if (!peaks || peaks.length === 0) {
      ctx.fillStyle = '#475569'
      ctx.fillRect(0, h / 2 - 1, w, 2)
      return
    }

    const barW = w / peaks.length
    const progressX = progress * w

    // Draw bars: played = amber, unplayed = slate
    for (let i = 0; i < peaks.length; i++) {
      const barH = Math.max(2, peaks[i] * h * 0.9)
      const x = i * barW
      const y = (h - barH) / 2
      ctx.fillStyle = x < progressX ? '#f59e0b' : '#64748b'
      ctx.fillRect(x, y, Math.max(1, barW - 0.5), barH)
    }

    // Draw playhead line
    if (playing || progress > 0) {
      ctx.fillStyle = '#fff'
      ctx.fillRect(progressX - 1, 0, 2, h)
    }
  }

  $effect(() => {
    void peaks
    void progress
    void playing
    draw()
  })

  // --- Audio helpers ---
  function ensureAudio(): HTMLAudioElement {
    if (audio) return audio
    audio = new Audio(previewUrl)
    audio.preload = 'metadata'
    audio.addEventListener('loadedmetadata', () => {
      // Use the audio element's actual duration (more accurate than
      // the file-size-based estimate from the peaks endpoint).
      if (audio && audio.duration && isFinite(audio.duration) && audio.duration > 0) {
        duration = audio.duration
      }
    })
    audio.addEventListener('ended', () => {
      playing = false
      progress = 0
      currentTime = 0
      cancelRaf()
    })
    audio.addEventListener('pause', () => {
      cancelRaf()
    })
    return audio
  }

  function cancelRaf(): void {
    if (raf) {
      cancelAnimationFrame(raf)
      raf = 0
    }
  }

  function updateProgress(): void {
    if (!audio || audio.paused) return
    currentTime = audio.currentTime
    const dur = audio.duration && isFinite(audio.duration) && audio.duration > 0
      ? audio.duration
      : duration
    progress = dur > 0 ? audio.currentTime / dur : 0
    raf = requestAnimationFrame(updateProgress)
  }

  // --- Time display ---
  function formatTime(s: number): string {
    if (!s || !isFinite(s)) return '0:00'
    const m = Math.floor(s / 60)
    const sec = Math.floor(s % 60)
    return `${m}:${sec.toString().padStart(2, '0')}`
  }

  let timeLabel = $derived(
    duration > 0
      ? playing || currentTime > 0
        ? `${formatTime(currentTime)} / ${formatTime(duration)}`
        : formatTime(duration)
      : '',
  )

  // --- Interaction ---
  async function togglePlay(): Promise<void> {
    if (!peaks) await loadPeaks()
    const a = ensureAudio()
    if (a.paused) {
      // If we just finished (progress=0), restart from beginning
      if (progress >= 1) {
        a.currentTime = 0
        progress = 0
        currentTime = 0
      }
      await a.play()
      playing = true
      updateProgress()
    } else {
      a.pause()
      playing = false
    }
  }

  async function seek(e: MouseEvent): Promise<void> {
    if (!canvasEl) return
    if (!peaks) await loadPeaks()
    const a = ensureAudio()
    const rect = canvasEl.getBoundingClientRect()
    const pct = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width))
    const dur = a.duration && isFinite(a.duration) && a.duration > 0
      ? a.duration
      : duration
    a.currentTime = pct * dur
    progress = pct
    currentTime = a.currentTime
    if (!a.paused) {
      // Already playing, keep tracking
    } else {
      // Paused — update display but don't resume
      draw()
    }
  }

  // --- Lifecycle ---
  $effect(() => {
    const container = containerEl
    if (!container) return
    io = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) {
          void loadPeaks()
          io?.disconnect()
          io = null
        }
      },
      { rootMargin: '200px' },
    )
    io.observe(container)

    const ro = new ResizeObserver(() => draw())
    ro.observe(container)

    return () => {
      io?.disconnect()
      io = null
      ro.disconnect()
    }
  })

  $effect(() => {
    return () => {
      cancelRaf()
      if (audio) {
        audio.pause()
        audio.src = ''
        audio = null
      }
    }
  })
</script>

<div class="flex items-center gap-2 w-full" bind:this={containerEl}>
  <Button
    variant="outline"
    size="icon"
    class="h-8 w-8 shrink-0"
    onclick={togglePlay}
    aria-label={playing ? 'Pause' : 'Play'}
    title={playing ? 'Pause' : 'Play'}
  >
    {#if playing}
      <Pause class="h-4 w-4" />
    {:else}
      <Play class="h-4 w-4" />
    {/if}
  </Button>
  <canvas
    bind:this={canvasEl}
    onclick={seek}
    class="flex-1 h-10 cursor-pointer block min-w-0 rounded"
    title="Click to seek"
  ></canvas>
  <span class="text-xs text-muted-foreground font-mono whitespace-nowrap shrink-0 min-w-[60px] text-right">
    {timeLabel}
  </span>
</div>
