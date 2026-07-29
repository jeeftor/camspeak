// Shared types matching the Go backend structs in internal/config/model.go
// and internal/library/store.go.

export type CameraType = 'hikvision' | 'reolink' | 'go2rtc' | 'onvif'

// Timing breakdown returned by several API endpoints. Keys are step names
// (e.g. "tts_ms", "transcode_ms", "send_ms") mapped to milliseconds.
export interface Timings {
  [key: string]: number
}

export interface Camera {
  name: string
  type: CameraType
  ip: string
  user: string
  pass: string
  channel: number
  stream: string
  enabled: boolean
  airplay_enabled: boolean
  airplay_name: string
  airplay_model: string
  gain: number
  vision_prompt: string
  note: string
}

export interface TTSConfig {
  url: string
  model: string
  default_voice: string
  api_key: string
}

export interface VisionConfig {
  url: string
  model: string
  api_key: string
  prompt: string
}

export interface MQTTConfig {
  broker: string
  user: string
  pass: string
}

export interface AirPlayConfig {
  enabled: boolean
  base_port: number
  prime_silence_ms: number
  model: string
  gain: number
}

export interface TTSPreset {
  name: string
  endpoint: string
  model: string
  api_key: string
  default_voice: string
  description: string
  is_active: boolean
}

export interface VisionPrompt {
  name: string
  prompt: string
  description: string
}

export interface Rule {
  id: number
  topic: string
  filter: Record<string, string>
  cameras: string[]
  preset: string
  text: string
  voice: string
  enabled: boolean
}

export interface Preset {
  name: string
  category: string
  text: string
  voice: string
  duration: number
  size: number
  created: string
  timings?: Timings
  total_ms?: number
}

export interface AppConfig {
  tts: TTSConfig
  vision: VisionConfig
  cameras: Record<string, Camera>
  mqtt: MQTTConfig
  airplay: AirPlayConfig
}

export interface Settings {
  frigate_url: string
  go2rtc_url: string
  advertise_ip: string
}

export interface Health {
  status: string
  version: string
}

// Request types

export interface SpeakReq {
  camera: string
  text: string
  voice: string
  gain: number
}

export interface PlayReq {
  camera: string
  preset: string
  gain: number
}

export interface SaveCameraReq {
  name: string
  type: CameraType
  ip: string
  user: string
  pass: string
  channel: number
  stream: string
  enabled: boolean
  vision_prompt: string
  airplay_name: string
  airplay_model: string
  gain?: number
}

export interface SaveTTSReq {
  name: string
  endpoint: string
  model: string
  default_voice: string
  api_key: string
  description: string
}

export interface SaveVisionReq {
  url: string
  model: string
  api_key: string
  prompt: string
}

export interface SaveRuleReq {
  topic: string
  filter: Record<string, string>
  cameras: string[]
  preset: string
  text: string
  voice: string
  enabled: boolean
}

export interface SaveVisionPromptReq {
  name: string
  prompt: string
  description: string
}

export interface MQTTTopic {
  topic: string
  count: number
  payload: string
  raw: string
  at: string
}

// Response types

export interface PingResponse {
  ok: boolean
}

export interface DetectCameraResponse {
  ip: string
  type: CameraType
  go2rtc_url: string
  note: string
}

export interface DiscoverResponse {
  discovered: number
}

export interface Go2rtcStreamsResponse {
  streams: string[]
  error: string
}

export interface VisionTestResult {
  ok: boolean
  models: number
}

export interface FrigateTestResult {
  ok: boolean
  data: { version?: string }
}

export interface Go2rtcTestResult {
  ok: boolean
  data: Record<string, unknown>
}

export interface VisionDescribeResult {
  description: string
  image?: string
  timings?: Timings
  total_ms?: number
}

// Responses from speak/play/describe endpoints. These endpoints return
// { status, ... } plus an optional timing breakdown.
export interface SpeakResponse {
  status?: string
  timings?: Timings
  total_ms?: number
}

export interface PlayResponse {
  status?: string
  timings?: Timings
  total_ms?: number
}

export interface DescribeResponse {
  status?: string
  description?: string
  image?: string
  timings?: Timings
  total_ms?: number
}

export interface VisionTestResponse {
  description?: string
  image?: string
  timings?: Timings
  total_ms?: number
}

export interface SavePresetResponse {
  name?: string
  category?: string
  text?: string
  voice?: string
  duration?: number
  size?: number
  created?: string
  timings?: Timings
  total_ms?: number
}
