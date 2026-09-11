<script lang="ts">
  import { formatMs } from '$lib/utils'
  import StageLabel from './StageLabel.svelte'
  import { stageColor, stageInfo } from '$lib/stages'
  import { accumulatedTimings, type TimingStage } from '$lib/timing-flow'

  let { steps, label = 'Timing flow', activeStage = '', running = false, status = '',
    elapsedMs, firstAudioMs, totalMs, ttsMode }: {
    ttsMode?: 'streaming' | 'buffered'
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
  const measured = $derived(steps.reduce((sum, step) => sum + (valid(step.duration) ? step.duration : 0), 0))
  let selectedStage = $state('')
  const selected = $derived(steps.find(step => step.stage === selectedStage))
  const selectedTotal = $derived(accumulated[steps.findIndex(step => step.stage === selectedStage)])
  const playbackIndex = $derived(steps.findIndex(step => stageInfo(step.stage).label === 'Playback'))
  const beforePlayback = $derived(playbackIndex > 0 ? accumulated[playbackIndex - 1] : undefined)

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
    <div class="relative flex h-4 w-full overflow-hidden rounded" aria-label="Stage duration bar">
      {#each steps as step, index}
        {#if valid(step.duration) && step.duration > 0}
          <button type="button" class="h-full" style:min-width="0" style:min-height="0" style:flex-shrink="0" style:padding="0" style:width={`${step.duration / Math.max(1, measured) * 100}%`} style:background={stageColor(step.stage)}
            title={`${step.label}: ${formatMs(step.duration)} · Accumulated: ${valid(accumulated[index]) ? formatMs(accumulated[index]) : 'unknown'} (excludes overhead). ${stageInfo(step.stage).description}`}
            onfocus={() => selectedStage = step.stage}
            onclick={() => selectedStage = step.stage}
            aria-label={`${step.label}: ${formatMs(step.duration)}; accumulated ${valid(accumulated[index]) ? formatMs(accumulated[index]) : 'unknown'}`}></button>
        {:else if running && activeStage === step.stage}
          <span class="h-full min-w-3 flex-1 animate-pulse" style:background={stageColor(step.stage)} aria-label={`${step.label} in progress`}></span>
        {/if}
      {/each}
      {#if valid(firstAudioMs) && valid(beforePlayback) && measured > 0}
        <span class="pointer-events-none absolute top-0 h-full border-l-2 border-white" style:left={`${beforePlayback / measured * 100}%`} title={`First audio sent: ${formatMs(firstAudioMs)}`}></span>
      {/if}
    </div>
    {#if selected}<p class="text-xs text-muted-foreground">{selected.label}: {formatMs(selected.duration)} · <span class="text-info">Σ {valid(selectedTotal) ? formatMs(selectedTotal) : 'unknown'} accumulated</span> (excludes overhead) · {stageInfo(selected.stage).description}</p>{/if}
    {#if running}<p class="text-xs text-muted-foreground">Live milestones · segment proportions settle when complete.</p>{/if}
    <dl aria-label={label} class="grid grid-cols-2 gap-x-3 gap-y-2 text-xs sm:grid-cols-3">
      {#each steps as step, index (step.stage)}
        <div class="min-w-0 border-t pt-1" class:text-primary={running && activeStage === step.stage}>
          <dt><span class="inline-block h-2 w-2 rounded-full" style:background={stageColor(step.stage)}></span> <span class="text-muted-foreground">{index + 1}.</span> <StageLabel stage={step.stage} label={step.label + (stageInfo(step.stage).label === 'TTS' && ttsMode ? ` · ${ttsMode === 'streaming' ? 'Streaming' : 'Buffered'}` : '')} /></dt>
          <dd class="flex flex-wrap gap-x-2 tabular-nums">
            <span>{valid(step.duration) ? formatMs(step.duration) : running && activeStage === step.stage ? 'In progress…' : running ? 'Waiting' : '—'}</span>
            {#if valid(accumulated[index])}<span class="text-info" title="Accumulated stage time">Σ {formatMs(accumulated[index]!)}</span>{/if}
          </dd>
        </div>
      {/each}
    </dl>
    <p class="text-xs text-muted-foreground">Stage duration · <span class="text-info">Σ accumulated stage time</span> (excludes overhead)</p>
    {#if ttsMode === 'streaming'}<p class="text-xs text-muted-foreground">Streaming milestones: TTS waits for initial PCM; generation and conversion continue during Playback.</p>{/if}
  {/if}
  {#if valid(firstAudioMs) || (!running && valid(totalMs))}
    <p class="flex flex-wrap gap-x-3 text-xs tabular-nums text-info">
      {#if valid(firstAudioMs)}<span>First audio sent: {formatMs(firstAudioMs)}</span>{/if}
      {#if !running && valid(totalMs)}<span>Total: {formatMs(totalMs)}</span>{/if}
    </p>
  {/if}
</div>
