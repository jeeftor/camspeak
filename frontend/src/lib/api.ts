// Centralized API client. All fetch calls go through this.
// Add auth headers, retry logic, or error formatting here once.

import type {
  AppConfig,
  Camera,
  CameraInfo,
  DescribeResponse,
  DetectCameraResponse,
  DiscoverResponse,
  FrigateTestResult,
  Go2rtcStreamsResponse,
  Go2rtcTestResult,
  Health,
  MQTTTopic,
  PingResponse,
  PlaybackState,
  PlayReq,
  PlayResponse,
  Preset,
  Rule,
  SaveCameraReq,
  SavePresetResponse,
  SaveRuleReq,
  SaveTTSReq,
  SaveVisionPromptReq,
  SaveVisionReq,
  Settings,
  SpeakReq,
  StreamInfo,
  SpeakResponse,
  TTSPreset,
  UploadJob,
  UploadJobAccepted,
  TTSTestResult,
  VisionConfig,
  VisionPrompt,
  VisionTestAllResponse,
  VisionTestResponse,
  VisionTestResult,
} from './types'

async function api<T>(path: string, opts?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { 'Content-Type': 'application/json', ...opts?.headers },
    ...opts,
  })
  if (!res.ok) throw new Error(await res.text())
  return res.status === 204 ? (undefined as T) : await res.json()
}

async function apiRaw(path: string, opts?: RequestInit): Promise<Response> {
  const res = await fetch(path, opts)
  if (!res.ok) throw new Error(await res.text())
  return res
}

// uploadWithProgress sends a FormData POST via XHR so we get upload progress
// events (fetch doesn't support upload progress). Returns the parsed JSON
// response body. onProgress is called with 0–100 during the upload phase.
function uploadWithProgress<T>(
  path: string,
  fd: FormData,
  onProgress?: (percent: number) => void,
): Promise<T> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
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
        reject(new Error(xhr.responseText || `HTTP ${xhr.status}`))
      }
    }

    xhr.onerror = () => reject(new Error('Network error'))
    xhr.send(fd)
  })
}

