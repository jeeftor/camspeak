<script lang="ts">
  import { Loader2, Timer, Play } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Textarea } from '$lib/components/ui/textarea'
  import { apiClient } from '$lib/api'
  import type { SnapshotBenchmarkResult, SnapshotBenchmarkResponse } from '$lib/types'

  let { cameras = [] } = $props()

  // Default prompts to test across all cameras
  const defaultPrompts = [
    'Describe what you see in this image.',
    'How many people are visible? If none, say "no people".',
    'Is there a vehicle in the frame? If so, describe it.',
    'What are the weather and lighting conditions?',
  ]

  let prompts = $state(defaultPrompts.map(p => p))
  let withVision = $state(true)
  let running = $state(false)
  let progress = $state('')
  let status = $state('')

  // Results: camera → prompt → results[]
  type CameraResults = Record<string, SnapshotBenchmarkResult[]>
  let allResults = $state<Record<string, CameraResults>>({})

  function addPrompt() {
    prompts = [...prompts, '']
  }

  function removePrompt(i: number) {
    prompts = prompts.filter((_, idx) => idx !== i)
  }

  async function runBenchmark() {
    const activeCameras = cameras.filter((c: any) => c.enabled)
    if (activeCameras.length === 0) {
      status = 'No enabled cameras'
      return
    }
    const activePrompts = prompts.filter(p => p.trim())
    if (activePrompts.length === 0) {
      status = 'No prompts to test'
      return
    }

    running = true
    status = ''
    allResults = {}

    let camDone = 0
    for (const cam of activeCameras) {
      progress = `Testing ${cam.name} (${camDone + 1}/${activeCameras.length})…`
      const camResults: CameraResults = {}

      for (const prompt of activePrompts) {
        try {
          const data: SnapshotBenchmarkResponse = await apiClient.snapshotBenchmark(
            cam.name, withVision, prompt,
          )
          camResults[prompt] = data.results
        } catch (e: any) {
          camResults[prompt] = [{
            method: 'error',
            ok: false,
            snap_sec: 0,
            bytes: 0,
            error: e.message ?? 'failed',
          }]
        }
      }

      allResults[cam.name] = camResults
      camDone++
    }

    progress = ''
    running = false
    status = `Done — tested ${activeCameras.length} camera${activeCameras.length === 1 ? '' : 's'} × ${activePrompts.length} prompt${activePrompts.length === 1 ? '' : 's'}`
  }

  function fmtSec(s: number | undefined): string {
    if (!s || s <= 0) return '—'
    return `${s.toFixed(2)}s`
  }

  function fmtBytes(b: number): string {
    if (b > 1024) return `${(b / 1024).toFixed(0)}KB`
    return `${b}B`
  }

  // Find the fastest total time per camera+prompt
  function bestTotal(results: SnapshotBenchmarkResult[]): number {
    const ok = results.filter(r => r.ok && r.total_sec)
    if (ok.length === 0) return Infinity
    return Math.min(...ok.map(r => r.total_sec!))
  }

  // Find the fastest capture time per camera+prompt
  function bestSnap(results: SnapshotBenchmarkResult[]): number {
    const ok = results.filter(r => r.ok)
    if (ok.length === 0) return Infinity
    return Math.min(...ok.map(r => r.snap_sec))
  }
</script>

