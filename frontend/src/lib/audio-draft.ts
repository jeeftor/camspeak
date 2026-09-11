import type { DescribeJob, DescribeResponse, Preset } from './types'

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
  describeJob: DescribeJob | null
  describeError: string
  pendingAction: string
  waveformPreset: Pick<Preset, 'name' | 'category' | 'duration'> | null
  previewEnabled: boolean
}

export function createAudioDraft(gain = 3, prompt = ''): AudioDraft {
  return { mode: 'speak', text: '', voice: '', preset: '', streamPreset: '', url: '', urlMode: 'auto',
    loop: 0, prompt, gain, savedGain: gain, gainSaving: false, busy: false,
    lastResult: null, describeJob: null, describeError: '', pendingAction: '', waveformPreset: null, previewEnabled: true }
}

export const describeStages = [
  { stage: 'snapshot', label: 'Snapshot', active: 'Capturing snapshot', timing: 'snapshot_ms' },
  { stage: 'vision', label: 'Vision', active: 'Describing image', timing: 'vision_ms' },
  { stage: 'tts', label: 'Speech', active: 'Generating speech', timing: 'tts_ms' },
  { stage: 'transcode', label: 'Convert', active: 'Converting audio', timing: 'transcode_ms' },
  { stage: 'connecting', label: 'Connect', active: 'Connecting to speaker', timing: 'send_open_ms' },
  { stage: 'playing', label: 'Playback', active: 'Sending camera audio', timing: 'send_ms' },
] as const

export function describeStageLabel(stage: DescribeJob['stage']): string {
  return describeStages.find(step => step.stage === stage)?.active
    ?? (stage === 'done' ? 'Audio sent' : stage === 'canceled' ? 'Describe canceled' : 'Describe failed')
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
