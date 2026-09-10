import { expect, test } from 'bun:test'
import { createAudioDraft, isValidRepeat } from '../src/lib/audio-draft'

test('repeat counts allow arbitrary finite repeats and the infinite sentinel', () => {
  for (const count of [-1, 0, 1, 2, 4, 25, 100]) expect(isValidRepeat(count)).toBe(true)
  for (const count of [-2, 0.5, NaN, Infinity, undefined as unknown as number]) {
    expect(isValidRepeat(count)).toBe(false)
  }
})

test('camera drafts keep independent finite and stream selections', () => {
  const front = createAudioDraft(0)
  const back = createAudioDraft()
  front.preset = JSON.stringify(['Sounds', 'Doorbell'])
  front.streamPreset = JSON.stringify(['Streams', 'Radio'])
  front.mode = 'stream'
  expect(front.preset).toBe(JSON.stringify(['Sounds', 'Doorbell']))
  expect(front.gain).toBe(0)
  expect(back.mode).toBe('speak')
  expect(back.streamPreset).toBe('')
  expect(back.preset).toBe('')
})
