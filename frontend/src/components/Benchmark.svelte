<script lang="ts">
  import { Loader2, Play, Plus, X, RefreshCw, ChevronDown, ChevronRight } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { apiClient } from '$lib/api'
  import { isVisionCapableModel } from '$lib/models'
  import type { SnapshotBenchmarkResult } from '$lib/types'
  import { timeUnit, toggleTimeUnit, fmtTime } from '$lib/timefmt.svelte'
  import HoverPreview from '$lib/components/HoverPreview.svelte'
  import PromptEditor from '$lib/components/PromptEditor.svelte'

  let { cameras = [] } = $props()

  // Default prompts
  const defaultPrompts = [
    'Describe what you see in this image.',
    'How many people are visible? If none, say "no people".',
    'Is there a vehicle in the frame? If so, describe it.',
    'What are the weather and lighting conditions?',
  ]

  let prompts = $state(defaultPrompts.map(p => p))
  let withVision = $state(true)
  let running = $state(false)
  let hoveredImage = $state<string | null>(null)
  let hoverX = $state(0)
  let hoverY = $state(0)

  // Model selection
  let availableModels = $state<string[]>([])
  let selectedModels = $state<Set<string>>(new Set())
  let modelsLoading = $state(false)
  let configuredModel = $state('')

  // Camera selection
  let selectedCameras = $state<Set<string>>(new Set())
  let camerasExpanded = $state(true)
  let modelsExpanded = $state(true)
  let promptsExpanded = $state(true)

  // Streaming state
  let totalSteps = $state(0)
  let currentStep = $state(0)
  let currentCamera = $state('')
  let currentMethod = $state('')
  let currentModel = $state('')

  // Live log feed
  let logLines = $state<string[]>([])

  // Results: flat list of all results
  type MatrixRow = {
    camera: string
    prompt: string
    method: string
    model: string
    result: SnapshotBenchmarkResult
  }
  let allRows = $state<MatrixRow[]>([])

  // Collapsed groups in results
  let collapsedCameras = $state<Set<string>>(new Set())

  const enabledCameras = $derived(cameras.filter((c: any) => c.enabled))

  // Initialize: select all cameras by default
  $effect(() => {
    if (selectedCameras.size === 0 && enabledCameras.length > 0) {
      selectedCameras = new Set(enabledCameras.map((c: any) => c.name))
    }
  })

  // Fetch available models
  async function fetchModels() {
    modelsLoading = true
    try {
      const cfg = await apiClient.getVisionConfig()
      configuredModel = cfg.model || ''
      const res = await apiClient.testVisionConfig(cfg.url, cfg.api_key)
      if (res.ok && res.data?.data) {
        availableModels = res.data.data
          .map(m => m.id)
          .filter(m => isVisionCapableModel(m))
          .sort()
        // Select all by default
        selectedModels = new Set(availableModels)
      }
    } catch (e) {
      logLines = [...logLines, `Error fetching models: ${e}`]
    } finally {
      modelsLoading = false
    }
  }

  // Auto-fetch models on mount
  fetchModels()

  function toggleCamera(name: string) {
    const next = new Set(selectedCameras)
    if (next.has(name)) next.delete(name)
    else next.add(name)
    selectedCameras = next
  }

  function toggleAllCameras() {
    if (selectedCameras.size === enabledCameras.length) {
      selectedCameras = new Set()
    } else {
      selectedCameras = new Set(enabledCameras.map((c: any) => c.name))
    }
  }

  function toggleModel(name: string) {
    const next = new Set(selectedModels)
    if (next.has(name)) next.delete(name)
    else next.add(name)
    selectedModels = next
  }

  function toggleAllModels() {
    if (selectedModels.size === availableModels.length) {
      selectedModels = new Set()
    } else {
      selectedModels = new Set(availableModels)
    }
  }

  function addPrompt() { prompts = [...prompts, ''] }
  function removePrompt(i: number) {
    if (prompts.length <= 1) return
    prompts = prompts.filter((_, idx) => idx !== i)
  }

  function toggleCameraCollapse(name: string) {
    const next = new Set(collapsedCameras)
    if (next.has(name)) next.delete(name)
    else next.add(name)
    collapsedCameras = next
  }

  async function runMatrix() {
    const activePrompts = prompts.filter(p => p.trim())
    const activeCameras = [...selectedCameras]
    const activeModels = withVision ? [...selectedModels] : [configuredModel || 'default']

    if (activeCameras.length === 0 || activePrompts.length === 0) return
    if (withVision && activeModels.length === 0) {
      logLines = [...logLines, 'No models selected']
      return
    }

    running = true
    allRows = []
    logLines = []
    totalSteps = 0
    currentStep = 0

    try {
      const resp = await fetch('/api/benchmark', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          cameras: activeCameras,
          prompts: activePrompts,
          models: withVision ? activeModels : [],
          with_vision: withVision,
        }),
      })
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`)

      const reader = resp.body!.getReader()
      const dec = new TextDecoder()
      let buf = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buf += dec.decode(value, { stream: true })

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
          } catch { /* skip */ }
        }
      }
    } catch (e: any) {
      logLines = [...logLines, `Error: ${e.message}`]
    } finally {
      running = false
    }
  }

  function handleEvent(type: string, ev: any) {
    switch (type) {
      case 'start':
        totalSteps = ev.total_steps
        logLines = [...logLines, `Starting: ${ev.cameras} cameras × ${ev.prompts} prompts × ${ev.models?.length || 1} models = ${ev.total_steps} total tests`]
        break
      case 'log':
        logLines = [...logLines, ev.msg]
        // Keep last 50 lines
        if (logLines.length > 50) logLines = logLines.slice(-50)
        break
      case 'progress':
        currentStep = ev.step
        currentCamera = ev.camera
        currentMethod = ev.method
        currentModel = ev.model
        break
      case 'result':
        for (const r of ev.results) {
          allRows = [...allRows, {
            camera: ev.camera,
            prompt: ev.prompt,
            method: r.method,
            model: r.model || ev.model || '',
            result: r,
          }]
        }
        break
      case 'done':
        logLines = [...logLines, `Complete: ${ev.total_steps} results in ${ev.elapsed_sec?.toFixed(1)}s`]
        break
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

  // Group results by camera
  function groupedResults(): Record<string, MatrixRow[]> {
    const groups: Record<string, MatrixRow[]> = {}
    for (const row of allRows) {
      if (!groups[row.camera]) groups[row.camera] = []
      groups[row.camera].push(row)
    }
    return groups
  }

  const progressPct = $derived(totalSteps > 0 ? Math.round((currentStep / totalSteps) * 100) : 0)
  const activeModelCount = $derived(withVision ? selectedModels.size : 1)
  const estimatedTests = $derived(selectedCameras.size * prompts.filter(p => p.trim()).length * activeModelCount)
</script>

<div class="flex flex-col gap-5 max-w-7xl">
  <!-- Header -->
  <div>
    <h2 class="text-lg font-semibold text-primary mb-1">Full Matrix Benchmark</h2>
    <p class="text-sm text-muted-foreground">
      Test all combinations: cameras × prompts × capture methods × models.
      Each capture method is run once, then the image is sent through all selected models.
      Model warmup runs before each camera to exclude cold-start time.
    </p>
  </div>

  <!-- Dimension selectors -->
  <div class="grid gap-4 md:grid-cols-2">
    <!-- Cameras -->
    <div class="rounded-lg border p-4 flex flex-col gap-2">
      <div class="flex items-center justify-between">
        <button class="flex items-center gap-1 text-sm font-semibold text-foreground"
          onclick={() => camerasExpanded = !camerasExpanded}>
          {#if camerasExpanded}<ChevronDown class="h-4 w-4" />{:else}<ChevronRight class="h-4 w-4" />{/if}
          Cameras ({selectedCameras.size}/{enabledCameras.length})
        </button>
        <Button variant="ghost" size="sm" onclick={toggleAllCameras} disabled={running}>
          {selectedCameras.size === enabledCameras.length ? 'Deselect all' : 'Select all'}
        </Button>
      </div>
      {#if camerasExpanded}
        <div class="flex flex-wrap gap-2">
          {#each enabledCameras as cam}
            <label class="flex items-center gap-1.5 text-xs rounded-md border px-2 py-1 cursor-pointer hover:bg-accent">
              <input type="checkbox" checked={selectedCameras.has(cam.name)}
                onchange={() => toggleCamera(cam.name)} disabled={running} class="accent-amber-500" />
              {cam.name}
            </label>
          {/each}
        </div>
      {/if}
    </div>

    <!-- Models -->
    <div class="rounded-lg border p-4 flex flex-col gap-2">
      <div class="flex items-center justify-between">
        <button class="flex items-center gap-1 text-sm font-semibold text-foreground"
          onclick={() => modelsExpanded = !modelsExpanded}>
          {#if modelsExpanded}<ChevronDown class="h-4 w-4" />{:else}<ChevronRight class="h-4 w-4" />{/if}
          Models ({withVision ? `${selectedModels.size}/${availableModels.length}` : 'N/A (no vision)'})
        </button>
        <div class="flex gap-1">
          <Button variant="ghost" size="sm" onclick={fetchModels} disabled={running || modelsLoading}>
            {#if modelsLoading}<Loader2 class="h-3 w-3 animate-spin" />{:else}<RefreshCw class="h-3 w-3" />{/if}
            Refresh
          </Button>
          {#if withVision}
            <Button variant="ghost" size="sm" onclick={toggleAllModels} disabled={running}>
              {selectedModels.size === availableModels.length ? 'Deselect all' : 'Select all'}
            </Button>
          {/if}
        </div>
      </div>
      {#if modelsExpanded}
        {#if !withVision}
          <p class="text-xs text-muted-foreground italic">Models only tested when "with vision" is enabled.</p>
        {:else if availableModels.length === 0}
          <p class="text-xs text-muted-foreground italic">
            {#if modelsLoading}Loading…{:else}No models found. Click Refresh to fetch from vision endpoint.{/if}
          </p>
        {:else}
          <div class="flex flex-wrap gap-2">
            {#each availableModels as m}
              <label class="flex items-center gap-1.5 text-xs rounded-md border px-2 py-1 cursor-pointer hover:bg-accent max-w-[250px]">
                <input type="checkbox" checked={selectedModels.has(m)}
                  onchange={() => toggleModel(m)} disabled={running} class="accent-amber-500" />
                <span class="truncate" title={m}>{m}</span>
              </label>
            {/each}
          </div>
        {/if}
      {/if}
    </div>
  </div>

  <!-- Prompts -->
  <div class="rounded-lg border p-4 flex flex-col gap-3">
    <div class="flex items-center justify-between">
      <button class="flex items-center gap-1 text-sm font-semibold text-foreground"
        onclick={() => promptsExpanded = !promptsExpanded}>
        {#if promptsExpanded}<ChevronDown class="h-4 w-4" />{:else}<ChevronRight class="h-4 w-4" />{/if}
        Prompts ({prompts.filter(p => p.trim()).length})
      </button>
      <Button variant="outline" size="sm" onclick={addPrompt} disabled={running}>
        <Plus class="h-3.5 w-3.5" /> Add
      </Button>
    </div>
    {#if promptsExpanded}
      {#each prompts as p, i}
        <div class="flex gap-2 items-start">
          <span class="text-xs text-muted-foreground font-mono mt-2 shrink-0">#{i + 1}</span>
          <PromptEditor bind:value={prompts[i]} disabled={running} class="flex-1"
            placeholder="Enter a prompt…" />
          <Button variant="ghost" size="sm" onclick={() => removePrompt(i)} disabled={running || prompts.length <= 1}
            class="shrink-0 mt-1">
            <X class="h-3.5 w-3.5" />
          </Button>
        </div>
      {/each}
    {/if}
  </div>

  <!-- Controls -->
  <div class="flex flex-wrap items-center gap-3">
    <label class="flex items-center gap-1.5 text-sm text-muted-foreground">
      <input type="checkbox" bind:checked={withVision} disabled={running} class="accent-amber-500" />
      with vision
    </label>

    <Button onclick={runMatrix} disabled={running || selectedCameras.size === 0 || prompts.filter(p => p.trim()).length === 0}>
      {#if running}
        <Loader2 class="h-4 w-4 animate-spin" />
      {:else}
        <Play class="h-4 w-4" />
      {/if}
      Run Full Matrix
    </Button>

    <span class="text-sm text-muted-foreground">
      {selectedCameras.size} camera{selectedCameras.size === 1 ? '' : 's'} ×
      {prompts.filter(p => p.trim()).length} prompt{prompts.filter(p => p.trim()).length === 1 ? '' : 's'} ×
      {activeModelCount} model{activeModelCount === 1 ? '' : 's'}
      ≈ {estimatedTests} tests
    </span>

    <button onclick={toggleTimeUnit} class="text-xs font-mono text-muted-foreground hover:text-primary border rounded px-2 py-1 ml-auto" title="Toggle time units (s/ms)">
      Time: {timeUnit()}
    </button>
  </div>

  <!-- Progress + Log -->
  {#if running || currentStep > 0}
    <div class="rounded-lg border p-4 flex flex-col gap-3">
      <!-- Progress bar -->
      <div class="flex flex-col gap-1">
        <div class="flex justify-between text-xs text-muted-foreground">
          <span>
            {#if running && currentCamera}
              <span class="font-semibold text-foreground">{currentCamera}</span> ·
              {currentMethod} ·
              {currentModel}
            {:else if currentStep === totalSteps && totalSteps > 0}
              Complete
            {/if}
          </span>
          <span>{currentStep}/{totalSteps} ({progressPct}%)</span>
        </div>
        <div class="h-2 w-full overflow-hidden rounded-full bg-muted">
          <div class="h-full bg-primary transition-all" style="width:{progressPct}%"></div>
        </div>
      </div>

      <!-- Live log feed -->
      {#if logLines.length > 0}
        <div class="bg-muted/50 rounded-md p-3 max-h-32 overflow-y-auto font-mono text-xs text-muted-foreground flex flex-col gap-0.5">
          {#each logLines as line, i}
            <div>{line}</div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  <!-- Results -->
  {#if allRows.length > 0}
    {#each Object.entries(groupedResults()).sort() as [camName, rows]}
      <div class="rounded-lg border p-4 flex flex-col gap-3">
        <button class="flex items-center gap-1 text-sm font-semibold text-primary"
          onclick={() => toggleCameraCollapse(camName)}>
          {#if collapsedCameras.has(camName)}<ChevronRight class="h-4 w-4" />{:else}<ChevronDown class="h-4 w-4" />{/if}
          {camName} ({rows.length} results)
        </button>

        {#if !collapsedCameras.has(camName)}
          <table class="text-sm">
            <thead>
              <tr class="text-xs text-muted-foreground border-b">
                <th class="text-left py-1.5 pr-3">Prompt</th>
                <th class="text-left py-1.5 pr-3">Method</th>
                <th class="text-left py-1.5 pr-3">Model</th>
                <th class="text-right py-1.5 pr-3 cursor-pointer select-none hover:text-primary" onclick={toggleTimeUnit} title="Click to toggle s/ms">Capture</th>
                {#if withVision}
                  <th class="text-right py-1.5 pr-3 cursor-pointer select-none hover:text-primary" onclick={toggleTimeUnit} title="Click to toggle s/ms">Vision</th>
                  <th class="text-right py-1.5 pr-3 cursor-pointer select-none hover:text-primary" onclick={toggleTimeUnit} title="Click to toggle s/ms">Total</th>
                {/if}
                <th class="text-right py-1.5 pr-3">Res</th>
                <th class="text-right py-1.5 pr-3">Size</th>
                <th class="text-left py-1.5">Status</th>
              </tr>
            </thead>
            <tbody>
              {#each rows as row}
                <tr class="border-b last:border-0">
                  <td class="py-1 pr-3 text-xs text-muted-foreground italic max-w-[150px] truncate" title={row.prompt}>
                    {row.prompt.length > 30 ? row.prompt.slice(0, 30) + '…' : row.prompt}
                  </td>
                  <td
                    class="py-1 pr-3 font-mono text-xs {row.result.image ? 'cursor-help' : ''}"
                    onmouseenter={(e) => { if (row.result.image) { hoveredImage = row.result.image; hoverX = e.clientX; hoverY = e.clientY } }}
                    onmousemove={(e) => { if (hoveredImage) { hoverX = e.clientX; hoverY = e.clientY } }}
                    onmouseleave={() => { hoveredImage = null }}
                  >{row.method}</td>
                  <td class="py-1 pr-3 font-mono text-xs max-w-[150px] truncate" title={row.model}>{row.model}</td>
                  <td class="py-1 pr-3 text-right font-mono text-xs cursor-pointer hover:text-primary {row.result.ok ? '' : 'text-muted-foreground'}" onclick={toggleTimeUnit} title="Toggle s/ms">
                    {row.result.ok ? fmtTime(row.result.snap_sec) : '—'}
                  </td>
                  {#if withVision}
                    <td class="py-1 pr-3 text-right font-mono text-xs cursor-pointer hover:text-primary {row.result.ok ? '' : 'text-muted-foreground'}" onclick={toggleTimeUnit} title="Toggle s/ms">
                      {row.result.ok && row.result.vision_sec ? fmtTime(row.result.vision_sec) : '—'}
                    </td>
                    <td class="py-1 pr-3 text-right font-mono text-xs cursor-pointer hover:text-primary {row.result.ok ? '' : 'text-muted-foreground'}" onclick={toggleTimeUnit} title="Toggle s/ms">
                      {row.result.ok ? fmtTime(row.result.total_sec) : '—'}
                    </td>
                  {/if}
                  <td class="py-1 pr-3 text-right font-mono text-xs text-muted-foreground">
                    {row.result.ok ? fmtRes(row.result) : '—'}
                  </td>
                  <td class="py-1 pr-3 text-right font-mono text-xs text-muted-foreground">
                    {row.result.ok ? fmtBytes(row.result.bytes) : '—'}
                  </td>
                  <td class="py-1 text-xs {row.result.ok ? 'text-green-600' : 'text-destructive'}">
                    {row.result.ok ? '✓' : '✗'}
                  </td>
                </tr>
                {#if withVision && row.result.ok && row.result.preview}
                  <tr class="border-b last:border-0">
                    <td colspan={withVision ? 9 : 7} class="py-1 pr-3 text-xs text-muted-foreground italic pl-8">
                      {row.result.preview}
                    </td>
                  </tr>
                {/if}
              {/each}
            </tbody>
          </table>
        {/if}
      </div>
    {/each}
  {/if}

  <!-- Hover image preview -->
  <HoverPreview bind:hoveredImage bind:hoverX bind:hoverY />
</div>
