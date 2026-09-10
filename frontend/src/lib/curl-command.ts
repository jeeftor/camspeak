/** Quote one literal POSIX shell argument, including embedded apostrophes. */
function shellQuote(value: string): string {
  return "'" + value.replaceAll("'", "'\\''") + "'"
}

/** Build a copyable request without interpreting user text as shell syntax. */
export function buildCurlCommand(baseURL: string, method: string, path: string, body?: Record<string, unknown>): string {
  const parts = [`curl -X ${shellQuote(method)} ${shellQuote(`${baseURL}${path}`)}`]
  if (body && Object.keys(body).length > 0) {
    parts.push("  -H 'Content-Type: application/json'")
    parts.push(`  -d ${shellQuote(JSON.stringify(body))}`)
  }
  return parts.join(' \\\n  ')
}
