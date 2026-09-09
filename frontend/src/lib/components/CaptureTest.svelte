<script>
  // Reusable capture test widget — grabs a snapshot and shows the image + timing.
  // Also supports benchmarking all capture methods for comparison.
  // Props: camera (string), stream (string), width (number), snapMethod (string), disabled
  import { Button } from '$lib/components/ui/button'
  import { apiClient } from '$lib/api'

  let {
    camera = '',
    stream = '',
    width = 0,
    snapMethod = '',
    disabled = false,
    compact = false,
  } = $props()

  let captureBusy = $state(false)
  let captureImage = $state('')
  let captureError = $state('')
  let captureMs = $state(0)
  let captureSize = $state(0)
  let benchBusy = $state(false)
  let benchResults = $state([])
  let benchError = $state('')

  async function testCapture() {
    if (!camera) return
    captureBusy = true
    captureError = ''
    captureImage = ''
    captureMs = 0
    captureSize = 0
    if (captureImage) { URL.revokeObjectURL(captureImage) }
    try {
      const t0 = performance.now()
      const resp = await apiClient.snapshot(camera, stream || undefined, width || undefined)
      const elapsed = Math.round(performance.now() - t0)
      const blob = await resp.blob()
      captureMs = elapsed
      captureSize = blob.size
      captureImage = URL.createObjectURL(blob)
    } catch (e) {
      captureError = e.message || String(e)
    } finally {
      captureBusy = false
    }
  }

  async function testBenchmarkAll() {
    if (!camera) return
    benchBusy = true
    benchError = ''
    benchResults = []
    try {
      const res = await apiClient.snapshotBenchmark(camera)
      benchResults = res.results ?? []
    } catch (e) {
      benchError = e.message || String(e)
    } finally {
      benchBusy = false
    }
  }

  function fmtSize(bytes) {
    if (!bytes) return '—'
    if (bytes < 1024) return bytes + 'B'
    return (bytes / 1024).toFixed(0) + 'KB'
  }
</script>

<div class="flex flex-col gap-2">
  <div class="flex items-center gap-2 flex-wrap">
    <Button size="sm" variant="outline" onclick={testCapture} disabled={captureBusy || disabled || !camera}>
      {#if captureBusy}Capturing…{:else}Test capture{/if}
    </Button>
    <Button size="sm" variant="ghost" onclick={testBenchmarkAll} disabled={benchBusy || disabled || !camera}>
      {#if benchBusy}Benchmarking…{:else}Benchmark all{/if}
    </Button>
    {#if captureMs > 0}
      <span class="text-xs text-muted-foreground">
        {captureMs}ms · {fmtSize(captureSize)}
      </span>
    {/if}
  </div>

  {#if captureError}
    <p class="text-xs text-destructive">{captureError}</p>
  {/if}

  {#if captureImage}
    <div class="flex flex-col gap-1">
      <img src={captureImage} alt="captured frame" class="rounded-md max-h-[{compact ? '180' : '240'}px] object-contain border" />
      {#if !compact}
        <span class="text-[11px] text-muted-foreground">
          Captured with: {snapMethod || 'auto'}{stream ? ', stream: ' + stream : ''}{width ? ', width: ' + width : ''}
        </span>
      {/if}
    </div>
  {/if}

  {#if benchError}
    <p class="text-xs text-destructive">{benchError}</p>
  {/if}

  {#if benchResults.length > 0}
    <div class="overflow-x-auto">
      <table class="w-full text-xs">
        <thead>
          <tr class="text-left text-muted-foreground border-b">
            <th class="py-1 pr-3">Method</th>
            <th class="py-1 pr-3">Time</th>
            <th class="py-1 pr-3">Size</th>
            <th class="py-1">Resolution</th>
          </tr>
        </thead>
        <tbody>
          {#each benchResults as r}
            <tr class="border-b border-border/50 {!r.ok ? 'opacity-50' : ''}">
              <td class="py-1 pr-3 font-mono">{r.method}</td>
              <td class="py-1 pr-3">{r.ok ? (r.snap_sec * 1000).toFixed(0) + 'ms' : '✗'}</td>
              <td class="py-1 pr-3">{fmtSize(r.bytes)}</td>
              <td class="py-1">{r.width && r.height ? r.width + '×' + r.height : '—'}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
