// Centralized API client. All fetch calls go through this.
// Add auth headers, retry logic, or error formatting here once.

import type {
  AppConfig,
  Camera,
  DetectCameraResponse,
  DiscoverResponse,
  FrigateTestResult,
  Go2rtcStreamsResponse,
  Go2rtcTestResult,
  Health,
  MQTTTopic,
  PingResponse,
  PlayReq,
  Preset,
  Rule,
  SaveCameraReq,
  SaveRuleReq,
  SaveTTSReq,
  SaveVisionPromptReq,
  SaveVisionReq,
  Settings,
  SpeakReq,
  TTSPreset,
  VisionConfig,
  VisionDescribeResult,
  VisionPrompt,
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

export const apiClient = {
  // --- Health ---
  health: () => api<Health>('/api/health'),

  // --- Cameras ---
  getCameras: () => api<Camera[]>('/api/cameras'),
  pingCamera: (name: string) =>
    api<PingResponse>(`/api/cameras/${encodeURIComponent(name)}/ping`, { method: 'POST' }),

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

  // --- Config: vision ---
  getVisionConfig: () => api<VisionConfig>('/api/config/vision'),
  saveVisionConfig: (cfg: SaveVisionReq) =>
    api('/api/config/vision', { method: 'PUT', body: JSON.stringify(cfg) }),
  testVisionConfig: () =>
    api<VisionTestResult>('/api/config/vision/test', { method: 'POST' }),

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
    api(`/api/config/airplay/${encodeURIComponent(name)}/toggle`, { method: 'POST' }),
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
  speak: (req: SpeakReq) => api('/api/speak', { method: 'POST', body: JSON.stringify(req) }),
  play: (req: PlayReq) => api('/api/play', { method: 'POST', body: JSON.stringify(req) }),
  playURL: (req: { camera: string; url: string; gain: number }) =>
    api('/api/play-url', { method: 'POST', body: JSON.stringify(req) }),
  playStream: (req: { camera: string; url: string; gain: number }) =>
    api('/api/play-stream', { method: 'POST', body: JSON.stringify(req) }),
  stop: (camera?: string) =>
    api('/api/stop', { method: 'POST', body: JSON.stringify(camera ? { camera } : {}) }),
  stopAll: () => api('/api/stop', { method: 'POST' }),
  beep: (req: { camera: string }) =>
    api('/api/beep', { method: 'POST', body: JSON.stringify(req) }),
  broadcast: (req: { text: string; voice: string; gain: number }) =>
    api('/api/broadcast', { method: 'POST', body: JSON.stringify(req) }),

  // --- Library ---
  getPresets: () => api<Preset[]>('/api/library'),
  savePreset: (req: { name: string; text: string; category: string; voice: string }) =>
    api('/api/library', { method: 'POST', body: JSON.stringify(req) }),
  uploadPreset: (fd: FormData) => apiRaw('/api/library/upload', { method: 'POST', body: fd }),
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
  snapshot: (camera: string) =>
    apiRaw(`/api/snapshot/${encodeURIComponent(camera)}`),
  describe: (req: { camera: string; prompt: string }) =>
    api<VisionDescribeResult>('/api/describe', { method: 'POST', body: JSON.stringify(req) }),
  visionTest: (fd: FormData) =>
    apiRaw('/api/vision/test', { method: 'POST', body: fd }),
  visionTestJSON: (req: { image: string; prompt: string }) =>
    api<VisionDescribeResult>('/api/vision/test', { method: 'POST', body: JSON.stringify(req) }),
}
