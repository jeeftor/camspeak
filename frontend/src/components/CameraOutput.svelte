<script lang="ts">
  import { Play } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import GainSlider from '$lib/components/GainSlider.svelte'
  import Markdown from '$lib/components/Markdown.svelte'
  import AudioPlayer from './AudioPlayer.svelte'
  import VuMeter from './VuMeter.svelte'
  import PlaybackStrip from './PlaybackStrip.svelte'
  import { describeStages, describeStageLabel, timingSteps, type AudioDraft } from '$lib/audio-draft'
  import { formatMs, stepLabel } from '$lib/utils'
  import type { CameraSummary } from '$lib/types'
  import type { PlaybackMonitor } from '$lib/playback.svelte'

  let { camera, draft = $bindable(), monitor, onGain, onReplay }: {
    camera: CameraSummary
    draft: AudioDraft
    monitor: PlaybackMonitor
    onGain: () => void
    onReplay: () => void
  } = $props()
  const playback = $derived(monitor.state.cameras[camera.name])
  const level = $derived(playback?.state === 'playing' ? monitor.state.levels[camera.name] ?? playback.level ?? 0 : 0)
  const hasLevel = $derived(monitor.state.levels[camera.name] !== undefined || playback?.level !== undefined)
  const steps = $derived(timingSteps(draft.lastResult?.timings))
  const job = $derived(draft.lastResult?.label === 'Describe' ? draft.describeJob : null)
  let timingsOpen = $state(typeof window !== 'undefined' && matchMedia('(min-width: 1024px)').matches)
</script>

<section class="flex min-w-0 flex-col gap-3 rounded-lg border bg-card p-3" aria-label={`${camera.name} Camera Output`}>
  <div class="flex items-center justify-between gap-2">
    <h4 class="text-sm font-semibold">Camera Output</h4>
    <span class="text-right text-xs text-muted-foreground">{draft.busy ? draft.pendingAction || 'Working…' : playback?.state || 'idle'}</span>
  </div>
  <PlaybackStrip cameraName={camera.name} {playback} preparing={draft.busy || job?.status === 'running'} showLevel={false} onRefresh={monitor.refresh} />
  {#if draft.waveformPreset}
    {@const preset = draft.waveformPreset}
    <AudioPlayer peaksUrl={`/api/library/${encodeURIComponent(preset.category)}/${encodeURIComponent(preset.name)}/peaks`}
      duration={preset.duration} visualMode audioLevel={level} vuOrientation="vertical"
      subtitle={`${preset.category} / ${preset.name} · reference waveform`} />
  {:else}
    <div class="flex flex-wrap items-center justify-between gap-2"><span class="text-xs text-muted-foreground">VU · camera audio</span><VuMeter {level} /></div>
  {/if}
  {#if playback?.state === 'playing' && !hasLevel}
    <p class="text-xs text-muted-foreground">No audio levels received yet. The meter does not confirm speaker sound.</p>
  {:else if job?.stage === 'connecting' && draft.busy}
    <p class="text-xs text-muted-foreground">Waiting for the speaker connection. Audio has not started sending.</p>
  {/if}
  <GainSlider bind:value={draft.gain} disabled={draft.gainSaving} onchange={onGain} aria-label={`${camera.name} volume`} />

  {#if draft.lastResult}
    {@const result = draft.lastResult}
    <div class="flex flex-col gap-2 border-t pt-3">
      <div class="flex items-center justify-between gap-2"><h5 class="text-sm font-medium">{draft.busy && result.label === 'Describe' ? 'Describe progress' : `Last action · ${result.label}`}</h5>
        <Button size="sm" variant="ghost" disabled={draft.busy} onclick={() => { draft.lastResult = null; draft.describeJob = null; draft.describeError = '' }}>Clear result</Button></div>
      {#if job}
        <p role="status" class="flex flex-wrap justify-between gap-1 text-xs">
          <span>{draft.describeError && job.status === 'running' ? 'Last confirmed progress' : describeStageLabel(job.stage)}</span>
          <span class="tabular-nums text-muted-foreground">Elapsed: {formatMs(job.elapsed_ms)}</span>
        </p>
        <dl aria-label="Describe timing flow" class="grid grid-cols-2 gap-x-3 gap-y-2 text-xs sm:grid-cols-3">
          {#each describeStages as step}
            {@const duration = result.timings?.[step.timing] ?? (step.stage === 'playing' ? result.timings?.send_playback_ms : undefined)}
            <div class="min-w-0 border-t pt-1" class:text-primary={draft.busy && job.stage === step.stage}>
              <dt>{step.label}</dt>
              <dd class="tabular-nums">{duration != null ? formatMs(duration) : job.stage === step.stage && draft.busy ? 'In progress…' : job.status === 'running' && !draft.describeError ? 'Waiting' : '—'}</dd>
            </div>
          {/each}
        </dl>
        <p class="text-xs text-muted-foreground">Stages track audio sent to the camera, not confirmation of audible sound.</p>
      {/if}
      {#if result.label === 'Describe' && draft.describeError}<p role="alert" class="break-words text-xs text-destructive">{draft.describeError}</p>{/if}
      {#if result.description}<Markdown content={result.description} />{/if}
      <p class="flex flex-wrap gap-x-3 text-xs text-primary">
        {#if result.ttfs_ms != null}<span>First audio sent: {formatMs(result.ttfs_ms)}</span>{/if}
        {#if result.total_ms != null && (!job || job.status !== 'running')}<span>Total: {formatMs(result.total_ms)}</span>{/if}
      </p>
      {#if steps.length && !job}
        <details bind:open={timingsOpen} class="text-sm">
          <summary class="cursor-pointer text-muted-foreground">Timing details</summary>
          <dl class="mt-2 grid grid-cols-2 gap-x-4 gap-y-2">
            {#each steps as [key, value]}
              <div class="min-w-0 border-t pt-1"><dt class="text-xs capitalize text-muted-foreground">{stepLabel(key)}</dt><dd class="text-xs tabular-nums">{formatMs(value)}</dd></div>
            {/each}
          </dl>
        </details>
      {:else if !job && result.total_ms == null && !draft.busy}<p class="text-xs text-muted-foreground">This action did not return timing details.</p>{/if}
      {#if result.image}<details><summary class="cursor-pointer text-xs text-muted-foreground">Captured frame</summary><img src={result.image} alt="Frame used for this description" class="mt-2 w-full rounded-lg" /></details>{/if}
      {#if result.description}<Button size="sm" variant="outline" disabled={draft.busy || draft.gainSaving} onclick={onReplay}><Play class="h-4 w-4" /> Speak again</Button>{/if}
    </div>
  {/if}
</section>
