<script>
  import { Camera, Eye, Loader2, RefreshCw, Save, Sparkles, Upload, Trash2, Bookmark } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { Textarea } from '$lib/components/ui/textarea'
  import CopyButton from '$lib/components/CopyButton.svelte'
  import Markdown from '$lib/components/Markdown.svelte'
  import { buildCurl } from '$lib/curl.svelte'
  import { apiClient } from '$lib/api'
  import { Tooltip } from '$lib/components/ui/tooltip'
  import { formatTimings, timingTooltipContent, isMobile } from '$lib/utils'

  let { cameras = [], globalPrompt = '', onSavePrompt } = $props()

  let selectedCamera = $state('')
  let prompt = $state(globalPrompt)
  let image = $state('') // base64 data URI
  let description = $state('')
  let visionTiming = $state('') // compact timing breakdown from last vision run
  let visionTimingsRaw = $state(undefined)
  let visionTotalMs = $state(undefined)
  let visionTtfsMs = $state(undefined)
  let desktop = $state(!isMobile())
  let busy = $state(false)
  let status = $state('')
  let statusType = $state('ok')
  let results = $state([]) // history of { prompt, description, time }
  let statusTimeout

  // Prompt presets
  let presets = $state([])
  let presetName = $state('')
  let showSavePreset = $state(false)

  // Update prompt when globalPrompt prop changes
  $effect(() => {
    if (!prompt && globalPrompt) prompt = globalPrompt
  })

  function setStatus(msg, type = 'ok') {
    status = msg
    statusType = type
    clearTimeout(statusTimeout)
    statusTimeout = setTimeout(() => (status = ''), 5000)
  }

  // --- Prompt presets ---
  async function loadPresets() {
    try {
      presets = await apiClient.listVisionPrompts() ?? []
    } catch (e) { /* ignore */ }
  }

  loadPresets()

  async function savePreset() {
    if (!presetName || !prompt) return
    try {
      await apiClient.saveVisionPrompt({ name: presetName, prompt })
      presetName = ''
      showSavePreset = false
      await loadPresets()
      setStatus('✓ Prompt preset saved')
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    }
  }

  async function deletePreset(name) {
    try {
      await apiClient.deleteVisionPrompt(name)
      await loadPresets()
      setStatus('✓ Preset deleted')
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    }
  }

  function loadPreset(p) {
    prompt = p.prompt
    setStatus(`Loaded preset: ${p.name}`)
  }

  // --- Image upload ---
  let fileInput = $state(null)

  async function onFileUpload(e) {
    const file = e.target.files?.[0]
    if (!file) return
    if (!file.type.startsWith('image/')) {
      setStatus('Please upload an image file', 'err')
      return
    }
    busy = true
    status = ''
    try {
      setStatus('Analyzing uploaded image…')
      const fd = new FormData()
      fd.append('prompt', prompt)
      fd.append('image', file)
      const res = await apiClient.visionTest(fd)
      const data = await res.json()
      image = data.image || ''
      description = data.description || ''
      visionTiming = formatTimings(data.timings)
      visionTimingsRaw = data.timings
      visionTotalMs = data.total_ms
      visionTtfsMs = data.ttfs_ms
      results = [{ prompt, description, time: new Date().toLocaleTimeString() }, ...results].slice(0, 10)
      setStatus('✓ Done')
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    } finally {
      busy = false
      if (fileInput) fileInput.value = ''
    }
  }

  // --- Vision execution ---
  async function runVision(capture = false) {
    if (!selectedCamera && !image) {
      setStatus('Select a camera first', 'err')
      return
    }
    busy = true
    status = ''
    try {
      const body = { prompt }
      if (capture || !image) {
        body.camera = selectedCamera
      } else {
        body.image = image
        body.camera = selectedCamera
      }
      setStatus(capture || !image ? 'Capturing + analyzing…' : 'Analyzing…')
      const data = await apiClient.visionTestJSON(body)
      image = data.image || image
      description = data.description || ''
      visionTiming = formatTimings(data.timings)
      visionTimingsRaw = data.timings
      visionTotalMs = data.total_ms
      visionTtfsMs = data.ttfs_ms
      results = [{ prompt, description, time: new Date().toLocaleTimeString() }, ...results].slice(0, 10)
      setStatus('✓ Done')
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    } finally {
      busy = false
    }
  }

  function captureAndRun() {
    image = ''
    description = ''
    visionTiming = ''
    visionTimingsRaw = undefined
    visionTotalMs = undefined
    visionTtfsMs = undefined
    runVision(true)
  }

  function reRun() {
    if (!image) {
      runVision(true)
      return
    }
    runVision(false)
  }

  function saveAsGlobal() {
    if (!prompt || !onSavePrompt) return
    onSavePrompt(prompt)
    setStatus('✓ Saved as global default prompt')
  }

  function clearAll() {
    image = ''
    description = ''
    visionTiming = ''
    visionTimingsRaw = undefined
    visionTotalMs = undefined
    visionTtfsMs = undefined
    results = []
    status = ''
  }

  // --- Test All Models ---
  let allResults = $state([])   // [{model, pending, description, error, ttfs_ms, gen_ms, total_ms}]
  let allBusy = $state(false)
  let allStatus = $state('')
  let allDoneCount = $state(0)
  let allModelCount = $state(0)

  async function runTestAll() {
    if (!selectedCamera && !image) {
      setStatus('Select a camera or capture an image first', 'err')
      return
    }
    allBusy = true
    allResults = []
    allStatus = ''
    allDoneCount = 0
    allModelCount = 0

    const body = { prompt, image: undefined, camera: undefined }
    if (image) { body.image = image; body.camera = selectedCamera }
    else { body.camera = selectedCamera }

    try {
      const resp = await fetch('/api/vision/test-all/stream', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
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

          if (ev.type === 'image') {
            if (!image && ev.image) image = ev.image
          } else if (ev.type === 'models') {
            allModelCount = ev.models.length
            allResults = ev.models.map(m => ({ model: m, pending: true }))
          } else if (ev.type === 'result') {
            allDoneCount++
            allResults = allResults.map(r =>
              r.model === ev.model ? { ...ev, pending: false } : r
            )
          } else if (ev.type === 'done') {
            allStatus = `Done — ${ev.count} model${ev.count === 1 ? '' : 's'} tested`
          }
        }
      }
    } catch (e) {
      allStatus = '✗ ' + e.message
    } finally {
      allBusy = false
    }
  }

  let curlCommand = $derived(
    buildCurl('POST', '/api/vision/test', image
      ? { camera: selectedCamera, prompt, image: '[base64 image data]' }
      : { camera: selectedCamera, prompt })
  )
