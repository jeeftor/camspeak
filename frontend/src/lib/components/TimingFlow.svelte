<script lang="ts">
  import { formatMs } from '$lib/utils'
  import { accumulatedTimings, type TimingStage } from '$lib/timing-flow'

  let { steps, label = 'Timing flow', activeStage = '', running = false, status = '',
    elapsedMs, firstAudioMs, totalMs }: {
    steps: TimingStage[]
    label?: string
    activeStage?: string
    running?: boolean
    status?: string
    elapsedMs?: number
    firstAudioMs?: number
    totalMs?: number
  } = $props()
  const accumulated = $derived(accumulatedTimings(steps))

  function valid(value: number | undefined): value is number {
    return value !== undefined && Number.isFinite(value) && value >= 0
  }
</script>

<div class="flex min-w-0 flex-col gap-2">
  {#if status || valid(elapsedMs)}
    <p role="status" class="flex flex-wrap justify-between gap-1 text-xs">
      <span>{status}</span>
      {#if valid(elapsedMs)}<span class="tabular-nums text-muted-foreground">Elapsed: {formatMs(elapsedMs)}</span>{/if}
    </p>
  {/if}
  {#if steps.length}
    <dl aria-label={label} class="grid grid-cols-2 gap-x-3 gap-y-2 text-xs sm:grid-cols-3">
      {#each steps as step, index (step.stage)}
        <div class="min-w-0 border-t pt-1" class:text-primary={running && activeStage === step.stage}>
          <dt><span class="text-muted-foreground">{index + 1}.</span> {step.label}</dt>
          <dd class="flex flex-wrap gap-x-2 tabular-nums">
            <span>{valid(step.duration) ? formatMs(step.duration) : running && activeStage === step.stage ? 'In progress…' : running ? 'Waiting' : '—'}</span>
            {#if valid(accumulated[index])}<span class="text-primary" title="Accumulated stage time">Σ {formatMs(accumulated[index]!)}</span>{/if}
          </dd>
        </div>
      {/each}
    </dl>
    <p class="text-xs text-muted-foreground">Stage duration · <span class="text-primary">Σ accumulated stage time</span> (excludes overhead)</p>
  {/if}
  {#if valid(firstAudioMs) || (!running && valid(totalMs))}
    <p class="flex flex-wrap gap-x-3 text-xs tabular-nums text-primary">
      {#if valid(firstAudioMs)}<span>First audio sent: {formatMs(firstAudioMs)}</span>{/if}
      {#if !running && valid(totalMs)}<span>Total: {formatMs(totalMs)}</span>{/if}
    </p>
  {/if}
</div>
