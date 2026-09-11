/** Shared labels and measurement boundaries; API timing keys remain unchanged. */
export const describeStages = [
  { stage: 'snapshot', label: 'Snapshot', active: 'Capturing snapshot', timing: 'snapshot_ms', description: 'Fetch the camera image used for vision analysis.' },
  { stage: 'vision', label: 'Vision', active: 'Describing image', timing: 'vision_ms', description: 'Send the image and prompt to the vision service and wait for its complete description.' },
  { stage: 'tts', label: 'TTS', active: 'Generating speech', timing: 'tts_ms', description: 'Text to speech: generate and receive the spoken audio.' },
  { stage: 'transcode', label: 'Convert (μ-law)', active: 'Converting audio', timing: 'transcode_ms', description: 'Convert audio to camera-compatible G.711 μ-law, including configured speaker warm-up silence.' },
  { stage: 'connecting', label: 'Connect', active: 'Connecting to speaker', timing: 'send_open_ms', description: 'Set up the camera speaker connection. This is not confirmation of audible sound.' },
  { stage: 'playing', label: 'Playback', active: 'Sending camera audio', timing: 'send_ms', description: 'Send audio to the camera. Camera buffering and audible output are not measured.' },
] as const

const extraStages = {
  ttft_ms: { label: 'First token (TTFT)', description: 'Request start to the first answer-text chunk. Includes network time, server queueing, possible model loading, image/prompt processing, and any reasoning before the answer. Model loading is not measured separately.' },
  gen_ms: { label: 'Generate answer', description: 'First answer-text chunk to completion of the response stream.' },
  ttfs_ms: { label: 'First audio sent', description: 'Action start to the first audio sent to the camera, not the first audible sound.' },
  total_ms: { label: 'Total', description: 'Overall measured elapsed time, including work between stages.' },
  load_ms: { label: 'Load audio', description: 'Load the selected audio preset; this is not AI model loading.' },
  download_ms: { label: 'Download', description: 'Download the requested audio file.' },
  save_ms: { label: 'Save', description: 'Save the generated audio preset.' },
} as const

export function stageInfo(key: string): { label: string; description: string } {
  const canonical = key === 'snap_ms' ? 'snapshot_ms' : key === 'send_playback_ms' ? 'send_ms' : key
  const stage = describeStages.find(item => item.stage === canonical || item.timing === canonical)
  return stage ?? extraStages[canonical as keyof typeof extraStages]
    ?? { label: key.replace(/_ms$/, '').replace(/_/g, ' '), description: 'Additional duration reported by the server.' }
}
