import { expect, test } from 'bun:test'
import { buildCurlCommand } from '../src/lib/curl-command'

test('copied curl preserves apostrophes and shell-looking text as literal arguments', () => {
  const body = { text: "Don't ring; ' $(printf injected) `printf bad`\nnext line", category: "owner's alerts" }
  const url = "https://example.com/owner's-camera"
  const command = buildCurlCommand(url, 'POST', '/api/speak', body)
  // Substitute a local argument printer: this never invokes curl or the network.
  const result = Bun.spawnSync(['sh', '-c', 'curl() { printf "%s\\n" "$@"; };\n' + command])
  expect(result.exitCode).toBe(0)
  const args = result.stdout.toString().trimEnd().split('\n')
  expect(args).toEqual(['-X', 'POST', url + '/api/speak', '-H', 'Content-Type: application/json', '-d', JSON.stringify(body)])
})
