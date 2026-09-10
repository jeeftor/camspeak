import { apiClient } from './api'
import type { PlaybackState } from './types'

/** Own one dashboard-wide playback poll and audio-level subscription. */
export function createPlaybackMonitor() {
  const state = $state({
    cameras: {} as Record<string, PlaybackState>,
    levels: {} as Record<string, number>,
    error: '',
  })
  let running = false
  let generation = 0
  let pending: Promise<void> | null = null
  let timer: ReturnType<typeof setTimeout> | undefined
  let events: EventSource | null = null

  function syncLevels() {
    const playing = Object.values(state.cameras).some(camera => camera.state === 'playing')
    if (!playing) {
      events?.close()
      events = null
      state.levels = {}
    } else if (!events) {
      events = new EventSource('/api/stream-levels')
      events.onmessage = event => {
        try { state.levels = JSON.parse(event.data) } catch { state.levels = {} }
      }
      events.onerror = () => { state.levels = {} }
    }
  }

  function refresh(): Promise<void> {
    if (!running) return Promise.resolve()
    if (pending) return pending
    const current = generation
    pending = (async () => {
      try {
        const cameras = await apiClient.getPlayback()
        if (!running || current !== generation) return
        state.cameras = cameras
        state.error = ''
        syncLevels()
      } catch {
        if (running && current === generation) {
          state.error = 'Playback status could not be refreshed. Your last known state is shown.'
        }
      } finally {
        pending = null
      }
    })()
    return pending
  }

  function start() {
    running = true
    generation++
    async function poll() {
      await refresh()
      if (running) timer = setTimeout(poll, 3000)
    }
    void poll()
    return () => {
      running = false
      generation++
      clearTimeout(timer)
      events?.close()
      events = null
    }
  }

  return { state, refresh, start }
}

export type PlaybackMonitor = ReturnType<typeof createPlaybackMonitor>
