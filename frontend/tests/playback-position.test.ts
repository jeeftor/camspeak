import { expect, test } from 'bun:test'
import { presetPlaybackPosition } from '../src/lib/playback-position'
import type { PlaybackState } from '../src/lib/types'

const start = Date.parse('2026-09-10T12:00:00Z')
const preset = { name: 'Doorbell', duration: 4 }
const playback: PlaybackState = { state: 'playing', source: 'play', detail: 'Doorbell', started_at: new Date(start).toISOString() }

test('preset playhead advances, clamps once complete, and tolerates future timestamps', () => {
  expect(presetPlaybackPosition(playback, preset, start + 1000)).toBe(0.25)
  expect(presetPlaybackPosition(playback, preset, start + 3000)).toBe(0.75)
  expect(presetPlaybackPosition(playback, preset, start + 9000)).toBe(1)
  expect(presetPlaybackPosition(playback, preset, start - 1000)).toBe(0)
})

test('pause freezes position and resume uses the server-adjusted start time', () => {
  const paused: PlaybackState = { ...playback, state: 'paused', paused_at: new Date(start + 1000).toISOString() }
  expect(presetPlaybackPosition(paused, preset, start + 3000)).toBe(0.25)
  expect(presetPlaybackPosition(paused, preset, start + 9000)).toBe(0.25)
  const resumed = { ...playback, started_at: new Date(start + 8000).toISOString() }
  expect(presetPlaybackPosition(resumed, preset, start + 10000)).toBe(0.5)
})

test('finite repeats wrap each iteration, infinite repeats keep wrapping', () => {
  const finite = { ...playback, detail: 'Doorbell (loop 3x)' }
  expect(presetPlaybackPosition(finite, preset, start + 5000)).toBe(0.25)
  expect(presetPlaybackPosition(finite, preset, start + 9000)).toBe(0.25)
  expect(presetPlaybackPosition(finite, preset, start + 12000)).toBe(1)
  const infinite = { ...playback, detail: 'Doorbell (loop)' }
  expect(presetPlaybackPosition(infinite, preset, start + 13000)).toBe(0.25)
})

test('preparing, stopped, unrelated and unknown playback remains a reference waveform', () => {
  for (const state of ['preparing', 'idle']) {
    expect(presetPlaybackPosition({ ...playback, state } as PlaybackState, preset, start + 1000)).toBeNull()
  }
  for (const detail of ['Other preset', 'Doorbell (loop nope)', 'Doorbell (loop 0x)', 'Doorbell (loop 1.5x)']) {
    expect(presetPlaybackPosition({ ...playback, detail }, preset, start + 1000)).toBeNull()
  }
  expect(presetPlaybackPosition({ ...playback, source: 'speak' }, preset, start + 1000)).toBeNull()
  expect(presetPlaybackPosition({ ...playback, started_at: undefined }, preset, start + 1000)).toBeNull()
  expect(presetPlaybackPosition({ ...playback, state: 'paused' }, preset, start + 1000)).toBeNull()
  expect(presetPlaybackPosition(playback, { ...preset, duration: 0 }, start + 1000)).toBeNull()
  expect(presetPlaybackPosition(undefined, preset, start + 1000)).toBeNull()
  expect(presetPlaybackPosition(playback, null, start + 1000)).toBeNull()
})
