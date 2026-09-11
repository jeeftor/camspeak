// Centralized API client. All fetch calls go through this.
// Add auth headers, retry logic, or error formatting here once.

import { cancelAudioPreparation } from './audio-preparation'

import type {
  AppConfig,
  BroadcastResponse,
  Camera,
  CameraSummary,
  CameraInfo,
  AnnounceResponse,
  DescribeResponse,
  DescribeJob,
  DescribeRequest,
  DetectCameraResponse,
  DiscoverResponse,
  FrigateTestResult,
  Go2rtcStreamsResponse,
  Go2rtcTestResult,
  Health,
  PingResponse,
  PlaybackState,
  PlayReq,
  PlayResponse,
  Preset,
  PresetAnalysis,
  SaveCameraReq,
  SavePresetResponse,
  SaveTTSReq,
  SaveVisionPromptReq,
  SaveVisionReq,
  Settings,
  SnapshotBenchmarkResponse,
  SpeakReq,
  StreamInfo,
  SpeakResponse,
  TTSPreset,
  UploadJob,
  UploadJobAccepted,
  TTSTestResult,
  VisionConfig,
  VisionPrompt,
  VisionTestResponse,
  VisionTestResult,
} from './types'

// truncateError limits error messages to a reasonable length so that
// HTML error pages (from cameras, proxies, etc.) don't flood the UI.
function truncateError(text: string): string {
  // Try to extract JSON { "message": "..." } first
  try {
    const parsed = JSON.parse(text)
    if (parsed.message) text = String(parsed.message)
    else if (parsed.error) text = String(parsed.error)
  } catch { /* not JSON — fall through */ }
  if (/<(?:!doctype\s+html|html|head|body)\b/i.test(text)) {
    return 'Your gateway returned a web page instead of an API response. Check your connection or sign in again.'
  }
  const trimmed = text.trim()
  if (trimmed.length > 300) return trimmed.slice(0, 300) + '…'
  return trimmed
}

class HTTPError extends Error {
  constructor(readonly status: number, body: string) {
    const detail = [408, 504, 524].includes(status)
      ? 'The gateway timed out. Audio may still be running; check Camera Output before trying again.'
      : truncateError(body) || 'The request failed.'
    super(`HTTP ${status}: ${detail}`)
  }
}

async function api<T>(path: string, opts?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...opts?.headers },
    ...opts,
  })
  if (!res.ok) throw new HTTPError(res.status, await res.text())
  if (res.status === 204) return undefined as T
  const body = await res.text()
  try { return JSON.parse(body) as T }
  catch { throw new HTTPError(res.status, body) }
}

async function apiRaw(path: string, opts?: RequestInit): Promise<Response> {
  const res = await fetch(path, opts)
  if (!res.ok) throw new HTTPError(res.status, await res.text())
  return res
}

// uploadWithProgress sends a FormData POST via XHR so we get upload progress
// events (fetch doesn't support upload progress). Returns the parsed JSON
// response body. onProgress is called with 0–100 during the upload phase.
function uploadWithProgress<T>(
  path: string,
  fd: FormData,
  onProgress?: (percent: number) => void,
  signal?: AbortSignal,
): Promise<T> {
  return new Promise((resolve, reject) => {
    signal?.throwIfAborted()
    const xhr = new XMLHttpRequest()
    const abort = () => xhr.abort()
    signal?.addEventListener('abort', abort, { once: true })
    xhr.onloadend = () => signal?.removeEventListener('abort', abort)
    xhr.onabort = () => reject(new DOMException('Upload canceled', 'AbortError'))
    xhr.open('POST', path)

    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && onProgress) {
        onProgress((e.loaded / e.total) * 100)
      }
    }

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        try {
          resolve(xhr.status === 204 ? (undefined as T) : JSON.parse(xhr.responseText))
        } catch {
          reject(new Error('Invalid JSON response'))
        }
      } else {
        reject(new HTTPError(xhr.status, xhr.responseText))
      }
    }

    xhr.onerror = () => reject(new Error('Network error'))
    xhr.send(fd)
  })
}

