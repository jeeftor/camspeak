// Consume one comparison; aborting also closes the response reader so the server
// can cancel its current model request instead of testing the remaining models.
export async function streamVisionComparison(
  body: object,
  signal: AbortSignal,
  onEvent: (event: any) => void,
): Promise<void> {
  signal.throwIfAborted()
  const response = await fetch('/api/vision/test-all/stream', {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body), signal,
  })
  if (!response.ok) throw new Error(`HTTP ${response.status}`)
  if (!response.body) throw new Error('The server returned no result stream')
  const reader = response.body.getReader()
  const cancel = () => { void reader.cancel().catch(() => {}) }
  signal.addEventListener('abort', cancel, { once: true })
  const decoder = new TextDecoder()
  let buffer = ''
  try {
    signal.throwIfAborted()
    while (true) {
      const { done, value } = await reader.read()
      signal.throwIfAborted()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop() ?? ''
      for (const line of lines) {
        signal.throwIfAborted()
        if (!line.startsWith('data:')) continue
        let event
        try { event = JSON.parse(line.slice(5).trim()) } catch { continue }
        onEvent(event)
      }
    }
  } finally {
    signal.removeEventListener('abort', cancel)
    await reader.cancel().catch(() => {})
    reader.releaseLock()
  }
}
