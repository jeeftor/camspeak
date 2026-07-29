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
