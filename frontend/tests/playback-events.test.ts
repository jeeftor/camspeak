import { expect, test } from 'bun:test'
import { eventRequest, mergePlaybackEvent, type PlaybackEvent } from '../src/lib/playback-events'
import { buildCurlCommand } from '../src/lib/curl-command'

const event: PlaybackEvent = { id: 3, camera: 'front', action: 'play', text: 'Alert', at: '2026-09-10T20:00:00Z', replay: { method: 'POST', path: '/api/play', body: { camera: 'front', preset: "It's $(touch /tmp/nope)", category: 'alerts', loop: -1, gain: 0 } } }

test('history and reconnect duplicates retain one stable row, newest first', () => {
  let rows = mergePlaybackEvent([], event)
  rows = mergePlaybackEvent(rows, { ...event, id: 4 })
  rows = mergePlaybackEvent(rows, event)
  expect(rows.map(row => row.id)).toEqual([4, 3])
})

test('replay preserves full request and shell quoting', () => {
  const request = eventRequest(event)!
  expect(request.body).toEqual(event.replay!.body)
  const curl = buildCurlCommand('https://camspeak.example', request.method, request.path, request.body)
  expect(curl).toContain("It'\\''s $(touch /tmp/nope)")
  expect(curl).toContain('"gain":0')
  expect(curl).toContain('"loop":-1')
})

test('legacy plays and vision results do not invent incomplete commands', () => {
  expect(eventRequest({ ...event, replay: undefined })).toBeUndefined()
  expect(eventRequest({ ...event, action: 'vision' })).toBeUndefined()
  expect(mergePlaybackEvent([], { ...event, action: 'config' })).toEqual([])
  expect(eventRequest({ ...event, action: 'stop', replay: undefined })?.body).toEqual({ camera: 'front' })
})
