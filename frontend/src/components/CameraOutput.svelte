<script lang="ts">
  import { Play } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import GainSlider from '$lib/components/GainSlider.svelte'
  import Markdown from '$lib/components/Markdown.svelte'
  import AudioPlayer from './AudioPlayer.svelte'
  import VuMeter from './VuMeter.svelte'
  import PlaybackStrip from './PlaybackStrip.svelte'
  import { timingSteps, type AudioDraft } from '$lib/audio-draft'
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
  let timingsOpen = $state(typeof window !== 'undefined' && matchMedia('(min-width: 1024px)').matches)
</script>

<section class="flex min-w-0 flex-col gap-3 rounded-lg border bg-card p-3" aria-label={`${camera.name} Camera Output`}>
  <div class="flex items-center justify-between gap-2">
    <h4 class="text-sm font-semibold">Camera Output</h4>
    <span class="text-xs text-muted-foreground">{draft.busy ? draft.pendingAction || 'Working…' : playback?.state || 'idle'}</span>
  </div>
  <PlaybackStrip cameraName={camera.name} {playback} preparing={draft.busy} showLevel={false} onRefresh={monitor.refresh} />
  {#if draft.waveformPreset}
    {@const preset = draft.waveformPreset}
    <AudioPlayer peaksUrl={`/api/library/${encodeURIComponent(preset.category)}/${encodeURIComponent(preset.name)}/peaks`}
      duration={preset.duration} visualMode audioLevel={level} vuOrientation="vertical"
      subtitle={`${preset.category} / ${preset.name} · reference waveform`} />
  {:else}
    <div class="flex flex-wrap items-center justify-between gap-2"><span class="text-xs text-muted-foreground">VU · camera audio</span><VuMeter {level} /></div>
  {/if}
  {#if playback?.state === 'playing' && !hasLevel}
    <p class="text-xs text-muted-foreground">Live levels are not reported for this playback. The meter does not confirm speaker sound.</p>
  {/if}
  <GainSlider bind:value={draft.gain} disabled={draft.gainSaving} onchange={onGain} aria-label={`${camera.name} volume`} />

  {#if draft.lastResult}
    {@const result = draft.lastResult}
    <div class="flex flex-col gap-2 border-t pt-3">
      <div class="flex items-center justify-between gap-2"><h5 class="text-sm font-medium">Last action · {result.label}</h5>
        <Button size="sm" variant="ghost" disabled={draft.busy} onclick={() => draft.lastResult = null}>Clear result</Button></div>
      {#if result.description}<Markdown content={result.description} />{/if}
      <p class="flex flex-wrap gap-x-3 text-xs text-primary">
        {#if result.ttfs_ms != null}<span>First sound: {formatMs(result.ttfs_ms)}</span>{/if}
        {#if result.total_ms != null}<span>Total: {formatMs(result.total_ms)}</span>{/if}
      </p>
      {#if steps.length}
        <details bind:open={timingsOpen} class="text-sm">
          <summary class="cursor-pointer text-muted-foreground">Timing details</summary>
          <dl class="mt-2 grid grid-cols-2 gap-x-4 gap-y-2">
            {#each steps as [key, value]}
              <div class="min-w-0 border-t pt-1"><dt class="text-xs capitalize text-muted-foreground">{stepLabel(key)}</dt><dd class="text-xs tabular-nums">{formatMs(value)}</dd></div>
            {/each}
          </dl>
        </details>
      {:else if result.total_ms == null}<p class="text-xs text-muted-foreground">This action did not return timing details.</p>{/if}
      {#if result.image}<details><summary class="cursor-pointer text-xs text-muted-foreground">Captured frame</summary><img src={result.image} alt="Frame used for this description" class="mt-2 w-full rounded-lg" /></details>{/if}
      {#if result.description}<Button size="sm" variant="outline" disabled={draft.busy || draft.gainSaving} onclick={onReplay}><Play class="h-4 w-4" /> Speak again</Button>{/if}
    </div>
  {/if}
</section>
