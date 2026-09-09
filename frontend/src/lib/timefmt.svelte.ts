// Shared time-format state — toggles between seconds and milliseconds.
// Click any time value in a benchmark table to flip all times on the page.
let _timeUnit: 's' | 'ms' = $state('s')

export function timeUnit(): 's' | 'ms' {
  return _timeUnit
}

export function toggleTimeUnit() {
  _timeUnit = _timeUnit === 's' ? 'ms' : 's'
}

export function fmtTime(sec: number | undefined): string {
  if (!sec || sec <= 0) return '—'
  if (_timeUnit === 'ms') {
    return `${(sec * 1000).toFixed(0)}ms`
  }
  return `${sec.toFixed(2)}s`
}
