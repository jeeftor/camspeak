<script lang="ts">
  import { Camera, Loader2, RefreshCw, Sparkles, Upload } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import CopyButton from '$lib/components/CopyButton.svelte'
  import Markdown from '$lib/components/Markdown.svelte'
  import { buildCurl } from '$lib/curl.svelte'
  import { apiClient } from '$lib/api'
  import { isVisionCapableModel } from '$lib/models'
  import { Tooltip } from '$lib/components/ui/tooltip'
  import { formatTimings, timingTooltipContent, isMobile } from '$lib/utils'
  import CameraSelect from '$lib/components/CameraSelect.svelte'
  import PromptEditor from '$lib/components/PromptEditor.svelte'

  let { cameras = [], globalPrompt = '', onSavePrompt } = $props()

  let selectedCamera = $state('')
  let selectedStream = $state('')
  let prompt = $state(globalPrompt)
  let image = $state('')
  let description = $state('')
  let visionTiming = $state('')
  let visionTimingsRaw = $state(undefined)
  let visionTotalMs = $state(undefined)
  let visionTtfsMs = $state(undefined)
  let desktop = $state(!isMobile())
  let busy = $state(false)
  let status = $state('')
  let statusType = $state('ok')
  let results = $state([])
  let statusTimeout

  let configuredModel = $state('')
  let selectedModel = $state('')
  let availableModels = $state([])
  let modelsLoading = $state(false)

  let presets = $state([])

  $effect(() => {
    if (!prompt && globalPrompt) prompt = globalPrompt
  })

  function setStatus(msg, type = 'ok') {
    status = msg
    statusType = type
    clearTimeout(statusTimeout)
    statusTimeout = setTimeout(() => (status = ''), 5000)
  }

  async function loadPresets() {
    try { presets = await apiClient.listVisionPrompts() ?? [] } catch { /* ignore */ }
  }
  loadPresets()

  async function loadConfiguredModel() {
    try {
      const cfg = await apiClient.getVisionConfig()
      configuredModel = cfg.model || ''
      if (!selectedModel) selectedModel = configuredModel
    } catch { /* ignore */ }
  }

  async function fetchModels() {
    modelsLoading = true
    try {
      const cfg = await apiClient.getVisionConfig()
      const res = await apiClient.testVisionConfig(cfg.url, cfg.api_key)
      if (res.ok && res.data?.data) {
        availableModels = res.data.data
          .map(m => m.id)
          .filter(m => isVisionCapableModel(m))
          .sort()
      }
    } catch (e) {
      setStatus('Failed to fetch models: ' + e.message, 'err')
    } finally {
      modelsLoading = false
    }
  }

  loadConfiguredModel()

  async function savePreset(name, promptValue) {
    if (!name || !promptValue) return
    try {
      await apiClient.saveVisionPrompt({ name, prompt: promptValue })
      await loadPresets()
      setStatus('Preset saved')
    } catch (e) { setStatus(e.message, 'err') }
  }

  async function deletePreset(name) {
    try { await apiClient.deleteVisionPrompt(name); await loadPresets() }
    catch (e) { setStatus(e.message, 'err') }
  }

  let fileInput = $state(null)

  async function onFileUpload(e) {
    const file = e.target.files?.[0]
    if (!file || !file.type.startsWith('image/')) return
    busy = true; status = ''
    try {
      const fd = new FormData()
      fd.append('prompt', prompt)
      if (selectedModel && selectedModel !== configuredModel) fd.append('model', selectedModel)
      fd.append('image', file)
      const res = await apiClient.visionTest(fd)
      const data = await res.json()
      image = data.image || ''
      description = data.description || ''
      visionTiming = formatTimings(data.timings)
      visionTimingsRaw = data.timings
      visionTotalMs = data.total_ms
      visionTtfsMs = data.ttfs_ms
      results = [{ prompt, description, time: new Date().toLocaleTimeString(), model: data.model || selectedModel || configuredModel }, ...results].slice(0, 10)
    } catch (e) { setStatus(e.message, 'err') }
    finally { busy = false; if (fileInput) fileInput.value = '' }
  }

  async function runVision(capture = false) {
    if (!selectedCamera && !image) { setStatus('Select a camera first', 'err'); return }
    busy = true; status = ''
    try {
      const body: any = { prompt }
      if (selectedModel && selectedModel !== configuredModel) body.model = selectedModel
      if (capture || !image) { body.camera = selectedCamera; if (selectedStream) body.stream = selectedStream }
      else { body.image = image; body.camera = selectedCamera }
      setStatus(capture || !image ? 'Capturing…' : 'Analyzing…')
      const data = await apiClient.visionTestJSON(body)
      image = data.image || image
      description = data.description || ''
      visionTiming = formatTimings(data.timings)
      visionTimingsRaw = data.timings
      visionTotalMs = data.total_ms
      visionTtfsMs = data.ttfs_ms
      results = [{ prompt, description, time: new Date().toLocaleTimeString(), model: data.model || selectedModel || configuredModel }, ...results].slice(0, 10)
      setStatus('Done')
    } catch (e) { setStatus(e.message, 'err') }
    finally { busy = false }
  }

  function captureAndRun() {
    image = ''; description = ''; visionTiming = ''
    visionTimingsRaw = undefined; visionTotalMs = undefined; visionTtfsMs = undefined
    runVision(true)
  }

  function clearAll() {
    image = ''; description = ''; visionTiming = ''
    visionTimingsRaw = undefined; visionTotalMs = undefined; visionTtfsMs = undefined
    results = []; status = ''
  }

  // --- Test All Models ---
  let allResults = $state([])
  let allBusy = $state(false)
  let allStatus = $state('')
  let allDoneCount = $state(0)
  let allModelCount = $state(0)

  async function runTestAll() {
    if (!selectedCamera && !image) { setStatus('Select a camera or capture an image first', 'err'); return }
    allBusy = true; allResults = []; allStatus = ''; allDoneCount = 0; allModelCount = 0
    const body: any = { prompt }
    if (image) { body.image = image; body.camera = selectedCamera }
    else { body.camera = selectedCamera; if (selectedStream) body.stream = selectedStream }
    try {
      const resp = await fetch('/api/vision/test-all/stream', {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
      })
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
      const reader = resp.body.getReader()
      const dec = new TextDecoder()
      let buf = ''
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        buf += dec.decode(value, { stream: true })
        const lines = buf.split('\n')
        buf = lines.pop()
        for (const line of lines) {
          if (!line.startsWith('data:')) continue
          let ev
          try { ev = JSON.parse(line.slice(5).trim()) } catch { continue }
          if (ev.type === 'image') { if (!image && ev.image) image = ev.image }
          else if (ev.type === 'models') {
            allModelCount = ev.models.length
            allResults = ev.models.map(m => ({ model: m, pending: true }))
          } else if (ev.type === 'result') {
            allDoneCount++
            allResults = allResults.map(r => r.model === ev.model ? { ...ev, pending: false } : r)
          } else if (ev.type === 'done') {
            allStatus = `Done — ${ev.count} model${ev.count === 1 ? '' : 's'}`
          }
        }
      }
    } catch (e) { allStatus = e.message }
    finally { allBusy = false }
  }

  function fmtMs(ms) {
    if (!ms || ms <= 0) return '0ms'
    if (ms < 1000) return `${ms}ms`
    const s = ms / 1000
    return `${s < 10 ? s.toFixed(2) : s.toFixed(1)}s`
  }

  let curlCommand = $derived(
    buildCurl('POST', '/api/vision/test', image
      ? { camera: selectedCamera, prompt, image: '[base64 image data]', ...(selectedModel && selectedModel !== configuredModel ? { model: selectedModel } : {}) }
      : { camera: selectedCamera, prompt, ...(selectedStream ? { stream: selectedStream } : {}), ...(selectedModel && selectedModel !== configuredModel ? { model: selectedModel } : {}) })
  )
