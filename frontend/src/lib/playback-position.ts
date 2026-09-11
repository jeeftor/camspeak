import type { PlaybackState, Preset } from './types'

/** Estimate file position from reported timing, never from measured speaker sound. */
export function presetPlaybackPosition(
  playback: PlaybackState | undefined,
  preset: Pick<Preset, 'name' | 'duration'> | null,
  now: number,
): number | null {
  if (!preset || !Number.isFinite(preset.duration) || preset.duration <= 0
    || playback?.source !== 'play' || !['playing', 'paused'].includes(playback.state)) return null

  // The server identifies finite/infinite preset loops in the playback detail.
  // Do not animate an old preset waveform for another action or another preset.
  const detail = playback.detail ?? ''
  let repeats = 1
  if (detail === `${preset.name} (loop)`) repeats = Infinity
  else if (detail !== preset.name) {
    const prefix = `${preset.name} (loop `
    if (!detail.startsWith(prefix) || !detail.endsWith('x)')) return null
    const count = detail.slice(prefix.length, -2)
    if (!/^\d+$/.test(count)) return null
    repeats = Number(count)
    if (!Number.isSafeInteger(repeats) || repeats < 1) return null
  }

  const start = Date.parse(playback.started_at ?? '')
  const end = playback.state === 'paused' ? Date.parse(playback.paused_at ?? '') : now
  if (!Number.isFinite(start) || !Number.isFinite(end)) return null
  const elapsed = Math.max(0, (end - start) / 1000)
  if (elapsed >= preset.duration * repeats) return 1
  return (elapsed % preset.duration) / preset.duration
}
