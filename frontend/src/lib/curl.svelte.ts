// Shared state for the base URL used in generated curl commands.
// Persists to localStorage so the user's choice survives reloads.
// Defaults to window.location.origin (however they accessed the UI).
//
// Uses a reactive object pattern for Svelte 5 runes in .svelte.ts files:
// components read `curlState.baseUrl` which is reactive.

const STORAGE_KEY = 'camspeak_curl_base_url'

function getInitial() {
  try {
    const saved = localStorage.getItem(STORAGE_KEY)
    if (saved) return saved
  } catch {}
  return typeof window !== 'undefined' ? window.location.origin : ''
}

export const curlState = $state({ baseUrl: getInitial() })

export function setCurlBaseUrl(url) {
  curlState.baseUrl = url
  try { localStorage.setItem(STORAGE_KEY, url) } catch {}
}

export function resetCurlBaseUrl() {
  const origin = typeof window !== 'undefined' ? window.location.origin : ''
  setCurlBaseUrl(origin)
}

// Build a curl command string for a given API call.
export function buildCurl(method, path, body) {
  const url = `${curlState.baseUrl}${path}`
  const parts = [`curl -X ${method} '${url}'`]
  if (body && Object.keys(body).length > 0) {
    parts.push(`  -H 'Content-Type: application/json'`)
    parts.push(`  -d '${JSON.stringify(body)}'`)
  }
  return parts.join(' \\\n  ')
}

// Copy text to clipboard, returns true on success.
export async function copyToClipboard(text) {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}
