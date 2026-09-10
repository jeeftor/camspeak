import type { DescribeResponse, Preset } from './types'

export interface AudioResult extends DescribeResponse {
  label: string
}

/** Inputs and results retained while you switch cameras in the dashboard. */
export interface AudioDraft {
  mode: 'speak' | 'preset' | 'stream' | 'url' | 'describe'
  text: string
  voice: string
  preset: string
  streamPreset: string
  url: string
  urlMode: 'auto' | 'file' | 'stream'
  loop: number
  prompt: string
  gain: number
  savedGain: number
  gainSaving: boolean
  busy: boolean
  lastResult: AudioResult | null
  pendingAction: string
  waveformPreset: Pick<Preset, 'name' | 'category' | 'duration'> | null
  previewEnabled: boolean
}

export function createAudioDraft(gain = 3, prompt = ''): AudioDraft {
  return { mode: 'speak', text: '', voice: '', preset: '', streamPreset: '', url: '', urlMode: 'auto',
    loop: 0, prompt, gain, savedGain: gain, gainSaving: false, busy: false,
    lastResult: null, pendingAction: '', waveformPreset: null, previewEnabled: true }
}

/** Preserve the API convention: zero plays once, N repeats N times, -1 loops. */
export function isValidRepeat(loop: number): boolean {
  return Number.isSafeInteger(loop) && loop >= -1
}

/** Show actual reported steps in pipeline order, without counting capture twice. */
export function timingSteps(timings: Record<string, number> | undefined): [string, number][] {
  const order = ['snapshot_ms', 'snap_ms', 'load_ms', 'download_ms', 'vision_ms', 'tts_ms',
    'transcode_ms', 'send_open_ms', 'send_ms', 'send_playback_ms', 'save_ms']
  return Object.entries(timings ?? {})
    .filter(([key, value]) => Number.isFinite(value) && value >= 0
      && !(key === 'snap_ms' && Number.isFinite(timings?.snapshot_ms)))
    .sort(([a], [b]) => (order.includes(a) ? order.indexOf(a) : order.length)
      - (order.includes(b) ? order.indexOf(b) : order.length))
}

/** Ignore taps and vertical scrolling; return the relative camera index change. */
export function cameraSwipe(dx: number, dy: number): number {
  return Math.abs(dx) >= 60 && Math.abs(dx) > Math.abs(dy) * 1.5 ? (dx < 0 ? 1 : -1) : 0
}
