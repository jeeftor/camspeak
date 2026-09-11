<script lang="ts">
  import ErrorNotice from '$lib/components/ErrorNotice.svelte'
  import { onMount, onDestroy } from 'svelte'
  import { Button } from '$lib/components/ui/button'
  import TimingFlow from '$lib/components/TimingFlow.svelte'
  import { apiClient } from '$lib/api'
  import { timingSteps } from '$lib/audio-draft'
  import { stageInfo } from '$lib/stages'
  import { formatMs } from '$lib/utils'
  import type { CameraSummary } from '$lib/types'
  let { preset, text, voice, rate, channels, disabled = false, busy = $bindable(false) }: { preset: string; text: string; voice: string; rate: number; channels: number; disabled?: boolean; busy?: boolean } = $props()
  type Run = { timings?: Record<string, number>; ttfs_ms?: number; total_ms?: number; error?: string }
  let cameras = $state<CameraSummary[]>([])
  let camera = $state('')
  let confirmed = $state(false)
  let reverse = $state(false)
  let status = $state('')
  let error = $state('')
  let id = $state('')
  let result = $state<{ buffered?: Run; streaming?: Run; mode?: string; timings?: Record<string, number> }>({})
  let controller: AbortController | undefined
  let disposed = false
  onMount(() => { apiClient.getCameras().then(value => { if (!disposed) cameras = value.filter(c => c.type === 'hikvision' && c.capabilities?.speak !== false) }).catch(e => error = e.message) })
  async function cancel() {
    if (!id) return
    try { await apiClient.cancelSpeakerBenchmark(id); status = 'Stopping comparison…' }
    catch (e) { error = `Could not confirm Stop. Use the camera Stop control. ${e instanceof Error ? e.message : e}` }
  }
  onDestroy(() => { disposed = true; controller?.abort(); if (busy && id) void cancel() })
  async function run() {
    if (busy || disabled || !confirmed || !camera) return
    busy = true; error = ''; result = {}; id = ''; status = 'Starting speaker comparison…'
    controller = new AbortController()
    try {
      const job = await apiClient.benchmarkSpeaker({ preset, camera, text, voice, sample_rate: rate, channels, confirm_playback: true, streaming_first: reverse }, job => {
        id = job.id
        if (disposed) { void cancel(); return }
        result = job.result as typeof result
        status = `${result.mode || 'Preparing'} · ${job.stage}`
      }, controller.signal)
      if (job.status !== 'done') error = job.error || 'Comparison stopped'
      status = job.status === 'done' ? 'Speaker comparison finished' : 'Comparison stopped'
    } catch (e) { if (!disposed) error = `Could not confirm the comparison. It was not retried. Use Stop if audio is still running. ${e instanceof Error ? e.message : e}` }
    finally { busy = false }
  }
  const difference = $derived(result.buffered?.ttfs_ms != null && result.streaming?.ttfs_ms != null ? result.buffered.ttfs_ms - result.streaming.ttfs_ms : undefined)
</script>

<section class="mt-4 flex min-w-0 flex-col gap-3 rounded border p-3">
  <h4 class="font-semibold">End-to-end speaker comparison</h4>
  <p class="text-xs text-warning">This plays the message twice on the selected camera and interrupts its current audio/AirPlay. Uses the text, voice and PCM format above. Saved playback mode is unchanged.</p>
  <label class="text-sm">Speaker<select aria-label="Benchmark speaker" bind:value={camera} disabled={busy || disabled} class="mt-1 w-full rounded border bg-background p-2"><option value="">Choose a Hikvision camera</option>{#each cameras as c}<option value={c.name}>{c.name}</option>{/each}</select></label>
  <label class="flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={confirmed} disabled={busy} />I understand this will play sound</label>
  <label class="flex items-center gap-2 text-xs"><input type="checkbox" bind:checked={reverse} disabled={busy} />Streaming first (reverse the order)</label>
  <p class="text-xs text-muted-foreground">Runs sequentially at the same starting gain. Repeat with reversed order to compare warm runs. First audio sent is not measured audible sound; listen for quality and try Stop.</p>
  <div class="flex flex-wrap gap-2"><Button onclick={run} disabled={busy || disabled || !confirmed || !camera || !text.trim() || !preset}>Play both modes on camera</Button>{#if busy || error && id}<Button variant="destructive" onclick={cancel} disabled={!id}>Stop comparison</Button>{/if}</div>
  {#if status}<p role="status" class="text-sm text-info">{status}</p>{/if}
  <ErrorNotice message={error} />
  {#if difference !== undefined}<p class="text-sm text-info">Streaming sent first audio {formatMs(Math.abs(difference))} {difference >= 0 ? 'earlier' : 'later'} than buffered.</p>{/if}
  <div class="grid min-w-0 gap-3">
    {#each ['buffered', 'streaming'] as mode}
      {@const completed = result[mode as 'buffered' | 'streaming']}
      {@const row = completed ?? (busy && result.mode === mode ? { timings: result.timings } : undefined)}
      {#if row}<article class="min-w-0 rounded border p-3"><h5 class="mb-2 font-semibold capitalize">{mode}</h5>
        <TimingFlow steps={timingSteps(row.timings).map(([stage, duration]) => ({ stage, duration, label: stageInfo(stage).label }))} ttsMode={mode as 'buffered' | 'streaming'} firstAudioMs={row.ttfs_ms} totalMs={row.total_ms} />
        {#if row.error}<p class="text-xs text-muted-foreground">Failed — see error notification.</p>{/if}
      </article>{/if}
    {/each}
  </div>
</section>
