<script lang="ts">
  import { Loader2, Camera, Timer } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { apiClient } from '$lib/api'
  import type { SnapshotBenchmarkResult } from '$lib/types'
  import { timeUnit, toggleTimeUnit, fmtTime } from '$lib/timefmt.svelte'
  import CameraSelect from '$lib/components/CameraSelect.svelte'
  import HoverPreview from '$lib/components/HoverPreview.svelte'
  import PromptEditor from '$lib/components/PromptEditor.svelte'

  let { cameras = [] } = $props()

  let selectedCamera = $state('')
  let withVision = $state(false)
  let customPrompt = $state('')
  let running = $state(false)
  let results = $state<SnapshotBenchmarkResult[]>([])
  let hoveredImage = $state<string | null>(null)
  let hoverX = $state(0)
  let hoverY = $state(0)

  async function runBenchmark() {
    if (!selectedCamera) return
    running = true
    results = []
    try {
      const data = await apiClient.snapshotBenchmark(selectedCamera, withVision, customPrompt.trim())
      results = data.results
    } catch (e: any) {
      console.error('capture benchmark error:', e)
    } finally {
      running = false
    }
  }

  function fmtBytes(b: number): string {
    if (b > 1024 * 1024) return `${(b / 1024 / 1024).toFixed(1)}MB`
    if (b > 1024) return `${(b / 1024).toFixed(0)}KB`
    return `${b}B`
  }

  function fmtRes(r: SnapshotBenchmarkResult): string {
    if (!r.width || !r.height) return '—'
    return `${r.width}×${r.height}`
  }

  // Find best time
  const bestKey = $derived(withVision ? 'total_sec' : 'snap_sec')
  const okResults = $derived(results.filter(r => r.ok))
  const bestTime = $derived(okResults.length > 0
    ? Math.min(...okResults.map(r => (r[bestKey] as number) || r.snap_sec))
    : Infinity)
</script>