<div class="flex flex-col gap-5 max-w-5xl">
  <!-- Header -->
  <div>
    <h2 class="text-lg font-semibold text-primary mb-1">Benchmark</h2>
    <p class="text-sm text-muted-foreground">
      Test a set of prompts against all enabled cameras and capture methods.
      The model is pre-warmed before each camera to exclude cold-start loading time.
    </p>
  </div>

  <!-- Prompts editor -->
  <div class="rounded-lg border p-4 flex flex-col gap-3">
    <div class="flex items-center justify-between">
      <h3 class="text-sm font-semibold text-foreground">Test Prompts</h3>
      <Button variant="outline" size="sm" onclick={addPrompt} disabled={running}>
        + Add Prompt
      </Button>
    </div>
    {#each prompts as p, i}
      <div class="flex gap-2 items-start">
        <Textarea
          bind:value={prompts[i]}
          disabled={running}
          rows={2}
          class="flex-1 text-sm"
          placeholder="Enter a prompt to test…"
        />
        <Button variant="ghost" size="sm" onclick={() => removePrompt(i)} disabled={running || prompts.length <= 1}
          class="shrink-0 mt-1">
          ✕
        </Button>
      </div>
    {/each}
  </div>

  <!-- Controls -->
  <div class="flex flex-wrap items-center gap-3">
    <label class="flex items-center gap-1.5 text-sm text-muted-foreground" title="Run the vision model against each captured frame to measure full pipeline latency">
      <input type="checkbox" bind:checked={withVision} disabled={running} class="accent-amber-500" />
      with vision
    </label>

    <Button onclick={runBenchmark} disabled={running}>
      {#if running}
        <Loader2 class="h-4 w-4 animate-spin" />
      {:else}
        <Play class="h-4 w-4" />
      {/if}
      Run Benchmark
    </Button>

    {#if progress}
      <span class="text-sm text-muted-foreground">{progress}</span>
    {/if}
    {#if status}
      <span class="text-sm text-muted-foreground">{status}</span>
    {/if}
  </div>

  <!-- Results -->
  {#if Object.keys(allResults).length > 0}
    {#each Object.entries(allResults) as [camName, camResults]}
      <div class="rounded-lg border p-4 flex flex-col gap-3">
        <h3 class="text-sm font-semibold text-primary">{camName}</h3>

        {#each Object.entries(camResults) as [prompt, results]}
          {@const bTotal = withVision ? bestTotal(results) : bestSnap(results)}
          <div class="flex flex-col gap-1">
            <div class="text-xs text-muted-foreground italic">
              "{prompt.length > 80 ? prompt.slice(0, 80) + '…' : prompt}"
            </div>
            <table class="text-sm">
              <thead>
                <tr class="text-xs text-muted-foreground border-b">
                  <th class="text-left py-1 pr-4">Method</th>
                  <th class="text-right py-1 pr-4">Capture</th>
                  {#if withVision}
                    <th class="text-right py-1 pr-4">Vision</th>
                    <th class="text-right py-1 pr-4">Total</th>
                  {/if}
                  <th class="text-right py-1 pr-4">Size</th>
                  <th class="text-left py-1">Status</th>
                </tr>
              </thead>
              <tbody>
                {#each results as r}
                  {@const isBest = r.ok && (withVision ? r.total_sec : r.snap_sec) === bTotal}
                  <tr class="border-b last:border-0">
                    <td class="py-1.5 pr-4 font-mono text-xs">{r.method}</td>
                    <td class="py-1.5 pr-4 text-right font-mono text-xs {r.ok ? '' : 'text-muted-foreground'}">
                      {r.ok ? fmtSec(r.snap_sec) : '—'}
                    </td>
                    {#if withVision}
                      <td class="py-1.5 pr-4 text-right font-mono text-xs {r.ok ? '' : 'text-muted-foreground'}">
                        {r.ok && r.vision_sec ? fmtSec(r.vision_sec) : '—'}
                      </td>
                      <td class="py-1.5 pr-4 text-right font-mono text-xs {isBest ? 'text-amber-500 font-semibold' : r.ok ? '' : 'text-muted-foreground'}">
                        {r.ok ? fmtSec(r.total_sec) : '—'}
                      </td>
                    {/if}
                    <td class="py-1.5 pr-4 text-right font-mono text-xs text-muted-foreground">
                      {r.ok ? fmtBytes(r.bytes) : '—'}
                    </td>
                    <td class="py-1.5 text-xs {r.ok ? 'text-green-600' : 'text-destructive'}">
                      {r.ok ? '✓' : `✗ ${r.error || 'failed'}`}
                    </td>
                  </tr>
                  {#if withVision && r.ok && r.preview}
                    <tr class="border-b last:border-0">
                      <td colspan={withVision ? 6 : 4} class="py-1 pr-4 text-xs text-muted-foreground italic">
                        {r.preview}
                      </td>
                    </tr>
                  {/if}
                {/each}
              </tbody>
            </table>
          </div>
        {/each}
      </div>
    {/each}

    <p class="text-xs text-muted-foreground">
      {#if withVision}
        Model pre-warmed before each camera. Best total (capture + vision) per prompt highlighted in amber.
        Larger images may give better descriptions but take longer to process.
      {:else}
        Fastest capture per prompt highlighted in amber. Enable "with vision" to see the full pipeline cost.
      {/if}
    </p>
  {/if}
</div>