</script>

<div class="flex flex-col gap-5 max-w-4xl">
  <!-- Header -->
  <div>
    <h2 class="text-lg font-semibold text-primary mb-1">Vision Playground</h2>
    <p class="text-sm text-muted-foreground">
      Capture a snapshot from a camera or upload an image, then test vision prompts against one model or all available models.
      Save your favorite prompts as presets for later reuse.
    </p>
  </div>

  <!-- Controls row -->
  <div class="flex flex-wrap items-end gap-3">
    <label class="flex flex-col gap-1 text-sm text-muted-foreground">
      Camera
      <select bind:value={selectedCamera} disabled={busy}
        class="rounded-md border border-input bg-transparent px-3 py-2 text-sm disabled:opacity-50 min-w-[160px]">
        <option value="">— select —</option>
        {#each cameras as cam}
          <option value={cam.name}>{cam.name}</option>
        {/each}
      </select>
    </label>

    <Button onclick={captureAndRun} disabled={busy || !selectedCamera} title="Capture fresh snapshot and run vision">
      {#if busy && status.toLowerCase().includes('captur')}
        <Loader2 class="h-4 w-4 animate-spin" />
      {:else}
        <Camera class="h-4 w-4" />
      {/if}
      Capture & Analyze
    </Button>

    <!-- Upload button -->
    <label class="inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium
      transition-colors border border-input bg-background hover:bg-accent hover:text-accent-foreground
      h-9 px-4 cursor-pointer disabled:opacity-50" title="Upload an image file to test against">
      <Upload class="h-4 w-4" />
      Upload Image
      <input bind:this={fileInput} type="file" accept="image/*" class="hidden" onchange={onFileUpload} disabled={busy} />
    </label>

    {#if image}
      <Button variant="outline" onclick={reRun} disabled={busy} title="Re-run vision on the same image with current prompt">
        {#if busy && !status.toLowerCase().includes('captur')}
          <Loader2 class="h-4 w-4 animate-spin" />
        {:else}
          <RefreshCw class="h-4 w-4" />
        {/if}
        Re-run with new prompt
      </Button>
    {/if}

    <Button variant="outline" onclick={runTestAll} disabled={allBusy || busy || (!selectedCamera && !image)}
      title="Run the current prompt against every model on the vision endpoint and compare results">
      {#if allBusy}
        <Loader2 class="h-4 w-4 animate-spin" />
      {:else}
        <Sparkles class="h-4 w-4" />
      {/if}
      Test All Models
    </Button>

    {#if image}
      <Button variant="ghost" onclick={clearAll} disabled={busy} title="Clear snapshot and results">
        Clear
      </Button>
    {/if}
  </div>

  <!-- Prompt presets bar -->
  <div class="flex flex-wrap items-center gap-2">
    <span class="text-xs font-semibold text-muted-foreground">Presets:</span>
    {#if presets.length === 0}
      <span class="text-xs text-muted-foreground italic">No saved presets yet</span>
    {:else}
      {#each presets as p (p.name)}
        <div class="flex items-center gap-0.5 rounded-md border bg-card text-xs">
          <button onclick={() => loadPreset(p)} disabled={busy}
            class="px-2 py-1 hover:bg-accent rounded-l-md disabled:opacity-50" title={p.prompt}>
            {p.name}
          </button>
          <button onclick={() => deletePreset(p.name)} disabled={busy}
            class="px-1 py-1 hover:bg-destructive/10 rounded-r-md disabled:opacity-50" title="Delete preset">
            <Trash2 class="h-3 w-3" />
          </button>
        </div>
      {/each}
    {/if}
    <Button variant="ghost" size="sm" onclick={() => showSavePreset = !showSavePreset} disabled={busy || !prompt}
      title="Save current prompt as a named preset">
      <Bookmark class="h-3.5 w-3.5" />
      Save as Preset
    </Button>
  </div>

  <!-- Save preset inline form -->
  {#if showSavePreset}
    <div class="flex gap-2 items-center">
      <Input bind:value={presetName} placeholder="Preset name…" class="max-w-[200px] text-sm" />
      <Button size="sm" onclick={savePreset} disabled={!presetName || !prompt}>Save</Button>
      <Button variant="ghost" size="sm" onclick={() => showSavePreset = false}>Cancel</Button>
    </div>
  {/if}

  <!-- Snapshot + prompt side by side -->
  {#if image || busy}
    <div class="flex gap-4 flex-col md:flex-row">
      <!-- Snapshot -->
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

      <!-- Prompt editor -->
      <div class="flex-1 min-w-0 flex flex-col gap-2">
        <div class="flex items-center justify-between">
          <p class="text-xs font-semibold text-muted-foreground">Vision Prompt</p>
          {#if globalPrompt && prompt !== globalPrompt}
            <Button variant="ghost" size="sm" onclick={() => prompt = globalPrompt} title="Reset to global default">
              Reset
            </Button>
          {/if}
        </div>
        <Textarea
          bind:value={prompt}
          rows="6"
          placeholder="e.g. Describe what you see in one or two sentences. Be concise and factual."
          disabled={busy}
          class="text-sm"
        />
        <div class="flex gap-2 flex-wrap">
          <Button variant="secondary" size="sm" onclick={reRun} disabled={busy || !image}
            title="Run vision on the same image with this prompt">
            <Sparkles class="h-3.5 w-3.5" />
            Test Prompt
          </Button>
          <Button variant="outline" size="sm" onclick={saveAsGlobal} disabled={busy || !prompt}
            title="Save this prompt as the global default">
            <Save class="h-3.5 w-3.5" />
            Save as Global Default
          </Button>
          <CopyButton
            text={curlCommand}
            label="Copy curl — vision test endpoint"
            preview previewType="curl"
            size="sm"
          />
        </div>
      </div>
    </div>
  {/if}

  <!-- Status -->
  {#if status}
    <div class="text-sm {statusType === 'err' ? 'text-destructive' : 'text-primary'}">
      {status}
    </div>
  {/if}

  <!-- Results history -->
  {#if results.length > 0}
    <div class="flex flex-col gap-2">
      <h3 class="text-sm font-semibold text-foreground">Prompt History ({results.length})</h3>
      <div class="flex flex-col gap-2">
        {#each results as r, i (r.time + i)}
          <div class="rounded-lg border bg-card p-3 flex flex-col gap-1.5">
            <div class="flex items-center justify-between gap-2">
              <span class="text-xs text-muted-foreground font-mono">{r.time}</span>
              {#if i === 0}
                <span class="text-xs text-primary font-medium">latest</span>
              {/if}
            </div>
            <p class="text-xs text-muted-foreground italic">"{r.prompt || '(empty — hardcoded default)'}"</p>
            <Markdown content={r.description} class="text-sm text-foreground" />
            {#if i === 0 && visionTiming}
              {#if desktop}
                <Tooltip
                  content={timingTooltipContent(visionTimingsRaw, visionTotalMs, visionTtfsMs)}
                  multiline
                  side="bottom"
                  class="text-xs"
                >
                  <p class="text-xs text-muted-foreground cursor-help w-fit">⏱ {visionTiming}</p>
                </Tooltip>
              {:else}
                <p class="text-xs text-muted-foreground">⏱ {visionTiming}</p>
              {/if}
            {/if}
            {#if i === 0}
              <div class="flex gap-1.5 mt-1">
                <Button variant="ghost" size="sm" onclick={() => prompt = r.prompt} title="Load this prompt into the editor">
                  Use this prompt
                </Button>
              </div>
            {/if}
          </div>
        {/each}
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
            <span class="ml-2 text-xs font-normal text-muted-foreground">
              {allDoneCount}/{allModelCount}
            </span>
          {/if}
        </h3>
        {#if allStatus}
          <span class="text-xs text-muted-foreground">{allStatus}</span>
        {/if}
      </div>

      {#if allResults.length > 0}
        <div class="grid gap-3 sm:grid-cols-2">
          {#each allResults as r (r.model)}
            <div class="rounded-lg border bg-card p-3 flex flex-col gap-2
              {r.pending ? 'opacity-60' : ''}">

              <!-- Model name + status -->
              <div class="flex items-center gap-2">
                {#if r.pending}
                  <Loader2 class="h-3.5 w-3.5 shrink-0 animate-spin text-muted-foreground" />
                {:else if r.error}
                  <span class="text-destructive text-xs shrink-0">✗</span>
                {:else}
                  <span class="text-primary text-xs shrink-0">✓</span>
                {/if}
                <span class="text-xs font-mono font-semibold text-foreground truncate" title={r.model}>{r.model}</span>
              </div>

              {#if r.pending}
                <p class="text-xs text-muted-foreground italic">Waiting…</p>
              {:else if r.error}
                <p class="text-xs text-destructive">{r.error}</p>
              {:else}
                <!-- Timing bar -->
                {#if r.total_ms > 0}
                  {@const prefillPct = Math.round((r.ttfs_ms / r.total_ms) * 100)}
                  {@const genPct = 100 - prefillPct}
                  <div class="flex flex-col gap-0.5">
                    <div class="flex h-2 w-full overflow-hidden rounded-full bg-muted">
                      <div class="bg-primary/70 h-full" style="width:{prefillPct}%"></div>
                      <div class="bg-primary/30 h-full" style="width:{genPct}%"></div>
                    </div>
                    <div class="flex justify-between text-[10px] text-muted-foreground">
                      <span title="Load + image encode + prefill">⚙ {r.ttfs_ms}ms setup</span>
                      <span title="Token generation">✍ {r.gen_ms}ms write</span>
                      <span title="Total wall-clock">⏱ {(r.total_ms/1000).toFixed(1)}s</span>
                    </div>
                  </div>
                {/if}
                <!-- Description -->
                <Markdown content={r.description ?? ''} class="text-sm text-foreground" />
              {/if}
            </div>
          {/each}
        </div>
      {:else if allBusy}
        <div class="flex items-center gap-2 text-sm text-muted-foreground">
          <Loader2 class="h-4 w-4 animate-spin" />
          Fetching model list…
        </div>
      {/if}
    </div>
  {/if}

  {#if !image && !busy}
    <div class="rounded-lg border border-dashed p-8 text-center text-muted-foreground">
      <Camera class="h-8 w-8 mx-auto mb-2 opacity-50" />
      <p>Select a camera and click "Capture & Analyze", or click "Upload Image" to start testing vision prompts.</p>
    </div>
  {/if}
</div>
