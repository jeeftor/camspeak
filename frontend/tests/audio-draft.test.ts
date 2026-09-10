import { expect, test } from 'bun:test'
import { createAudioDraft, isValidRepeat, timingSteps, cameraSwipe } from '../src/lib/audio-draft'

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
  front.lastResult = { label: 'Describe', timings: { vision_ms: 1200 }, total_ms: 2000 }
  expect(back.lastResult).toBeNull()
  expect(front.previewEnabled).toBe(true)
})

test('output timings follow the pipeline, retain zero and unknown steps, and deduplicate capture aliases', () => {
  expect(timingSteps({ send_playback_ms: 8000, vision_ms: 1200, snapshot_ms: 180,
    snap_ms: 180, tts_ms: 0, custom_ms: 25, bad_ms: NaN, invalid_ms: -1 }).map(([key]) => key))
    .toEqual(['snapshot_ms', 'vision_ms', 'tts_ms', 'send_playback_ms', 'custom_ms'])
  expect(timingSteps(undefined)).toEqual([])
})

test('camera swipes require a deliberate horizontal gesture', () => {
  expect(cameraSwipe(-90, 10)).toBe(1)
  expect(cameraSwipe(90, 10)).toBe(-1)
  expect(cameraSwipe(30, 5)).toBe(0)
  expect(cameraSwipe(80, 90)).toBe(0)
})
