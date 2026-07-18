<script>
  import { Camera, Eye, Loader2, RefreshCw, Save, Sparkles } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { Textarea } from '$lib/components/ui/textarea'
  import CopyButton from '$lib/components/CopyButton.svelte'
  import { buildCurl } from '$lib/curl.svelte'

  let { cameras = [], globalPrompt = '', onSavePrompt } = $props()

  let selectedCamera = $state('')
  let prompt = $state(globalPrompt)
  let image = $state('') // base64 data URI
  let description = $state('')
  let busy = $state(false)
  let status = $state('')
  let statusType = $state('ok')
  let results = $state([]) // history of { prompt, description, time }
  let statusTimeout

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
        body.image = image // reuse cached image
        body.camera = selectedCamera // for logging
      }
      setStatus(capture || !image ? 'Capturing + analyzing…' : 'Analyzing…')
      const res = await fetch('/api/vision/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      if (!res.ok) throw new Error(await res.text())
      const data = await res.json()
      image = data.image || image
      description = data.description || ''
      results = [{ prompt, description, time: new Date().toLocaleTimeString() }, ...results].slice(0, 10)
      setStatus('✓ Done')
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    } finally {
      busy = false
    }
  }

  function captureAndRun() {
    image = '' // force re-capture
    description = ''
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
    results = []
    status = ''
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
    <h2 class="text-lg font-semibold text-primary mb-1">Vision Prompt Test</h2>
    <p class="text-sm text-muted-foreground">
      Capture a snapshot from a camera, then test different vision prompts against it.
      Refine your prompt until you get good descriptions, then save it as the global default.
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

    {#if image}
      <Button variant="ghost" onclick={clearAll} disabled={busy} title="Clear snapshot and results">
        Clear
      </Button>
    {/if}
  </div>

  <!-- Snapshot + prompt side by side -->
  {#if image || busy}
    <div class="flex gap-4 flex-col md:flex-row">
      <!-- Snapshot -->
      <div class="flex-1 min-w-0">
        <p class="text-xs font-semibold text-muted-foreground mb-1.5">Snapshot</p>
        <div class="relative rounded-lg border overflow-hidden">
          {#if image}
            <img src={image} alt="Camera snapshot" class="w-full" />
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
            <p class="text-sm text-foreground">{r.description}</p>
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

  {#if !image && !busy}
    <div class="rounded-lg border border-dashed p-8 text-center text-muted-foreground">
      <Camera class="h-8 w-8 mx-auto mb-2 opacity-50" />
      <p>Select a camera and click "Capture & Analyze" to start testing vision prompts.</p>
    </div>
  {/if}
</div>