<div class="flex flex-col gap-5 max-w-4xl">
  <!-- Header -->
  <div>
    <h2 class="text-lg font-semibold text-primary mb-1">Capture Benchmark</h2>
    <p class="text-sm text-muted-foreground">
      Test all capture methods for a single camera. Compare capture time, image resolution, and file size.
      Enable "with vision" to also measure the full pipeline (capture + model inference).
    </p>
  </div>

  <!-- Controls -->
  <div class="flex flex-wrap items-end gap-3">
    <label class="flex flex-col gap-1 text-sm text-muted-foreground">
      Camera
      <CameraSelect bind:value={selectedCamera} {cameras} disabled={running} class="min-w-[180px]" />
    </label>

    <label class="flex items-center gap-1.5 text-sm text-muted-foreground self-center pb-2" title="Run the vision model against each captured frame">
      <input type="checkbox" bind:checked={withVision} disabled={running} class="accent-amber-500" />
      with vision
    </label>

    <Button onclick={runBenchmark} disabled={running || !selectedCamera}>
      {#if running}
        <Loader2 class="h-4 w-4 animate-spin" />
      {:else}
        <Timer class="h-4 w-4" />
      {/if}
      Run Benchmark
    </Button>
  </div>

  <!-- Custom prompt (only relevant with vision) -->
  {#if withVision}
    <div class="rounded-lg border p-3 flex flex-col gap-1.5">
      <label class="text-xs font-semibold text-muted-foreground">
        Custom Prompt <span class="font-normal">(optional — overrides camera/global prompt)</span>
      </label>
      <PromptEditor bind:value={customPrompt} disabled={running}
        placeholder="Leave empty to use the camera's configured prompt" />
    </div>
  {/if}

  <!-- Results -->
  {#if running}
    <div class="flex items-center gap-2 text-sm text-muted-foreground">
      <Loader2 class="h-4 w-4 animate-spin" />
      {withVision ? 'Capturing + running vision on each method…' : 'Testing all capture methods…'}
    </div>
  {:else if results.length > 0}
    <div class="rounded-lg border p-4 flex flex-col gap-3">
      <h3 class="text-sm font-semibold text-foreground flex items-center gap-2">
        {selectedCamera}
        {#if withVision}<span class="text-xs font-normal text-muted-foreground">with vision</span>{/if}
        <button onclick={toggleTimeUnit} class="text-xs font-mono text-muted-foreground hover:text-primary border rounded px-1.5 py-0.5 ml-2" title="Toggle time units (s/ms)">
          {timeUnit()}
        </button>
      </h3>
      <table class="text-sm">
        <thead>
          <tr class="text-xs text-muted-foreground border-b">
            <th class="text-left py-1.5 pr-4">Method</th>
            <th class="text-right py-1.5 pr-4 cursor-pointer select-none hover:text-primary" onclick={toggleTimeUnit} title="Click to toggle s/ms">Capture</th>
            {#if withVision}
              <th class="text-right py-1.5 pr-4 cursor-pointer select-none hover:text-primary" onclick={toggleTimeUnit} title="Click to toggle s/ms">Vision</th>
              <th class="text-right py-1.5 pr-4 cursor-pointer select-none hover:text-primary" onclick={toggleTimeUnit} title="Click to toggle s/ms">Total</th>
            {/if}
            <th class="text-right py-1.5 pr-4">Resolution</th>
            <th class="text-right py-1.5 pr-4">Size</th>
            <th class="text-left py-1.5">Status</th>
          </tr>
        </thead>
        <tbody>
          {#each results as r}
            {@const isBest = r.ok && ((r[bestKey] as number) || r.snap_sec) === bestTime}
            <tr class="border-b last:border-0">
              <td
                class="py-1.5 pr-4 font-mono text-xs {isBest ? 'text-amber-500 font-semibold' : ''} {r.image ? 'cursor-help' : ''}"
                onmouseenter={(e) => { if (r.image) { hoveredImage = r.image; hoverX = e.clientX; hoverY = e.clientY } }}
                onmousemove={(e) => { if (hoveredImage) { hoverX = e.clientX; hoverY = e.clientY } }}
                onmouseleave={() => { hoveredImage = null }}
              >{r.method}</td>
              <td class="py-1.5 pr-4 text-right font-mono text-xs cursor-pointer hover:text-primary {r.ok ? '' : 'text-muted-foreground'}" onclick={toggleTimeUnit} title="Toggle s/ms">
                {r.ok ? fmtTime(r.snap_sec) : '—'}
              </td>
              {#if withVision}
                <td class="py-1.5 pr-4 text-right font-mono text-xs cursor-pointer hover:text-primary {r.ok ? '' : 'text-muted-foreground'}" onclick={toggleTimeUnit} title="Toggle s/ms">
                  {r.ok && r.vision_sec ? fmtTime(r.vision_sec) : '—'}
                </td>
                <td class="py-1.5 pr-4 text-right font-mono text-xs cursor-pointer hover:text-primary {isBest ? 'text-amber-500 font-semibold' : r.ok ? '' : 'text-muted-foreground'}" onclick={toggleTimeUnit} title="Toggle s/ms">
                  {r.ok ? fmtTime(r.total_sec) : '—'}
                </td>
              {/if}
              <td class="py-1.5 pr-4 text-right font-mono text-xs text-muted-foreground">
                {r.ok ? fmtRes(r) : '—'}
              </td>
              <td class="py-1.5 pr-4 text-right font-mono text-xs text-muted-foreground">
                {r.ok ? fmtBytes(r.bytes) : '—'}
              </td>
              <td class="py-1.5 text-xs {r.ok ? 'text-green-600' : 'text-destructive'}">
                {r.ok ? '✓' : `✗ ${r.error || 'failed'}`}
              </td>
            </tr>
            {#if withVision && r.ok && r.preview}
              <tr class="border-b last:border-0">
                <td colspan={withVision ? 7 : 5} class="py-1 pr-4 text-xs text-muted-foreground italic">
                  {r.preview}
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
      <p class="text-xs text-muted-foreground">
        {#if withVision}
          Model pre-warmed before benchmarking. Best total (capture + vision) highlighted in amber.
          Resolution decoded after timing so it doesn't affect measurements.
        {:else}
          Fastest capture highlighted in amber. Resolution decoded after timing so it doesn't affect measurements.
        {/if}
      </p>
    </div>
  {:else if !selectedCamera}
    <div class="rounded-lg border border-dashed p-8 text-center text-muted-foreground">
      <Camera class="h-8 w-8 mx-auto mb-2 opacity-50" />
      <p>Select a camera and click "Run Benchmark" to test all capture methods.</p>
    </div>
  {/if}

  <!-- Hover image preview -->
  <HoverPreview bind:hoveredImage bind:hoverX bind:hoverY />
</div>
