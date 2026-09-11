<script>
  import { onDestroy } from 'svelte'
  import { Button } from '$lib/components/ui/button'
  import { apiClient } from '$lib/api'
  let { camera = '', stream = '', width = 0, snapMethod = '', disabled = false,
    compact = false, streams = [], cameraType = '', onSelect } = $props()
  let busy = $state(false)
  let results = $state([])
  let controller
  let urls = []
  function clear() { urls.forEach(url => URL.revokeObjectURL(url)); urls = []; results = [] }
  onDestroy(() => { controller?.abort(); clear() })
  async function capture(source, signal) {
    const start = performance.now()
    try {
      const response = await apiClient.snapshot(camera, source.stream || undefined, width || undefined, signal, source.method)
      const blob = await response.blob()
      const url = URL.createObjectURL(blob)
      urls.push(url)
      const image = new Image()
      image.src = url
      await image.decode()
      return { ...source, actual: response.headers.get('X-Capture-Source') || 'Source not reported', url,
        ms: Math.round(performance.now() - start), bytes: blob.size, resolution: `${image.naturalWidth}×${image.naturalHeight}` }
    } catch (e) { return { ...source, error: e.message || String(e) } }
  }
  async function run(all) {
    if (busy || !camera) return
    clear(); busy = true
    controller = new AbortController()
    const signal = controller.signal
    const candidates = all ? [
      ...(['hikvision', 'reolink'].includes(cameraType) ? [
        { method: 'isapi', stream: 'main', label: 'Direct API · Main' },
        { method: 'isapi', stream: 'sub', label: 'Direct API · Sub' },
      ] : []),
      ...[...new Set([...streams.map(s => s.name), ...(!['', 'main', 'sub'].includes(stream) ? [stream] : [])])]
        .map(name => ({ method: 'go2rtc', stream: name, label: `go2rtc · ${name}` })),
      { method: 'frigate', stream: '', label: 'Frigate · Latest image' },
    ] : [{ method: snapMethod || 'auto', stream, label: 'Current selection' }]
    try {
      for (const source of candidates) {
        if (signal.aborted) break
        const result = await capture(source, signal)
        if (!signal.aborted) results = [...results, result]
      }
    } finally { busy = false }
  }
</script>
<div class="flex min-w-0 flex-col gap-3">
  <p class="text-xs text-muted-foreground">Compare sources, inspect the images, then choose one. Times include network and image decoding. Save the camera to apply your choice. Tests use saved camera credentials.</p>
  <div class="flex flex-wrap gap-2">
    <Button size="sm" onclick={() => run(true)} disabled={busy || disabled || !camera}>{busy ? 'Capturing…' : 'Compare capture sources'}</Button>
    <Button size="sm" variant="outline" onclick={() => run(false)} disabled={busy || disabled || !camera}>Test selection</Button>
    {#if busy}<Button size="sm" variant="ghost" onclick={() => controller?.abort()}>Cancel test</Button>{/if}
  </div>
  <div class="grid min-w-0 gap-3">
    {#each results as result}
      <article class="min-w-0 rounded border p-2 text-xs">
        <strong class="break-words">{result.label}</strong>
        {#if result.error}<p class="break-words text-destructive">Unavailable: {result.error}</p>
        {:else}
          <img src={result.url} alt={result.label + ' snapshot'} class="my-2 max-h-40 w-full rounded object-contain" />
          <p>{result.ms}ms · {(result.bytes / 1024).toFixed(0)}KB · {result.resolution}</p>
          <p class="break-words text-muted-foreground">Used: {result.actual}</p>
          {#if onSelect}<Button size="sm" variant="outline" disabled={busy} onclick={() => onSelect(result.method, result.stream)}>Use this source</Button>{/if}
        {/if}
      </article>
    {/each}
  </div>
</div>
