export interface TimingStage {
  stage: string
  label: string
  duration?: number
}

/** Sum consecutive known stages; never present a partial sum as a complete total. */
export function accumulatedTimings(steps: TimingStage[]): (number | undefined)[] {
  let total = 0
  let known = true
  return steps.map(step => {
    if (step.duration === undefined || !Number.isFinite(step.duration) || step.duration < 0) known = false
    if (!known) return undefined
    total += step.duration!
    return total
  })
}
