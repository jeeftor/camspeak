const pending = new Map<string, Set<AbortController>>()

/** Keep local upload preparation under the same Stop controls as playback. */
export function registerAudioPreparation(camera: string, controller: AbortController): () => void {
  const controllers = pending.get(camera) ?? new Set<AbortController>()
  controllers.add(controller)
  pending.set(camera, controllers)
  return () => {
    controllers.delete(controller)
    if (!controllers.size) pending.delete(camera)
  }
}

/** Cancel this browser's pending uploads before issuing a camera Stop request. */
export function cancelAudioPreparation(camera?: string): void {
  for (const [name, controllers] of pending) {
    if (camera && camera !== name) continue
    for (const controller of controllers) controller.abort()
  }
}
