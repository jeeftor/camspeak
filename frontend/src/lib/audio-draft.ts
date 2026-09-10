/** Inputs and operation state retained when you switch cameras or close the panel. */
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
  description: string
  image: string
}

export function createAudioDraft(gain = 3, prompt = ''): AudioDraft {
  return { mode: 'speak', text: '', voice: '', preset: '', streamPreset: '', url: '', urlMode: 'auto',
    loop: 0, prompt, gain, savedGain: gain, gainSaving: false, busy: false, description: '', image: '' }
}

/** Preserve the API convention: zero plays once, N repeats N times, -1 loops. */
export function isValidRepeat(loop: number): boolean {
  return Number.isSafeInteger(loop) && loop >= -1
}
