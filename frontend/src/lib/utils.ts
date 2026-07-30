export function cn(...classes: (string | false | null | undefined)[]) {
  return classes.filter(Boolean).join(' ')
}

// Format a millisecond value compactly: under 1000ms shows "450ms",
// 1000ms and above shows seconds with one decimal, e.g. "1.4s".
export function formatMs(ms: number | undefined | null): string {
  if (ms == null || isNaN(ms)) return ''
  if (ms < 1000) return `${Math.round(ms)}ms`
  return `${(ms / 1000).toFixed(1)}s`
}

// Pretty label for a timing step key. Strips a trailing "_ms" suffix and
// applies a few friendly aliases (e.g. "tts_ms" -> "TTS", "snapshot_ms" -> "snap").
const STEP_ALIASES: Record<string, string> = {
  tts_ms: 'TTS',
  transcode_ms: 'transcode',
  send_ms: 'send',
  load_ms: 'load',
  snapshot_ms: 'snap',
  vision_ms: 'vision',
  save_ms: 'save',
}

export function stepLabel(key: string): string {
  if (STEP_ALIASES[key]) return STEP_ALIASES[key]
  return key.replace(/_ms$/, '').replace(/_/g, ' ')
}

// Render a compact timing breakdown string from a Timings object, e.g.
// "TTS 450ms · transcode 120ms · send 800ms". Returns '' if no timings.
export function formatTimings(timings: Record<string, number> | undefined): string {
  if (!timings) return ''
  const parts = Object.entries(timings)
    .filter(([, v]) => v != null && !isNaN(v))
    .map(([k, v]) => `${stepLabel(k)} ${formatMs(v)}`)
  return parts.join(' · ')
}

// Render a total + breakdown summary, e.g. "1.4s: TTS 450ms · send 800ms".
// Falls back to just the breakdown if total is missing.
export function formatTimingSummary(
  timings: Record<string, number> | undefined,
  totalMs: number | undefined,
): string {
  const breakdown = formatTimings(timings)
  if (totalMs != null && !isNaN(totalMs)) {
    return breakdown ? `${formatMs(totalMs)}: ${breakdown}` : formatMs(totalMs)
  }
  return breakdown
}

// Detect whether the current device is a touch-only / mobile device.
// Returns true when the primary pointer is coarse (no hover) and touch is supported.
export function isMobile(): boolean {
  if (typeof window === 'undefined') return false
  const hasTouch = navigator.maxTouchPoints > 0 || 'ontouchstart' in window
  const coarsePointer = window.matchMedia?.('(pointer: coarse)')?.matches ?? false
  const noHover = window.matchMedia?.('(hover: none)')?.matches ?? false
  return hasTouch && (coarsePointer || noHover)
}

// Human-readable description for each timing step, used in tooltips.
const STEP_DESCRIPTIONS: Record<string, string> = {
  snapshot_ms: 'Time to capture a still image from the camera stream',
  vision_ms: 'Time for the vision AI model to analyze the image and produce a description',
  tts_ms: 'Time to convert the description text into speech audio',
  transcode_ms: 'Time to transcode audio to G.711 μ-law 8 kHz mono (camera format)',
  send_ms: 'Time to stream the audio to the camera speaker',
  load_ms: 'Time to load audio data from disk',
  save_ms: 'Time to save audio data to disk',
}

// Build a detailed, multi-line timing breakdown for use in a tooltip.
// Each line: "Label  450ms — description of what this step does"
export function timingTooltipContent(
  timings: Record<string, number> | undefined,
  totalMs: number | undefined,
): string {
  const lines: string[] = []
  if (totalMs != null && !isNaN(totalMs)) {
    lines.push(`Total: ${formatMs(totalMs)}`)
  }
  if (timings) {
    for (const [key, val] of Object.entries(timings)) {
      if (val == null || isNaN(val)) continue
      const label = stepLabel(key)
      const desc = STEP_DESCRIPTIONS[key] ?? ''
      lines.push(desc ? `${label} ${formatMs(val)} — ${desc}` : `${label} ${formatMs(val)}`)
    }
  }
  return lines.join('\n')
}