/** Keep every Describe request shorter than the WAN proxy's request timeout. */
async function describeRequest(path: string, opts: RequestInit, signal?: AbortSignal): Promise<DescribeJob> {
  signal?.throwIfAborted()
  const controller = new AbortController()
  const abort = () => controller.abort(signal?.reason)
  signal?.addEventListener('abort', abort, { once: true })
  const timer = setTimeout(() => controller.abort(new DOMException('Status request timed out', 'TimeoutError')), 10_000)
  try {
    const job = await api<DescribeJob>(path, { ...opts, signal: controller.signal, cache: 'no-store' })
    if (!job.id || !['running', 'done', 'error', 'canceled'].includes(job.status) || !job.result) {
      throw new Error('The server returned an invalid Describe job')
    }
    if (job.error) job.error = truncateError(job.error)
    return job
  } finally {
    clearTimeout(timer)
    signal?.removeEventListener('abort', abort)
  }
}

function waitForDescribePoll(signal?: AbortSignal): Promise<void> {
  signal?.throwIfAborted()
  return new Promise((resolve, reject) => {
    const abort = () => { clearTimeout(timer); reject(signal?.reason) }
    const timer = setTimeout(() => { signal?.removeEventListener('abort', abort); resolve() }, 500)
    signal?.addEventListener('abort', abort, { once: true })
  })
}

/** Start once; only status reads are retried, so a proxy error cannot replay audio. */
async function describeAndWait(
  req: DescribeRequest,
  onProgress?: (job: DescribeJob) => void,
  signal?: AbortSignal,
  startPath = '/api/describe/jobs',
  statusPath = '/api/describe/jobs',
): Promise<DescribeJob> {
  const operation = startPath === '/api/describe/jobs' ? 'Describe' : 'Speaker benchmark'
  let job: DescribeJob
  try {
    job = await describeRequest(startPath, { method: 'POST', body: JSON.stringify(req) }, startPath === '/api/describe/jobs' ? signal : undefined)
  } catch (cause) {
    signal?.throwIfAborted()
    throw new Error(`${cause instanceof Error ? cause.message : String(cause)} ${operation} was not retried. Check Camera Output before trying again.`)
  }
  const id = job.id
  let failures = 0
  const started = Date.now()
  for (;;) {
    onProgress?.(job)
    signal?.throwIfAborted()
    if (job.status !== 'running') return job
    await waitForDescribePoll(signal)
    try {
      if (Date.now() - started > 15 * 60_000) throw new Error(`${operation} status monitoring expired`)
      job = await describeRequest(`${statusPath}/${encodeURIComponent(id)}`, {}, signal)
      if (job.id !== id) throw new Error('The server returned a different Describe job')
      failures = 0
    } catch (cause) {
      signal?.throwIfAborted()
      failures++
      if (failures <= 3 && (!(cause instanceof HTTPError) || cause.status >= 500 || [408, 429].includes(cause.status))) continue
      throw new Error(`Cannot read ${operation} progress. ${cause instanceof Error ? cause.message : String(cause)} Audio may still be running; use Stop or check Camera Output before trying again.`)
    }
  }
}

export interface UploadProgress {
  step: string
  percent: number
  label: string
}

