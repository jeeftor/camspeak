<script lang="ts">
  /**
   * AudioPlayer — unified audio player widget.
   *
   * Combines:
   *   - MiniWaveform (canvas peaks + play/pause + click-to-seek)
   *   - Time / duration display
   *   - VU meter (always visible, driven by audioLevel prop)
   *   - Optional metadata line (title, subtitle, extra fields)
   *
   * Two modes:
   *   - Interactive (default): play button plays audio in the browser via HTML5 Audio.
   *   - Visual (visualMode=true): no play button, progress driven by externalProgress prop.
   *     Used when audio plays on an external device (e.g. camera speaker) and we
   *     only need visual feedback.
   *
   * Props:
   *   peaksUrl        - URL to fetch peaks JSON { peaks: number[], duration: number }
   *   audioUrl        - URL to the audio file for browser playback (optional in visual mode)
   *   duration        - initial duration in seconds (overridden by peaks/audio metadata)
   *   visualMode      - hide play button, use externalProgress
   *   externalProgress - 0..1 playhead position (visual mode only)
   *   audioLevel      - 0..1 audio level for VU meter
   *   title           - metadata title (e.g. preset name)
   *   subtitle        - metadata subtitle (e.g. category)
   *   metadata        - array of extra metadata strings (e.g. codec, bitrate, size)
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
  } = $props()

  // Build the metadata line: title · subtitle · meta1 · meta2 ...
  let metaLine = $derived(
    [title, subtitle, ...metadata].filter(Boolean).join(' · ')
  )
</script>

<div class="flex flex-col gap-1">
  <!-- Waveform + play button + time -->
  <MiniWaveform
    {peaksUrl}
    {audioUrl}
    {duration}
    {visualMode}
    {externalProgress}
  />

  <!-- VU meter: always visible — flat when idle, live during playback -->
  <div class="flex items-center gap-1.5">
    <div class="flex h-2 w-32 overflow-hidden rounded-full bg-muted gap-px" title="Audio level">
      {#each Array(20) as _, i}
        {@const lit = (i + 1) / 20 <= audioLevel}
        {@const segClass = lit ? (i < 12 ? 'bg-green-500' : i < 17 ? 'bg-yellow-500' : 'bg-red-500') : 'bg-muted-foreground/20'}
        <div class="flex-1 transition-colors duration-75 {segClass}"></div>
      {/each}
    </div>
    <span class="text-[10px] tabular-nums text-muted-foreground">{Math.round(audioLevel * 100)}%</span>
  </div>

  <!-- Metadata line -->
  {#if metaLine}
    <div class="text-xs text-muted-foreground truncate" title={metaLine}>
      {metaLine}
    </div>
  {/if}
</div>
