import { afterEach, expect, mock, test } from 'bun:test'
import { streamVisionComparison } from '../src/lib/vision-comparison'

const originalFetch = globalThis.fetch
afterEach(() => { globalThis.fetch = originalFetch })

test('comparison passes abort signal and cancellation closes a pending reader', async () => {
  const controller = new AbortController()
  const canceled = mock(() => {})
  const events = mock(() => {})
  const response = new Response(new ReadableStream({ cancel: canceled }))
  globalThis.fetch = mock(async (_input, options) => {
    expect(options?.signal).toBe(controller.signal)
    return response
  }) as unknown as typeof fetch
  const running = streamVisionComparison({}, controller.signal, events)
  await Promise.resolve()
  controller.abort()
  await expect(running).rejects.toMatchObject({ name: 'AbortError' })
  expect(canceled).toHaveBeenCalledTimes(1)
  expect(response.body?.locked).toBe(false)
  expect(events).not.toHaveBeenCalled()
})

test('aborting after one event suppresses already buffered stale events', async () => {
  const controller = new AbortController()
  const events: unknown[] = []
  globalThis.fetch = mock(async () => new Response(
    'data: {"type":"models","models":["vision"]}\n\ndata: {"type":"done"}\n\n',
  )) as unknown as typeof fetch
  await expect(streamVisionComparison({}, controller.signal, event => {
    events.push(event)
    controller.abort()
  })).rejects.toMatchObject({ name: 'AbortError' })
  expect(events).toEqual([{ type: 'models', models: ['vision'] }])
})

test('comparison parses split chunks and releases the reader on completion', async () => {
  const encoder = new TextEncoder()
  const response = new Response(new ReadableStream({ start(controller) {
    controller.enqueue(encoder.encode('data: {"type":"mo'))
    controller.enqueue(encoder.encode('dels"}\n\ndata: {bad json}\n\ndata: {"type":"done"}\n\n'))
    controller.close()
  } }))
  globalThis.fetch = mock(async () => response) as unknown as typeof fetch
  const events: unknown[] = []
  await streamVisionComparison({}, new AbortController().signal, event => events.push(event))
  expect(events).toEqual([{ type: 'models' }, { type: 'done' }])
  expect(response.body?.locked).toBe(false)
})
