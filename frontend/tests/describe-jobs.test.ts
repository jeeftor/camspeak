import { afterEach, expect, mock, spyOn, test } from 'bun:test'
import { apiClient } from '../src/lib/api'
import type { DescribeJob } from '../src/lib/types'

afterEach(() => mock.restore())

const running = { id: 'job-1', camera: 'front', status: 'running', stage: 'vision', elapsed_ms: 250,
  result: { timings: { snapshot_ms: 250 } } } as DescribeJob

test('Describe starts once and reports partial progress before the completed result', async () => {
  const fetch = spyOn(globalThis, 'fetch')
    .mockResolvedValueOnce(Response.json(running, { status: 202 }))
    .mockResolvedValueOnce(Response.json({ ...running, stage: 'tts', result: { description: 'A parcel.', timings: { snapshot_ms: 250, vision_ms: 300 } } }))
    .mockResolvedValueOnce(Response.json({ ...running, status: 'done', stage: 'done', result: { description: 'A parcel.', total_ms: 2000 } }))
  const progress: DescribeJob[] = []
  const job = await apiClient.describeAndWait({ camera: 'front' }, value => progress.push(value))
  expect(progress.map(value => value.stage)).toEqual(['vision', 'tts', 'done'])
  expect(progress[1].result.description).toBe('A parcel.')
  expect(job.result.total_ms).toBe(2000)
  expect(fetch.mock.calls.map(([url, options]) => [url, options?.method ?? 'GET'])).toEqual([
    ['/api/describe/jobs', 'POST'], ['/api/describe/jobs/job-1', 'GET'], ['/api/describe/jobs/job-1', 'GET'],
  ])
  expect(fetch.mock.calls.every(([, options]) => options?.signal instanceof AbortSignal)).toBe(true)
})

test('a transient proxy failure retries only the safe status read', async () => {
  const fetch = spyOn(globalThis, 'fetch')
    .mockResolvedValueOnce(Response.json(running, { status: 202 }))
    .mockResolvedValueOnce(new Response('<!DOCTYPE html><html>Cloudflare</html>', { status: 524 }))
    .mockResolvedValueOnce(Response.json({ ...running, status: 'done', stage: 'done' }))
  expect((await apiClient.describeAndWait({ camera: 'front' })).status).toBe('done')
  expect(fetch.mock.calls.filter(([, options]) => options?.method === 'POST')).toHaveLength(1)
})

test('an uncertain start is not retried and never exposes proxy HTML', async () => {
  const fetch = spyOn(globalThis, 'fetch').mockResolvedValue(new Response('<!DOCTYPE html><html>Cloudflare</html>', { status: 524 }))
  await expect(apiClient.describeAndWait({ camera: 'front' })).rejects.toThrow('HTTP 524')
  expect(fetch).toHaveBeenCalledTimes(1)
  const error = await apiClient.health().catch(error => error as Error)
  expect(error).toBeInstanceOf(Error)
  expect((error as Error).message).not.toContain('<!DOCTYPE')
})

test('Stop cancellation is a terminal job with its partial result retained', async () => {
  spyOn(globalThis, 'fetch')
    .mockResolvedValueOnce(Response.json(running))
    .mockResolvedValueOnce(Response.json({ ...running, status: 'canceled', stage: 'canceled', error: 'Describe canceled' }))
  const job = await apiClient.describeAndWait({ camera: 'front' })
  expect(job.status).toBe('canceled')
  expect(job.result.timings).toEqual({ snapshot_ms: 250 })
})

test('authentication pages returned with HTTP 200 have a readable error', async () => {
  spyOn(globalThis, 'fetch').mockResolvedValue(new Response('<html><body>Sign in</body></html>', { headers: { 'Content-Type': 'text/html' } }))
  await expect(apiClient.health()).rejects.toThrow('sign in')
})

test('leaving a view aborts polling without posting another operation', async () => {
  const fetch = spyOn(globalThis, 'fetch').mockResolvedValue(Response.json(running))
  const controller = new AbortController()
  const pending = apiClient.describeAndWait({ camera: 'front' }, () => controller.abort(), controller.signal)
  await expect(pending).rejects.toMatchObject({ name: 'AbortError' })
  expect(fetch).toHaveBeenCalledTimes(1)
})

test('persistent polling errors stop retrying and leave the outcome explicitly unknown', async () => {
  const fetch = spyOn(globalThis, 'fetch')
    .mockResolvedValueOnce(Response.json(running))
  for (let attempt = 0; attempt < 4; attempt++) {
    fetch.mockResolvedValueOnce(new Response('<html>Bad gateway</html>', { status: 502 }))
  }
  await expect(apiClient.describeAndWait({ camera: 'front' })).rejects.toThrow('Audio may still be running')
  expect(fetch).toHaveBeenCalledTimes(5)
  expect(fetch.mock.calls.filter(([, options]) => options?.method === 'POST')).toHaveLength(1)
})

test('a denied status read is not retried', async () => {
  const fetch = spyOn(globalThis, 'fetch')
    .mockResolvedValueOnce(Response.json(running))
    .mockResolvedValueOnce(Response.json({ message: 'Sign in again' }, { status: 401 }))
  await expect(apiClient.describeAndWait({ camera: 'front' })).rejects.toThrow('HTTP 401')
  expect(fetch).toHaveBeenCalledTimes(2)
})

test('job errors also sanitize nested upstream HTML while retaining completed timings', async () => {
  spyOn(globalThis, 'fetch').mockResolvedValue(Response.json({ ...running, status: 'error', stage: 'error', error: 'vision: <html>Bad gateway</html>' }))
  const job = await apiClient.describeAndWait({ camera: 'front' })
  expect(job.error).toContain('gateway')
  expect(job.error).not.toContain('<html>')
  expect(job.result.timings).toEqual(running.result.timings)
})
