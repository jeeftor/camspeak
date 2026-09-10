import { afterAll, expect, test } from 'bun:test'
import { JSDOM } from 'jsdom'

const dom = new JSDOM('<!doctype html>')
const originalWindow = globalThis.window
Object.defineProperty(globalThis, 'window', { value: dom.window, configurable: true })
const { renderMarkdown } = await import('../src/lib/markdown')

afterAll(() => {
  dom.window.close()
  Object.defineProperty(globalThis, 'window', { value: originalWindow, configurable: true })
})

test('untrusted model Markdown cannot introduce executable HTML or links', () => {
  const html = renderMarkdown('<img src="x" onerror="alert(1)"><script>alert(2)</script>\n\n[bad](javascript:alert%283%29)\n\n<svg onload="alert(4)"></svg>')
  const result = new JSDOM(html)
  expect(result.window.document.querySelector('script, svg, [onerror], [onload]')).toBeNull()
  expect(result.window.document.querySelector('a[href^="javascript:"]')).toBeNull()
  result.window.close()
})

test('ordinary Markdown formatting and safe links remain usable', () => {
  const html = renderMarkdown('**Camera ready**\n\n[Help](https://example.com/help)')
  expect(html).toContain('<strong>Camera ready</strong>')
  expect(html).toContain('href="https://example.com/help"')
})
