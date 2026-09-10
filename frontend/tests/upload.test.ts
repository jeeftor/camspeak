import { afterEach, expect, test } from 'bun:test'
import { apiClient } from '../src/lib/api'

const originalFetch = globalThis.fetch
const originalXHR = globalThis.XMLHttpRequest
const jobs: Array<Record<string, unknown>> = []
let requestedJobs = 0

class UploadXHR {
  status = 202
  responseText = JSON.stringify({ job_id: 'fixture-job' })
  upload = { onprogress: null }
  onload?: () => void
  onloadend?: () => void
  onabort?: () => void
  open() {}
  send() { queueMicrotask(() => { this.onload?.(); this.onloadend?.() }) }
  abort() { this.onabort?.(); this.onloadend?.() }
}

function mockUpload(responses: Array<Record<string, unknown>>) {
  jobs.splice(0, jobs.length, ...responses)
  requestedJobs = 0
  globalThis.XMLHttpRequest = UploadXHR as unknown as typeof XMLHttpRequest
  globalThis.fetch = (async () => {
    requestedJobs++
    return Response.json(jobs.shift())
  }) as unknown as typeof fetch
}

afterEach(() => {
  globalThis.fetch = originalFetch
  globalThis.XMLHttpRequest = originalXHR
})

test('upload waits through conversion and retains category/name identity', async () => {
  mockUpload([
    { status: 'transcoding', percent: 20, step: 'Converting' },
    { status: 'done', percent: 100, name: 'chime', category: 'upstairs' },
  ])
  const progress: string[] = []
  const result = await apiClient.uploadAndWait(new File(['audio'], 'chime.wav'), 'chime', 'upstairs', p => progress.push(p.step))
  expect(requestedJobs).toBe(2)
  expect(progress).toEqual(['transcoding', 'done'])
  expect(result).toMatchObject({ status: 'done', category: 'upstairs', name: 'chime' })
})

test('conversion failure rejects instead of making playback eligible', async () => {
  mockUpload([{ status: 'error', error: 'Unsupported audio format' }])
  await expect(apiClient.uploadAndWait(new File(['bad'], 'bad.wav'), 'bad', 'uploads'))
    .rejects.toThrow('Unsupported audio format')
})

test('canceling while polling prevents further job requests', async () => {
  mockUpload([{ status: 'transcoding', percent: 0 }])
  const controller = new AbortController()
  const pending = apiClient.uploadAndWait(new File(['audio'], 'a.wav'), 'a', 'uploads', undefined, controller.signal)
  setTimeout(() => controller.abort(), 10)
  await expect(pending).rejects.toMatchObject({ name: 'AbortError' })
  expect(requestedJobs).toBe(1)
})
