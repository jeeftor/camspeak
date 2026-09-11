import { expect, test } from 'bun:test'
import { accumulatedTimings } from '../src/lib/timing-flow'

const stages = (...durations: (number | undefined)[]) => durations.map((duration, index) => ({ stage: String(index), label: String(index), duration }))

test('stage totals follow pipeline order and retain zero duration', () => {
  expect(accumulatedTimings(stages(279, 1800, 872, 0, 92))).toEqual([279, 2079, 2951, 2951, 3043])
})

test('unknown or invalid stages do not produce misleading cumulative totals', () => {
  for (const missing of [undefined, NaN, Infinity, -1]) {
    expect(accumulatedTimings(stages(279, missing, 872))).toEqual([279, undefined, undefined])
  }
})
