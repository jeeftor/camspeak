import { apiClient } from './api'
import type { UploadProgress } from './api'
import type { AudioDraft } from './audio-draft'
import { registerAudioPreparation } from './audio-preparation'

/** Serialize volume updates and restore the last confirmed value on failure. */
export async function saveCameraGain(camera: string, draft: AudioDraft) {
  if (draft.gainSaving) return
  draft.gainSaving = true
  try {
    while (draft.gain !== draft.savedGain) {
      const requested = draft.gain
      await apiClient.setVolume(camera, requested)
      draft.savedGain = requested
    }
  } catch (cause) {
    draft.gain = draft.savedGain
    throw cause
  } finally { draft.gainSaving = false }
}

/** Upload and convert fully before starting camera playback. */
export async function uploadAudioToCamera(
  camera: string,
  file: File,
  onProgress: (progress: UploadProgress) => void,
  signal: AbortSignal,
) {
  signal.throwIfAborted()
  const controller = new AbortController()
  const abort = () => controller.abort()
  signal.addEventListener('abort', abort, { once: true })
  const unregister = registerAudioPreparation(camera, controller)
  try {
    // LAN deployments commonly use HTTP, where crypto.randomUUID is unavailable.
    const name = `drop_${Date.now()}_${Math.random().toString(36).slice(2, 10)}`
    const job = await apiClient.uploadAndWait(file, name, 'drops', onProgress, controller.signal)
    controller.signal.throwIfAborted()
    onProgress({ step: 'Sending your audio', percent: 100, label: 'Sending your audio' })
    await apiClient.play({ camera, preset: job.name, category: job.category })
  } finally {
    signal.removeEventListener('abort', abort)
    unregister()
  }
}
