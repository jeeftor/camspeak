<script lang="ts">
  import { onDestroy } from 'svelte'
  import { Button } from '$lib/components/ui/button'
  import { apiClient } from '$lib/api'
  import { formatMs } from '$lib/utils'

  let { presets = [], initialRate = 24000, initialChannels = 1, onFormatChange }: { presets: { name: string; model: string; default_voice: string }[]; initialRate?: number; initialChannels?: number; onFormatChange?: (rate: number, channels: number) => void } = $props()
  let preset = $state('')
  let text = $state('Hello! This is a speech streaming comparison. Your parcel is waiting by the front door.')
  let voice = $state('')
  let rate = $state(24000)
  let channels = $state(1)
  $effect(() => { rate = initialRate; channels = initialChannels })
  let busy = $state(false)
  let status = $state('')
  let results = $state<{ mode: string; first_byte_ms?: number; total_ms?: number; bytes?: number; audio?: string; error?: string }[]>([])
  let controller: AbortController | undefined
  const selected = $derived(presets.find(p => p.name === preset))
  $effect(() => { if (!presets.some(p => p.name === preset)) preset = presets[0]?.name ?? '' })
  onDestroy(() => controller?.abort())

  async function run(modes: ('buffered' | 'streaming')[]) {
    if (busy || !selected) return
    const request = { preset, text, voice: voice || selected.default_voice, sample_rate: rate, channels }
    controller = new AbortController()
    const signal = controller.signal
    busy = true
    results = []
    try {
      for (const mode of modes) {
        if (signal.aborted) break
        status = `Testing ${mode}…`
        try {
          const result = await apiClient.benchmarkTTS({ ...request, mode }, signal)
          if (!signal.aborted) results = [...results, result]
        } catch (e) {
          if (!signal.aborted) results = [...results, { mode, error: e instanceof Error ? e.message : String(e) }]
        }
      }
    } finally { busy = false; status = signal.aborted ? 'Canceled' : 'Comparison finished' }
  }
</script>

<section class="mt-4 flex flex-col gap-3 rounded-lg border bg-card p-4">
  <h3 class="font-semibold text-primary">TTS streaming test · experimental</h3>
  <p class="text-sm text-muted-foreground">Compare this saved endpoint/model. These tests do not change playback mode or play on cameras. Local previews require pressing Play.</p>
  <div class="flex flex-wrap gap-3">
    <label class="flex min-w-0 flex-col gap-1 text-sm">TTS preset
      <select bind:value={preset} disabled={busy} class="max-w-full rounded border bg-background p-2">{#each presets as p}<option value={p.name}>{p.name} · {p.model}</option>{/each}</select>
    </label>
    <label class="flex flex-col gap-1 text-sm">Voice override<input bind:value={voice} disabled={busy} placeholder={selected?.default_voice} class="rounded border bg-background p-2" /></label>
  </div>
  <label class="flex flex-col gap-1 text-sm">Test text<textarea bind:value={text} disabled={busy} maxlength="2000" rows="3" class="rounded border bg-background p-2"></textarea></label>
  <div class="flex flex-wrap gap-3">
    <label class="flex flex-col gap-1 text-sm">Streaming PCM rate (Hz)<input type="number" min="8000" max="96000" bind:value={rate} onchange={() => onFormatChange?.(rate, Number(channels))} disabled={busy} class="w-36 rounded border bg-background p-2" /></label>
    <label class="flex flex-col gap-1 text-sm">Channels<select bind:value={channels} onchange={() => onFormatChange?.(rate, Number(channels))} disabled={busy} class="rounded border bg-background p-2"><option value={1}>Mono</option><option value={2}>Stereo</option></select></label>
  </div>
  <p class="text-xs text-warning">Streaming requests raw signed 16-bit little-endian PCM. Confirm your server's sample rate and channels; these values control local playback speed. This test does not enable streaming camera playback.</p>
  <div class="flex flex-wrap gap-2">
    <Button onclick={() => run(['buffered', 'streaming'])} disabled={busy || !preset || !text.trim()}>Compare buffered vs streaming</Button>
    <Button variant="outline" onclick={() => run(['streaming'])} disabled={busy || !preset || !text.trim()}>Test streaming only</Button>
    {#if busy}<Button variant="destructive" onclick={() => controller?.abort()}>Cancel test</Button>{/if}
  </div>
  {#if status}<p role="status" class="text-sm text-info">{status}</p>{/if}
  <p class="text-xs text-muted-foreground">First byte is measured at CamSpeak, not first sound. A WAV header can arrive before usable audio. Total measures full response delivery. Compare runs may include model warm-up; repeat to compare warm runs.</p>
  <div class="grid gap-3 md:grid-cols-2">
    {#each results as result}
      <article class="min-w-0 rounded border p-3">
        <h4 class="font-semibold capitalize">{result.mode}</h4>
        {#if result.error}<p role="alert" class="break-words text-sm text-destructive">{result.error}</p>
        {:else}
          <dl class="my-2 grid grid-cols-2 gap-2 text-sm"><div><dt>First byte</dt><dd class="text-info">{formatMs(result.first_byte_ms)}</dd></div><div><dt>Total</dt><dd>{formatMs(result.total_ms)}</dd></div><div><dt>Audio bytes</dt><dd>{result.bytes}</dd></div></dl>
          <audio controls preload="none" src={result.audio} class="w-full" aria-label={`${result.mode} local audio preview`}></audio>
        {/if}
      </article>
    {/each}
  </div>
</section>