export const apiClient = {
  // --- Health ---
  health: () => api<Health>('/api/health'),

  // --- Cameras ---
  getCameras: () => api<Camera[]>('/api/cameras'),
  pingCamera: (name: string) =>
    api<PingResponse>(`/api/cameras/${encodeURIComponent(name)}/ping`, { method: 'POST' }),
  getCameraInfo: (name: string) =>
    api<CameraInfo>(`/api/cameras/${encodeURIComponent(name)}/info`),

  // --- Config: general ---
  getConfig: () => api<AppConfig>('/api/config'),
  getSettings: () => api<Settings>('/api/config/settings'),
  saveSettings: (settings: Partial<Settings>) =>
    api('/api/config/settings', { method: 'PUT', body: JSON.stringify(settings) }),
  testSettingsURL: (url: string) =>
    api<FrigateTestResult | Go2rtcTestResult>('/api/config/settings/test', {
      method: 'POST',
      body: JSON.stringify({ url }),
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
  toggleCamera: (name: string, enabled: boolean) =>
    api(`/api/config/cameras/${encodeURIComponent(name)}/toggle`, {
      method: 'POST',
      body: JSON.stringify({ enabled }),
    }),
  detectCamera: (ip: string) =>
    api<DetectCameraResponse>('/api/config/cameras/detect', {
      method: 'POST',
      body: JSON.stringify({ ip }),
    }),
  discoverCameras: () =>
    api<DiscoverResponse>('/api/config/cameras/discover', { method: 'POST' }),

  // --- Config: TTS ---
  listTTSPresets: () => api<{ presets: TTSPreset[]; active: { url: string } | null }>('/api/config/tts'),
  saveTTSPreset: (preset: SaveTTSReq) =>
    api('/api/config/tts', { method: 'POST', body: JSON.stringify(preset) }),
  activateTTSPreset: (name: string) =>
    api(`/api/config/tts/${encodeURIComponent(name)}/activate`, { method: 'POST' }),
  deleteTTSPreset: (name: string) =>
    api(`/api/config/tts/${encodeURIComponent(name)}`, { method: 'DELETE' }),
  testTTSConfig: (url: string, apiKey: string) =>
    api<TTSTestResult>('/api/config/tts/test', { method: 'POST', body: JSON.stringify({ url, api_key: apiKey }) }),

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
    api<{ enabled: boolean; base_port: number; prime_silence_ms: number; model: string; gain: number; per_camera: Record<string, unknown> }>('/api/config/airplay'),
  toggleAirPlay: (name: string) =>
    api(`/api/config/airplay/${encodeURIComponent(name)}/toggle`, { method: 'PATCH' }),
  saveAirPlayConfig: (cfg: { enabled: boolean; base_port: number; prime_silence_ms: number; model: string; gain: number }) =>
    api('/api/config/airplay', { method: 'PUT', body: JSON.stringify(cfg) }),

  // --- Config: go2rtc ---
  getGo2rtcStreams: () => api<Go2rtcStreamsResponse>('/api/config/go2rtc/streams'),

  // --- Config: rules ---
  listRules: () => api<Rule[]>('/api/config/rules'),
  saveRule: (rule: SaveRuleReq) =>
    api('/api/config/rules', { method: 'POST', body: JSON.stringify(rule) }),

  // --- MQTT ---
  getMQTTStatus: () => api<{ status: string; broker: string }>('/api/mqtt/status'),
  getMQTTTopics: () => api<MQTTTopic[]>('/api/mqtt/topics'),
  subscribeMQTT: (topic: string) =>
    api('/api/mqtt/subscribe', { method: 'POST', body: JSON.stringify({ topic }) }),

  // --- Voices ---
  getVoices: () => api<string[]>('/api/voices'),

  // --- Speak / Play / Stop ---
  speak: (req: SpeakReq) =>
    api<SpeakResponse>('/api/speak', { method: 'POST', body: JSON.stringify(req) }),
  play: (req: PlayReq) =>
    api<PlayResponse>('/api/play', { method: 'POST', body: JSON.stringify(req) }),
  playURL: (req: { camera: string; url: string; gain: number }) =>
    api('/api/play-url', { method: 'POST', body: JSON.stringify(req) }),
  playStream: (req: { camera: string; url: string; gain: number }) =>
    api('/api/play-stream', { method: 'POST', body: JSON.stringify(req) }),
  stop: (camera?: string) =>
    api('/api/stop', { method: 'POST', body: JSON.stringify(camera ? { camera } : {}) }),
  stopAll: () => api('/api/stop', { method: 'POST' }),
  pause: (camera?: string) =>
    api('/api/pause', { method: 'POST', body: JSON.stringify(camera ? { camera } : {}) }),
  resume: (camera?: string) =>
    api('/api/resume', { method: 'POST', body: JSON.stringify(camera ? { camera } : {}) }),
  getPlayback: () => api<Record<string, PlaybackState>>('/api/playback'),
  beep: (req: { camera: string }) =>
    api('/api/beep', { method: 'POST', body: JSON.stringify(req) }),
  broadcast: (req: { text: string; voice: string; gain: number }) =>
    api('/api/broadcast', { method: 'POST', body: JSON.stringify(req) }),

  // --- Library ---
  getPresets: () => api<Preset[]>('/api/library'),
  savePreset: (req: { name: string; text: string; category: string; voice: string }) =>
    api<SavePresetResponse>('/api/library', { method: 'POST', body: JSON.stringify(req) }),
  uploadPreset: (fd: FormData) => apiRaw('/api/library/upload', { method: 'POST', body: fd }),
  uploadPresetWithProgress: (fd: FormData, onProgress?: (percent: number) => void) =>
    uploadWithProgress('/api/library/upload', fd, onProgress),
  getUploadJob: (id: string) => api<UploadJob>(`/api/library/upload/jobs/${encodeURIComponent(id)}`),
  deletePreset: (category: string, name: string) =>
    api(`/api/library/${encodeURIComponent(category)}/${encodeURIComponent(name)}`, { method: 'DELETE' }),
  renamePreset: (oldCategory: string, oldName: string, req: { name: string; category: string }) =>
    api(`/api/library/${encodeURIComponent(oldCategory)}/${encodeURIComponent(oldName)}`, {
      method: 'PATCH',
      body: JSON.stringify(req),
    }),
  ttsPreview: (req: { text: string; voice: string }) =>
    apiRaw('/api/tts/preview', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    }),

  // --- Vision ---
  snapshot: (camera: string, stream?: string, width?: number) => {
    const params = new URLSearchParams()
    if (stream) params.set('stream', stream)
    if (width) params.set('width', String(width))
    const qs = params.toString()
    return apiRaw(`/api/snapshot/${encodeURIComponent(camera)}${qs ? '?' + qs : ''}`)
  },
  streams: () =>
    api<{ status: string; streams: StreamInfo[] }>('/api/streams'),
  describe: (req: { camera: string; prompt?: string; gain?: number }) =>
    api<DescribeResponse>('/api/describe', { method: 'POST', body: JSON.stringify(req) }),
  visionTest: (fd: FormData) =>
    apiRaw('/api/vision/test', { method: 'POST', body: fd }),
  visionTestJSON: (req: { image?: string; camera?: string; prompt: string }) =>
    api<VisionTestResponse>('/api/vision/test', { method: 'POST', body: JSON.stringify(req) }),
  visionTestAll: (req: { image?: string; camera?: string; prompt: string }) =>
    api<VisionTestAllResponse>('/api/vision/test-all', { method: 'POST', body: JSON.stringify(req) }),
}
