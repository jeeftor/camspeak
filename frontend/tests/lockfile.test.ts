import { expect, test } from 'bun:test'

test('locked package downloads use the public npm registry', async () => {
  const lockfile = await Bun.file(new URL('../bun.lock', import.meta.url)).text()
  const urls = [...lockfile.matchAll(/https?:\/\/[^"\s]+/g)]

  expect(urls.length).toBeGreaterThan(0)
  for (const [url] of urls) {
    expect(new URL(url).origin).toBe('https://registry.npmjs.org')
  }
})
