// Shared vision-capable model filter.
// Mirrors the backend's `isVisionCapableModel` in internal/api/vision.go.
// Keep this in sync with the Go implementation so the frontend never shows
// models the backend would reject (or vice versa).

const VISION_KEYWORDS = [
  'vision',
  'llava',
  'moondream',
  'cogvlm',
  'internvl',
  'pixtral',
]

const VISION_SEPARATORS = ['-vl-', '-vl_', '-vl.', '_vl-', '_vl_']

export function isVisionCapableModel(id: string): boolean {
  const lower = id.toLowerCase()
  if (VISION_KEYWORDS.some((kw) => lower.includes(kw))) return true
  if (VISION_SEPARATORS.some((sep) => lower.includes(sep))) return true
  if (lower.endsWith('-vl') || lower.endsWith('_vl')) return true
  if (lower.includes('minicpm-v') || lower.includes('minicpm_v')) return true
  return false
}
