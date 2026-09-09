<script lang="ts">
  import { Loader2, Play, Plus, X } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Textarea } from '$lib/components/ui/textarea'
  import type { SnapshotBenchmarkResult } from '$lib/types'

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

  // Streaming state
  let totalSteps = $state(0)
  let currentStep = $state(0)
  let currentCamera = $state('')
  let currentPrompt = $state('')

  // Results: camera → prompt → results[]
  type ResultMap = Record<string, Record<string, SnapshotBenchmarkResult[]>>
  let results = $state<ResultMap>({})

  function addPrompt() {
    prompts = [...prompts, '']
  }

  function removePrompt(i: number) {
    if (prompts.length <= 1) return
    prompts = prompts.filter((_, idx) => idx !== i)
  }

  async function runBenchmark() {
    const activePrompts = prompts.filter(p => p.trim())
    if (activePrompts.length === 0) return

    running = true
    results = {}
    totalSteps = 0
    currentStep = 0
    currentCamera = ''
    currentPrompt = ''

    try {
      const resp = await fetch('/api/benchmark', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ prompts: activePrompts, with_vision: withVision }),
      })
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`)

      const reader = resp.body!.getReader()
      const dec = new TextDecoder()
      let buf = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buf += dec.decode(value, { stream: true })

        // Parse SSE events
        const events = buf.split('\n\n')
        buf = events.pop() || ''

        for (const eventBlock of events) {
          const lines = eventBlock.split('\n')
          let eventType = ''
          let eventData = ''
          for (const line of lines) {
            if (line.startsWith('event: ')) eventType = line.slice(7)
            if (line.startsWith('data: ')) eventData = line.slice(6)
          }
          if (!eventType || !eventData) continue

          try {
            const ev = JSON.parse(eventData)
            handleEvent(eventType, ev)
          } catch { /* skip malformed */ }
        }
      }
    } catch (e: any) {
      console.error('benchmark stream error:', e)
    } finally {
      running = false
    }
  }

  function handleEvent(type: string, ev: any) {
    switch (type) {
      case 'start':
        totalSteps = ev.total_steps
        break
      case 'progress':
        currentStep = ev.step
        currentCamera = ev.camera
        currentPrompt = ev.prompt
        break
      case 'result':
        if (!results[ev.camera]) results[ev.camera] = {}
        results[ev.camera] = { ...results[ev.camera], [ev.prompt]: ev.results }
        results = { ...results } // trigger reactivity
        break
      case 'done':
        break
    }
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
  function bestTime(results: SnapshotBenchmarkResult[], withV: boolean): number {
    const ok = results.filter(r => r.ok)
    if (ok.length === 0) return Infinity
    const key = withV ? 'total_sec' : 'snap_sec'
    return Math.min(...ok.map(r => (r[key] as number) || r.snap_sec))
  }

  // Collect all unique method names across all results
  function allMethods(): string[] {
    const methods = new Set<string>()
    for (const cam of Object.values(results)) {
      for (const rs of Object.values(cam)) {
        for (const r of rs) methods.add(r.method)
      }
    }
    return [...methods].sort()
  }

  // Get the best method for a camera+prompt
  function bestMethod(results: SnapshotBenchmarkResult[], withV: boolean): string {
    const ok = results.filter(r => r.ok)
    if (ok.length === 0) return '—'
    const key = withV ? 'total_sec' : 'snap_sec'
    const best = Math.min(...ok.map(r => (r[key] as number) || r.snap_sec))
    const winner = ok.find(r => ((r[key] as number) || r.snap_sec) === best)
    return winner?.method || '—'
  }

  const enabledCameras = $derived(cameras.filter((c: any) => c.enabled))
  const progressPct = $derived(totalSteps > 0 ? Math.round((currentStep / totalSteps) * 100) : 0)
</script>

<div class="flex flex-col gap-5 max-w-6xl">
  <!-- Header -->
  <div>
    <h2 class="text-lg font-semibold text-primary mb-1">Benchmark</h2>
    <p class="text-sm text-muted-foreground">
      Test a set of prompts against all enabled cameras and capture methods.
      Results stream in real-time. The model is pre-warmed before each camera to exclude cold-start loading time.
    </p>
  </div>

  <!-- Prompts editor -->
  <div class="rounded-lg border p-4 flex flex-col gap-3">
    <div class="flex items-center justify-between">
      <h3 class="text-sm font-semibold text-foreground">Test Prompts ({prompts.length})</h3>
      <Button variant="outline" size="sm" onclick={addPrompt} disabled={running}>
        <Plus class="h-3.5 w-3.5" />
        Add Prompt
      </Button>
    </div>
    {#each prompts as p, i}
      <div class="flex gap-2 items-start">
        <span class="text-xs text-muted-foreground font-mono mt-2 shrink-0">#{i + 1}</span>
        <Textarea
          bind:value={prompts[i]}
          disabled={running}
          rows={2}
          class="flex-1 text-sm"
          placeholder="Enter a prompt to test…"
        />
        <Button variant="ghost" size="sm" onclick={() => removePrompt(i)} disabled={running || prompts.length <= 1}
          class="shrink-0 mt-1">
          <X class="h-3.5 w-3.5" />
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

    <span class="text-sm text-muted-foreground">
      {enabledCameras.length} camera{enabledCameras.length === 1 ? '' : 's'} × {prompts.filter(p => p.trim()).length} prompt{prompts.filter(p => p.trim()).length === 1 ? '' : 's'}
      = {enabledCameras.length * prompts.filter(p => p.trim()).length} total tests
    </span>
  </div>

  <!-- Progress bar -->
  {#if running || currentStep > 0}
    <div class="flex flex-col gap-1">
      <div class="flex justify-between text-xs text-muted-foreground">
        <span>
          {#if running && currentCamera}
            Testing <span class="font-semibold text-foreground">{currentCamera}</span>
            with prompt "{currentPrompt.length > 50 ? currentPrompt.slice(0, 50) + '…' : currentPrompt}"
          {:else if currentStep === totalSteps && totalSteps > 0}
            Complete
          {/if}
        </span>
        <span>{currentStep}/{totalSteps}</span>
      </div>
      <div class="h-2 w-full overflow-hidden rounded-full bg-muted">
        <div class="h-full bg-primary transition-all" style="width:{progressPct}%"></div>
      </div>
    </div>
  {/if}

  <!-- Summary matrix -->
  {#if Object.keys(results).length > 0}
    <div class="rounded-lg border p-4 flex flex-col gap-3">
      <h3 class="text-sm font-semibold text-foreground">Summary — Best Method per Camera × Prompt</h3>
      <table class="text-sm">
        <thead>
          <tr class="text-xs text-muted-foreground border-b">
            <th class="text-left py-1.5 pr-4">Camera</th>
            {#each prompts.filter(p => p.trim()) as p, i}
              <th class="text-left py-1.5 px-2" title={p}>
                Prompt #{i + 1}
              </th>
            {/each}
          </tr>
        </thead>
        <tbody>
          {#each Object.keys(results).sort() as camName}
            <tr class="border-b last:border-0">
              <td class="py-1.5 pr-4 font-medium text-xs">{camName}</td>
              {#each prompts.filter(p => p.trim()) as p}
                {@const rs = results[camName]?.[p] ?? []}
                {@const best = bestMethod(rs, withVision)}
                {@const bestT = bestTime(rs, withVision)}
                <td class="py-1.5 px-2 text-xs">
                  {#if rs.length === 0}
                    <span class="text-muted-foreground">—</span>
                  {:else if best === '—'}
                    <span class="text-destructive">✗ all failed</span>
                  {:else}
                    <span class="font-mono text-amber-500 font-semibold">{best}</span>
                    <span class="text-muted-foreground ml-1">
                      {withVision ? fmtSec(rs.find(r => r.method === best)?.total_sec) : fmtSec(rs.find(r => r.method === best)?.snap_sec)}
                    </span>
                  {/if}
                </td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  <!-- Detailed results per camera -->
  {#if Object.keys(results).length > 0}
    {#each Object.keys(results).sort() as camName}
      <div class="rounded-lg border p-4 flex flex-col gap-4">
        <h3 class="text-sm font-semibold text-primary">{camName}</h3>

        {#each prompts.filter(p => p.trim()) as prompt}
          {@const rs = results[camName]?.[prompt] ?? []}
          {#if rs.length > 0}
            {@const bTime = bestTime(rs, withVision)}
            <div class="flex flex-col gap-1.5">
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
                  {#each rs as r}
                    {@const isBest = r.ok && (withVision ? r.total_sec : r.snap_sec) === bTime}
                    <tr class="border-b last:border-0">
                      <td class="py-1.5 pr-4 font-mono text-xs {isBest ? 'text-amber-500 font-semibold' : ''}">{r.method}</td>
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
          {/if}
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