/** Wait for conversion before callers use the newly uploaded preset. */
async function uploadAndWait(
  file: File,
  name: string,
  category: string,
  onProgress?: (progress: UploadProgress) => void,
  signal?: AbortSignal,
): Promise<UploadJob> {
  const fd = new FormData()
  fd.append('file', file)
  fd.append('name', name)
  fd.append('category', category)
  const accepted = await uploadWithProgress<UploadJobAccepted>('/api/library/upload', fd,
    (percent) => onProgress?.({ step: 'uploading', percent, label: 'Uploading' }), signal)
  if (!accepted.job_id) throw new Error('Upload response did not include a conversion job')
  for (;;) {
    signal?.throwIfAborted()
    const job = await api<UploadJob>(`/api/library/upload/jobs/${encodeURIComponent(accepted.job_id)}`, { signal })
    if (job.status === 'error') throw new Error(job.error || 'Audio conversion failed')
    onProgress?.({ step: job.status, percent: Math.max(0, job.percent), label: job.step || 'Converting' })
    signal?.throwIfAborted()
    if (job.status === 'done') return job
    await new Promise<void>((resolve, reject) => {
      const abort = () => { clearTimeout(timer); reject(new DOMException('Upload canceled', 'AbortError')) }
      const timer = setTimeout(() => { signal?.removeEventListener('abort', abort); resolve() }, 500)
      signal?.addEventListener('abort', abort, { once: true })
    })
  }
}