</script>

<div class="flex flex-col gap-5 max-w-4xl">
  <!-- Header -->
  <div>
    <h2 class="text-lg font-semibold text-primary mb-1">Vision Playground</h2>
    <p class="text-sm text-muted-foreground">
      Capture a snapshot or upload an image, then test vision prompts. Compare models side-by-side.
    </p>
  </div>

  <!-- Prompt editor (always visible) -->
  <PromptEditor
    bind:value={prompt}
    {presets}
    {globalPrompt}
    showPresetButtons={true}
    disabled={busy}
    placeholder="e.g. Describe what you see in one or two sentences."
    onSavePreset={savePreset}
    onDeletePreset={deletePreset}
    onSetGlobal={onSavePrompt}
    onResetGlobal={() => (prompt = globalPrompt)}
  />

  <!-- Controls row -->
  <div class="flex flex-wrap items-end gap-3">
    <label class="flex flex-col gap-1 text-sm text-muted-foreground">
      Camera
      <CameraSelect bind:value={selectedCamera} {cameras} disabled={busy} class="min-w-[160px]" />
    </label>

    {#if selectedCamera && (cameras.find(c => c.name === selectedCamera)?.type === 'hikvision' || cameras.find(c => c.name === selectedCamera)?.type === 'reolink')}
      <label class="flex flex-col gap-1 text-sm text-muted-foreground">
        Stream
        <select bind:value={selectedStream} disabled={busy}
          class="rounded-md border border-input bg-transparent px-3 py-2 text-sm disabled:opacity-50 min-w-[100px]">
          <option value="">sub (auto)</option>
          <option value="main">main</option>
          <option value="sub">sub</option>
        </select>
      </label>
    {/if}

    <label class="flex flex-col gap-1 text-sm text-muted-foreground">
      Model
      <div class="flex items-center gap-1">
        <select bind:value={selectedModel} disabled={busy}
          class="rounded-md border border-input bg-transparent px-3 py-2 text-sm disabled:opacity-50 min-w-[180px] max-w-[280px]"
          title={selectedModel || configuredModel || 'No model configured'}>
          {#if configuredModel && !availableModels.includes(configuredModel)}
            <option value={configuredModel}>{configuredModel} (configured)</option>
          {/if}
          {#each availableModels as m}
            <option value={m}>{m}</option>
          {/each}
          {#if availableModels.length === 0 && !configuredModel}
            <option value="">— not configured —</option>
          {/if}
        </select>
        <button onclick={(e) => { e.preventDefault(); if (!modelsLoading) fetchModels() }}
          disabled={busy || modelsLoading}
          class="inline-flex items-center justify-center h-9 w-9 rounded-md border border-input bg-background hover:bg-accent disabled:opacity-50 shrink-0"
          title="Fetch available models">
          {#if modelsLoading}<Loader2 class="h-4 w-4 animate-spin" />{:else}<RefreshCw class="h-4 w-4" />{/if}
        </button>
      </div>
    </label>

    <Button onclick={captureAndRun} disabled={busy || !selectedCamera}>
      {#if busy && status.toLowerCase().includes('captur')}<Loader2 class="h-4 w-4 animate-spin" />{:else}<Camera class="h-4 w-4" />{/if}
      Capture & Analyze
    </Button>

    <label class="inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium
      transition-colors border border-input bg-background hover:bg-accent h-9 px-4 cursor-pointer disabled:opacity-50">
      <Upload class="h-4 w-4" /> Upload
      <input bind:this={fileInput} type="file" accept="image/*" class="hidden" onchange={onFileUpload} disabled={busy} />
    </label>

    <Button variant="outline" onclick={runTestAll} disabled={allBusy || busy || (!selectedCamera && !image)}>
      {#if allBusy}<Loader2 class="h-4 w-4 animate-spin" />{:else}<Sparkles class="h-4 w-4" />{/if}
      Test All Models
    </Button>

    {#if image}
      <Button variant="outline" onclick={() => runVision(false)} disabled={busy}>
        <RefreshCw class="h-4 w-4" /> Re-run
      </Button>
      <Button variant="ghost" onclick={clearAll} disabled={busy}>Clear</Button>
    {/if}
  </div>

  <!-- Status -->
  {#if status}
    <div class="text-sm {statusType === 'err' ? 'text-destructive' : 'text-primary'}">{status}</div>
  {/if}

  <!-- Image + description -->
  {#if image || busy}
    <div class="flex gap-4 flex-col md:flex-row">
      <div class="flex-1 min-w-0">
        <p class="text-xs font-semibold text-muted-foreground mb-1.5">Image</p>
        <div class="relative rounded-lg border overflow-hidden">
          {#if image}
            <img src={image} alt="Vision test image" class="w-full" />
          {:else}
            <div class="flex items-center justify-center h-48 bg-muted">
              <Loader2 class="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          {/if}
        </div>
      </div>
      <div class="flex-1 min-w-0 flex flex-col gap-2">
        <p class="text-xs font-semibold text-muted-foreground">Result</p>
        {#if description}
          <Markdown content={description} class="text-sm text-foreground" />
          {#if visionTiming}
            {#if desktop}
              <Tooltip content={timingTooltipContent(visionTimingsRaw, visionTotalMs, visionTtfsMs)} multiline side="bottom" class="text-xs">
                <p class="text-xs text-muted-foreground cursor-help w-fit">⏱ {visionTiming}</p>
              </Tooltip>
            {:else}
              <p class="text-xs text-muted-foreground">⏱ {visionTiming}</p>
            {/if}
          {/if}
        {:else if busy}
          <p class="text-sm text-muted-foreground">Analyzing…</p>
        {/if}
        <CopyButton text={curlCommand} label="Copy curl" preview previewType="curl" size="sm" />
      </div>
    </div>
  {/if}

  <!-- Test All Models results -->
  {#if allBusy || allResults.length > 0}
    <div class="flex flex-col gap-3">
      <div class="flex items-center justify-between gap-4">
        <h3 class="text-sm font-semibold text-foreground">
          Model Comparison
          {#if allBusy && allModelCount > 0}
            <span class="ml-2 text-xs font-normal text-muted-foreground">{allDoneCount}/{allModelCount}</span>
          {/if}
        </h3>
        {#if allStatus}<span class="text-xs text-muted-foreground">{allStatus}</span>{/if}
      </div>

      {#if allResults.length > 0}
        <div class="grid gap-3 sm:grid-cols-2">
          {#each allResults as r (r.model)}
            <div class="rounded-lg border bg-card p-3 flex flex-col gap-2 {r.pending ? 'opacity-60' : ''}">
              <div class="flex items-center gap-2">
                {#if r.pending}<Loader2 class="h-3.5 w-3.5 shrink-0 animate-spin text-muted-foreground" />
                {:else if r.error}<span class="text-destructive text-xs shrink-0">✗</span>
                {:else}<span class="text-primary text-xs shrink-0">✓</span>{/if}
                <span class="text-xs font-mono font-semibold text-foreground truncate" title={r.model}>{r.model}</span>
              </div>
              {#if r.pending}
                <p class="text-xs text-muted-foreground italic">Waiting…</p>
              {:else if r.error}
                <p class="text-xs text-destructive">{r.error}</p>
              {:else}
                {#if r.total_ms > 0}
                  {@const prefillPct = Math.round((r.ttfs_ms / r.total_ms) * 100)}
                  {@const genPct = 100 - prefillPct}
                  <div class="flex flex-col gap-0.5">
                    <div class="flex h-2.5 w-full overflow-hidden rounded-full bg-muted">
                      <div class="bg-amber-400 h-full transition-all" style="width:{prefillPct}%"></div>
                      <div class="bg-sky-500 h-full transition-all" style="width:{genPct}%"></div>
                    </div>
                    <div class="flex justify-between text-[10px]">
                      <span class="text-amber-500">⚙ {fmtMs(r.ttfs_ms)} setup</span>
                      <span class="text-sky-500">✍ {fmtMs(r.gen_ms)} write</span>
                      <span class="text-muted-foreground">⏱ {fmtMs(r.total_ms)}</span>
                    </div>
                  </div>
                {/if}
                <Markdown content={r.description ?? ''} class="text-sm text-foreground" />
              {/if}
            </div>
          {/each}
        </div>
      {:else if allBusy}
        <div class="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 class="h-4 w-4 animate-spin" /> Fetching model list…
        </div>
      {/if}
    </div>
  {/if}

  <!-- History -->
  {#if results.length > 0}
    <div class="flex flex-col gap-2">
      <h3 class="text-sm font-semibold text-foreground">History ({results.length})</h3>
      {#each results as r, i (r.time + i)}
        <div class="rounded-lg border bg-card p-3 flex flex-col gap-1.5">
          <div class="flex items-center justify-between gap-2">
            <span class="text-xs text-muted-foreground font-mono">{r.time}</span>
            <div class="flex items-center gap-2">
              {#if r.model}<span class="text-xs text-muted-foreground font-mono truncate max-w-[200px]" title={r.model}>{r.model}</span>{/if}
              {#if i === 0}<span class="text-xs text-primary font-medium">latest</span>{/if}
            </div>
          </div>
          <p class="text-xs text-muted-foreground italic">"{r.prompt || '(empty — default)'}"</p>
          <Markdown content={r.description} class="text-sm text-foreground" />
          {#if i === 0}
            <Button variant="ghost" size="sm" onclick={() => prompt = r.prompt}>Use this prompt</Button>
          {/if}
        </div>
      {/each}
    </div>
  {/if}

  {#if !image && !busy}
    <div class="rounded-lg border border-dashed p-8 text-center text-muted-foreground">
      <Camera class="h-8 w-8 mx-auto mb-2 opacity-50" />
      <p>Select a camera and click "Capture & Analyze", or click "Upload" to start.</p>
    </div>
  {/if}
</div>
