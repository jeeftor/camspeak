export interface PlaybackEvent {
  id?: number
  camera: string
  action: string
  text?: string
  voice?: string
  at: string
  replay?: { method: string; path: string; body: Record<string, unknown>; redacted?: boolean }
}

const actions = new Set(['speak', 'play', 'play-url', 'play-stream', 'describe', 'announce', 'broadcast', 'beep', 'stop', 'stop-all', 'pause', 'resume'])

export function eventKey(event: PlaybackEvent): string {
  return event.id ? `id:${event.id}` : JSON.stringify([event.at, event.camera, event.action, event.text, event.voice])
}

export function mergePlaybackEvent(events: PlaybackEvent[], event: PlaybackEvent): PlaybackEvent[] {
  if (!actions.has(event.action)) return events
  return [event, ...events.filter(previous => eventKey(previous) !== eventKey(event))]
    .sort((a, b) => Date.parse(b.at) - Date.parse(a.at) || (b.id ?? 0) - (a.id ?? 0)).slice(0, 100)
}

export function eventRequest(event: PlaybackEvent): PlaybackEvent['replay'] {
  if (!actions.has(event.action)) return undefined
  if (event.replay?.method === 'POST' && /^\/api\/(speak|play|play-url|play-stream|describe|announce|broadcast|beep|stop|pause|resume)$/.test(event.replay.path)) return event.replay
  // Old records omitted gain, category, loop and vision options; only controls are unambiguous.
  if (['stop', 'pause', 'resume'].includes(event.action) && event.camera) {
    return { method: 'POST', path: `/api/${event.action}`, body: { camera: event.camera } }
  }
  if (event.action === 'stop-all') return { method: 'POST', path: '/api/stop', body: {} }
  return undefined
}