export const apiClient = {
  // --- Health ---
  health: () => api<Health>('/api/health'),

  // --- Cameras ---
  getCameras: () => api<CameraSummary[]>('/api/cameras'),
  pingCamera: (name: string) =>
    api<PingResponse>(`/api/cameras/${encodeURIComponent(name)}/ping`, { method: 'POST' }),
  getCameraInfo: (name: string) =>
    api<CameraInfo>(`/api/cameras/${encodeURIComponent(name)}/info`),

  // --- Config: general ---
  getConfig: () => api<AppConfig>('/api/config'),
  benchmarkTTS: (request: { preset: string; mode: 'buffered' | 'streaming'; text: string; voice: string; sample_rate: number; channels: number }, signal?: AbortSignal) =>
    api<{ mode: string; first_byte_ms: number; total_ms: number; bytes: number; duration: number; audio: string }>('/api/config/tts/benchmark', { method: 'POST', body: JSON.stringify(request), signal }),
  benchmarkSpeaker: (request: { camera: string; preset: string; text: string; voice: string; sample_rate: number; channels: number; confirm_playback: boolean; streaming_first: boolean }, progress: (job: DescribeJob) => void, signal?: AbortSignal) =>
    describeAndWait(request, progress, signal, '/api/config/tts/benchmark/speaker', '/api/config/tts/benchmark/jobs'),
  cancelSpeakerBenchmark: (id: string) => api(`/api/config/tts/benchmark/jobs/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  getSettings: () => api<Settings>('/api/config/settings'),
  saveSettings: (settings: Partial<Settings>) =>
    api('/api/config/settings', { method: 'PUT', body: JSON.stringify(settings) }),
  testSettingsURL: (url: string, type: 'frigate' | 'go2rtc' = 'frigate') =>
    api<FrigateTestResult | Go2rtcTestResult>('/api/config/settings/test', {
      method: 'POST',
      body: JSON.stringify({ url, type }),
    }),

  // --- Config: cameras ---
  listCamerasConfig: () => api<Camera[]>('/api/config/cameras'),
  saveCamera: (cam: SaveCameraReq) =>
    api('/api/config/cameras', { method: 'POST', body: JSON.stringify(cam) }),
  setVolume: (camera: string, gain: number) =>
    api(`/api/cameras/${encodeURIComponent(camera)}/volume`, {
      method: 'PUT',
      body: JSON.stringify({ gain }),
    }),
  deleteCamera: (name: string) =>
    api(`/api/config/cameras/${encodeURIComponent(name)}`, { method: 'DELETE' }),
  detectCamera: (ip: string, user?: string, pass?: string) =>
    api<DetectCameraResponse>('/api/config/cameras/detect', {
      method: 'POST',
      body: JSON.stringify({ ip, user: user ?? '', pass: pass ?? '' }),
    }),
  discoverCameras: () =>
    api<DiscoverResponse>('/api/config/cameras/discover', { method: 'POST' }),
  reorderCameras: (cameras: string[]) =>
    api('/api/config/cameras/reorder', { method: 'POST', body: JSON.stringify({ cameras }) }),

  // --- Config: TTS ---
  listTTSPresets: () => api<{ presets: TTSPreset[]; active: { url: string } | null }>('/api/config/tts'),
  saveTTSPreset: (preset: SaveTTSReq) =>
    api('/api/config/tts', { method: 'POST', body: JSON.stringify(preset) }),
  activateTTSPreset: (name: string) =>
    api(`/api/config/tts/${encodeURIComponent(name)}/activate`, { method: 'POST' }),
  deleteTTSPreset: (name: string) =>
    api(`/api/config/tts/${encodeURIComponent(name)}`, { method: 'DELETE' }),
  testTTSConfig: (url: string, apiKey: string, model?: string) =>
    api<TTSTestResult>('/api/config/tts/test', { method: 'POST', body: JSON.stringify({ url, api_key: apiKey, model }) }),

  // --- Config: vision ---
  getVisionConfig: () => api<VisionConfig>('/api/config/vision'),
  saveVisionConfig: (cfg: SaveVisionReq) =>
    api('/api/config/vision', { method: 'PUT', body: JSON.stringify(cfg) }),
  testVisionConfig: (url: string, apiKey: string) =>
    api<VisionTestResult>('/api/config/vision/test', { method: 'POST', body: JSON.stringify({ url, api_key: apiKey }) }),

  // --- Config: vision prompts ---
  listVisionPrompts: () => api<VisionPrompt[]>('/api/config/vision-prompts'),
  saveVisionPrompt: (prompt: SaveVisionPromptReq) =>
    api('/api/config/vision-prompts', { method: 'POST', body: JSON.stringify(prompt) }),
  deleteVisionPrompt: (name: string) =>
    api(`/api/config/vision-prompts/${encodeURIComponent(name)}`, { method: 'DELETE' }),

  // --- Config: airplay ---
  getAirPlayConfig: () =>
    api<{ enabled: boolean; base_port: number; prime_silence_ms: number; model: string; gain: number; per_camera: unknown[] }>('/api/config/airplay'),
  toggleAirPlay: (name: string) =>
    api(`/api/config/airplay/${encodeURIComponent(name)}/toggle`, { method: 'PATCH' }),
  saveAirPlayConfig: (cfg: { enabled: boolean; base_port: number; prime_silence_ms: number; model: string; gain: number }) =>
    api('/api/config/airplay', { method: 'PUT', body: JSON.stringify(cfg) }),

  // --- Config: go2rtc ---
  getGo2rtcStreams: () => api<Go2rtcStreamsResponse>('/api/config/go2rtc/streams'),

  // --- Voices ---
  getVoices: () => api<string[]>('/api/voices'),

  // --- Speak / Play / Stop ---
  speak: (req: SpeakReq) =>
    api<SpeakResponse>('/api/speak', { method: 'POST', body: JSON.stringify(req) }),
  play: (req: PlayReq) =>
    api<PlayResponse>('/api/play', { method: 'POST', body: JSON.stringify(req) }),
  playURL: (req: { camera: string; url: string; gain?: number }) =>
    api('/api/play-url', { method: 'POST', body: JSON.stringify(req) }),
  playStream: (req: { camera: string; url: string; gain?: number }) =>
    api('/api/play-stream', { method: 'POST', body: JSON.stringify(req) }),
  stop: (camera?: string) => {
    cancelAudioPreparation(camera)
    return api('/api/stop', { method: 'POST', body: JSON.stringify(camera ? { camera } : {}) })
  },
  stopAll: () => {
    cancelAudioPreparation()
    return api('/api/stop', { method: 'POST' })
  },
  pause: (camera?: string) =>
    api('/api/pause', { method: 'POST', body: JSON.stringify(camera ? { camera } : {}) }),
  resume: (camera?: string) =>
    api('/api/resume', { method: 'POST', body: JSON.stringify(camera ? { camera } : {}) }),
  getPlayback: () => api<Record<string, PlaybackState>>('/api/playback'),
  beep: (req: { camera: string }) =>
    api('/api/beep', { method: 'POST', body: JSON.stringify(req) }),
  broadcast: (req: { text?: string; preset?: string; category?: string; voice?: string; gain?: number }) =>
    api<BroadcastResponse>('/api/broadcast', { method: 'POST', body: JSON.stringify(req) }),

  // --- Library ---
  getPresets: () => api<Preset[]>('/api/library'),
  uploadAndWait,
  savePreset: (req: { name: string; text?: string; url?: string; category: string; voice?: string }) =>
    api<SavePresetResponse>('/api/library', { method: 'POST', body: JSON.stringify(req) }),
  uploadPreset: (fd: FormData) => apiRaw('/api/library/upload', { method: 'POST', body: fd }),
  uploadPresetWithProgress: (fd: FormData, onProgress?: (percent: number) => void) =>
    uploadWithProgress<UploadJobAccepted>('/api/library/upload', fd, onProgress),
  getUploadJob: (id: string) => api<UploadJob>(`/api/library/upload/jobs/${encodeURIComponent(id)}`),
  deletePreset: (category: string, name: string) =>
    api(`/api/library/${encodeURIComponent(category)}/${encodeURIComponent(name)}`, { method: 'DELETE' }),
  renamePreset: (oldCategory: string, oldName: string, req: { name: string; category: string }) =>
    api(`/api/library/${encodeURIComponent(oldCategory)}/${encodeURIComponent(oldName)}`, {
      method: 'PATCH',
      body: JSON.stringify(req),
    }),
  analyzePreset: (category: string, name: string) =>
    api<PresetAnalysis>(`/api/library/${encodeURIComponent(category)}/${encodeURIComponent(name)}/analyze`),
  setPresetGain: (category: string, name: string, gain: number) =>
    api(`/api/library/${encodeURIComponent(category)}/${encodeURIComponent(name)}/gain`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ gain }),
    }),
  ttsPreview: (req: { text: string; voice: string }) =>
    apiRaw('/api/tts/preview', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    }),

  // --- Vision ---
  snapshot: (camera: string, stream?: string, width?: number, signal?: AbortSignal, method?: string) => {
    const params = new URLSearchParams()
    if (stream) params.set('stream', stream)
    if (width) params.set('width', String(width))
    if (method) params.set('method', method)
    const qs = params.toString()
    return apiRaw(`/api/snapshot/${encodeURIComponent(camera)}${qs ? '?' + qs : ''}`, { signal })
  },
  snapshotBenchmark: (camera: string, vision = false, prompt = '') => {
    const params = new URLSearchParams()
    if (vision) params.set('vision', 'true')
    if (prompt) params.set('prompt', prompt)
    const qs = params.toString()
    return api<SnapshotBenchmarkResponse>(`/api/snapshot/${encodeURIComponent(camera)}/benchmark${qs ? '?' + qs : ''}`)
  },
  streams: () =>
    api<{ status: string; streams: StreamInfo[] }>('/api/streams'),
  describe: (req: DescribeRequest) =>
    api<DescribeResponse>('/api/describe', { method: 'POST', body: JSON.stringify(req) }),
  describeAndWait,
  announce: (req: { source_camera: string; target_camera: string; prompt?: string; voice?: string; gain?: number }) =>
    api<AnnounceResponse>('/api/announce', { method: 'POST', body: JSON.stringify(req) }),
  visionTest: (fd: FormData) =>
    apiRaw('/api/vision/test', { method: 'POST', body: fd }),
  visionTestJSON: (req: { image?: string; camera?: string; stream?: string; prompt: string; model?: string }) =>
    api<VisionTestResponse>('/api/vision/test', { method: 'POST', body: JSON.stringify(req) }),
}
