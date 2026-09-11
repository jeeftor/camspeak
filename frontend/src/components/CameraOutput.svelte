<script lang="ts">
  import { Play } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import GainSlider from '$lib/components/GainSlider.svelte'
  import Markdown from '$lib/components/Markdown.svelte'
  import AudioPlayer from './AudioPlayer.svelte'
  import VuMeter from './VuMeter.svelte'
  import PlaybackStrip from './PlaybackStrip.svelte'
  import TimingFlow from '$lib/components/TimingFlow.svelte'
  import { describeStages, describeStageLabel, timingSteps, type AudioDraft } from '$lib/audio-draft'
  import { presetPlaybackPosition } from '$lib/playback-position'
  import { stepLabel } from '$lib/utils'
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
  const job = $derived(draft.lastResult?.label === 'Describe' ? draft.describeJob : null)
  const steps = $derived(job ? describeStages.map(step => ({ ...step,
    duration: draft.lastResult?.timings?.[step.timing] ?? (step.stage === 'playing' ? draft.lastResult?.timings?.send_playback_ms : undefined),
  })) : timingSteps(draft.lastResult?.timings).map(([key, duration]) => ({
    stage: key, duration, label: describeStages.find(step => step.timing === key)?.label
      ?? (key === 'send_playback_ms' ? 'Playback' : key === 'snap_ms' ? 'Snapshot' : stepLabel(key)),
  })))
  let playbackClock = $state(Date.now())
  const waveformPosition = $derived(presetPlaybackPosition(playback, draft.waveformPreset, playbackClock))
  $effect(() => {
    const current = playback
    const preset = draft.waveformPreset
    playbackClock = Date.now()
    if (current?.state !== 'playing' || presetPlaybackPosition(current, preset, Date.now()) === null) return
    const timer = setInterval(() => playbackClock = Date.now(), 100)
    return () => clearInterval(timer)
  })
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
      externalProgress={waveformPosition ?? 0}
      subtitle={`${preset.category} / ${preset.name} · ${waveformPosition === null ? 'reference waveform' : 'estimated playback position'}`} />
    {#if waveformPosition !== null}<p class="text-xs text-muted-foreground">Position is estimated from camera playback timing, not confirmation of audible sound.</p>{/if}
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
      <TimingFlow {steps} label={`${result.label} timing flow`}
        ttsMode={result.tts_mode}
        activeStage={job?.stage} running={job?.status === 'running' && draft.busy && !draft.describeError}
        status={job ? draft.describeError && job.status === 'running' ? 'Last confirmed progress' : describeStageLabel(job.stage) : ''}
        elapsedMs={job?.elapsed_ms} firstAudioMs={result.ttfs_ms}
        totalMs={job?.status === 'running' ? undefined : result.total_ms} />
      {#if steps.length}
        <p class="text-xs text-muted-foreground">Stages track audio sent to the camera, not confirmation of audible sound.</p>
      {/if}
      {#if result.label === 'Describe' && draft.describeError}<p role="alert" class="break-words text-xs text-destructive">{draft.describeError}</p>{/if}
      {#if result.description}<Markdown content={result.description} />{/if}
      {#if result.capture_source}<p class="text-xs text-muted-foreground">Captured using {result.capture_source}</p>{/if}
      {#if !steps.length && !job && result.total_ms == null && !draft.busy}<p class="text-xs text-muted-foreground">This action did not return timing details.</p>{/if}
      {#if result.image}<details><summary class="cursor-pointer text-xs text-muted-foreground">Captured frame</summary><img src={result.image} alt="Frame used for this description" class="mt-2 w-full rounded-lg" /></details>{/if}
      {#if result.description}<Button size="sm" variant="outline" disabled={draft.busy || draft.gainSaving} onclick={onReplay}><Play class="h-4 w-4" /> Speak again</Button>{/if}
    </div>
  {/if}
</section>
