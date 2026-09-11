<script>
  import { onDestroy, untrack } from 'svelte'
  import { Sparkles, Save, Upload, Play, Pause, X, Loader2, Pencil, ArrowUp, ArrowDown, Radio, Wand2, Gauge, Plus, Square } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { Select } from '$lib/components/ui/select'
  import { Textarea } from '$lib/components/ui/textarea'
  import { toast } from '$lib/components/ui/toast'
  import { apiClient } from '$lib/api'
  import { formatMs, formatTimingSummary, formatSeconds } from '$lib/utils'
  import AudioPlayer from './AudioPlayer.svelte'
  import VoiceSelect from '$lib/components/VoiceSelect.svelte'
  import Modal from '$lib/components/Modal.svelte'

  let { presets = [], voices = [], onRefresh } = $props()

  let addOpen = $state(false)
  let tab = $state('upload')
  let cameras = $state([])
  let testCamera = $state('')
  let testingCamera = $state('')
  let testBusy = $state(false)
  let testStatus = $state('')
  let streamTestRequest = null
  let busy = $derived(genBusy || uploadBusy || streamBusy)

  async function openAdd() {
    currentAudio?.pause()
    playingKey = ''
    addOpen = true
    try { cameras = (await apiClient.getCameras()).filter(camera => camera.enabled !== false) }
    catch (e) { testStatus = `Camera list unavailable: ${e.message}` }
  }

  async function stopTest() {
    const camera = testingCamera
    if (!camera) return
    testBusy = true
    try {
      // Wait for the start request so a late response cannot restart a closed test.
      await streamTestRequest?.catch(() => {})
      await apiClient.stop(camera)
      testingCamera = ''
      testStatus = 'Test stopped'
    } catch (e) {
      testStatus = `Could not stop test: ${e.message}. Use Stop on the Cameras screen.`
      toast.error(testStatus)
    } finally { testBusy = false }
  }

  async function testStream() {
    if (!testCamera || !streamURL.trim() || testingCamera) return
    testingCamera = testCamera
    testBusy = true
    testStatus = 'Starting stream test…'
    try {
      streamTestRequest = apiClient.playStream({ camera: testCamera, url: streamURL.trim() })
      await streamTestRequest
      testStatus = `Stream sent to ${testingCamera}. Listen to the speaker, then stop the test.`
    } catch (e) {
      // A network error does not prove that playback failed; retain the Stop action.
      testStatus = `Test status unknown: ${e.message}. Stop the test before retrying.`
    } finally { testBusy = false; streamTestRequest = null }
  }

  $effect(() => {
    if (!addOpen) {
      untrack(() => {
        genAudioEl?.pause()
        uploadAudioEl?.pause()
        genPlaying = false
        if (testingCamera) void stopTest()
      })
    }
  })
  let genName = $state('')
  let genText = $state('')
  let genCategory = $state('alerts')
  let genVoice = $state('')
  let genBusy = $state(false)
  let genStatus = $state('')
  let genTimeout
  let genAudio = $state(null)
  let genAudioEl = $state(null)
  let genPlaying = $state(false)

  onDestroy(() => {
    if (genAudio) URL.revokeObjectURL(genAudio)
    if (genAudioEl) { genAudioEl.pause(); genAudioEl = null }
    if (uploadPreview) URL.revokeObjectURL(uploadPreview)
    clearTimeout(genTimeout)
    clearTimeout(statusTimeout)
    clearTimeout(uploadTimeout)
    uploadController?.abort()
    currentAudio?.pause()
    if (testingCamera) void stopTest()
  })

  let uploadName = $state('')
  let uploadCategory = $state('uploads')
  let uploadFile = $state(null)
  let uploadPreview = $state(null)
  let uploadAudioEl = $state(null)
  let uploadDragging = $state(false)
  let uploadBusy = $state(false)
  let uploadStatus = $state('')
  let libError = $state('')
  let statusTimeout
  let uploadTimeout

  // Upload progress dialog state
  let uploadProgress = $state(null) // null = no dialog, {step, percent, label} = active
  let uploadController = null

  let sortBy = $state('name')
  let sortOrder = $state('asc')

  // Stream preset state
  let streamName = $state('')
  let streamCategory = $state('streams')
  let streamURL = $state('')
  let streamBusy = $state(false)
  let streamStatus = $state('')

  // Per-preset gain state
  let gainBusyKey = $state('')       // which preset is being analyzed
  let gainEditKey = $state('')       // which preset's gain slider is open
  let gainEditValue = $state(1.0)     // slider value while editing
  let gainAnalysis = $state(null)    // {rms, suggested_gain} from last analyze

  let grouped = $derived((() => {
    const groups = presets.reduce((acc, p) => {
      ;(acc[p.category] ??= []).push(p)
      return acc
    }, {})
    const comparator = (a, b) => {
      let cmp = 0
      if (sortBy === 'name') cmp = a.name.localeCompare(b.name)
      else if (sortBy === 'duration') cmp = (a.duration ?? 0) - (b.duration ?? 0)
      return sortOrder === 'asc' ? cmp : -cmp
    }
    for (const cat in groups) {
      groups[cat].sort(comparator)
    }
    return groups
  })())

  async function generate() {
    if (!genText) return
    genBusy = true; genStatus = ''
    genAudioEl?.pause()
    if (genAudio) { URL.revokeObjectURL(genAudio); genAudio = null }
    genPlaying = false
    try {
      const res = await apiClient.ttsPreview({ text: genText, voice: genVoice })
      const ttsMs = res.headers.get('X-TTS-Ms')
      const blob = await res.blob()
      genAudio = URL.createObjectURL(blob)
      const ms = ttsMs ? formatMs(Number(ttsMs)) : ''
      genStatus = ms ? `✓ Generated (${ms}) — ready to preview or save` : '✓ Generated — ready to preview or save'
      genAudioEl = new Audio(genAudio)
      genAudioEl.onended = () => { genPlaying = false }
    } catch (e) {
      genStatus = '✗ ' + e.message
    } finally {
      genBusy = false
      clearTimeout(genTimeout); genTimeout = setTimeout(() => (genStatus = ''), 4000)
    }
  }

  async function togglePreview() {
    if (!genAudio || !genAudioEl) return
    if (genPlaying) { genAudioEl.pause(); genPlaying = false }
    else {
      try { await genAudioEl.play(); genPlaying = true }
      catch (e) { genStatus = `Preview failed: ${e.message}` }
    }
  }

  async function save() {
    if (!genName || !genText || !genAudio) return
    genBusy = true; genStatus = ''
    try {
      const data = await apiClient.savePreset({ name: genName, text: genText, category: genCategory, voice: genVoice })
      const timing = formatTimingSummary(data.timings, data.total_ms, data.ttfs_ms)
      genStatus = timing ? `✓ Saved (${timing})` : '✓ Saved'
      toast.success(`Preset "${genName}" saved`)
      genName = ''; genText = ''
      genAudioEl?.pause()
      if (genAudio) { URL.revokeObjectURL(genAudio); genAudio = null }
      genPlaying = false
      onRefresh()
    } catch (e) {
      genStatus = '✗ ' + e.message
    } finally {
      genBusy = false
      clearTimeout(statusTimeout); statusTimeout = setTimeout(() => (genStatus = ''), 4000)
    }
  }

  async function saveStream() {
    if (!streamName || !streamURL) return
    streamBusy = true; streamStatus = ''
    try {
      await apiClient.savePreset({ name: streamName, url: streamURL, category: streamCategory })
      streamStatus = '✓ Saved'
      toast.success(`Stream preset "${streamName}" saved`)
      streamName = ''; streamURL = ''
      onRefresh()
    } catch (e) {
      streamStatus = '✗ ' + e.message
    } finally {
      streamBusy = false
      clearTimeout(statusTimeout); statusTimeout = setTimeout(() => (streamStatus = ''), 4000)
    }
  }

  function handleUploadFile(file) {
    uploadFile = file
    if (file) {
      uploadName = file.name.replace(/\.[^/.]+$/, '')
      if (uploadPreview) URL.revokeObjectURL(uploadPreview)
      uploadPreview = URL.createObjectURL(file)
    } else {
      if (uploadPreview) URL.revokeObjectURL(uploadPreview)
      uploadPreview = null
    }
  }

  function clearUpload() {
    uploadName = ''
    uploadCategory = 'uploads'
    uploadFile = null
    if (uploadPreview) URL.revokeObjectURL(uploadPreview)
    uploadPreview = null
  }

  async function upload() {
    if (!uploadName || !uploadFile) return
    uploadBusy = true; uploadStatus = ''
    uploadProgress = { step: 'uploading', percent: 0, label: 'Uploading' }
    try {
      uploadController = new AbortController()
      await apiClient.uploadAndWait(uploadFile, uploadName, uploadCategory,
        (progress) => { uploadProgress = progress }, uploadController.signal)

      uploadStatus = '✓ Uploaded'
      toast.success(`Preset "${uploadName}" uploaded`)
      clearUpload()
      onRefresh()
    } catch (e) {
      if (e.name === 'AbortError') return
      uploadStatus = '✗ ' + e.message
      toast.error(`Upload failed: ${e.message}`)
    } finally {
      uploadProgress = null
      uploadBusy = false
      uploadController = null
      clearTimeout(uploadTimeout); uploadTimeout = setTimeout(() => (uploadStatus = ''), 4000)
    }
  }

  async function deletePreset(category, name) {
    if (!confirm(`Delete ${category}/${name}?`)) return
    try {
      await apiClient.deletePreset(category, name)
      toast.success(`Preset "${name}" deleted`)
      onRefresh()
    } catch (e) {
      libError = '✗ ' + e.message
      toast.error(`Failed to delete preset: ${e.message}`)
    }
  }

  let currentAudio = $state(null)
  let playingKey = $state('')

  // Rename state
  let editingKey = $state('')
  let editName = $state('')
  let editCategory = $state('')
  let renameStatus = $state('')

  function startRename(p) {
    const key = `${p.category}/${p.name}`
    if (editingKey === key) {
      editingKey = ''
      return
    }
    editingKey = key
    editName = p.name
    editCategory = p.category
    renameStatus = ''
  }

  async function doRename(oldCategory, oldName) {
    renameStatus = ''
    try {
      await apiClient.renamePreset(oldCategory, oldName, { name: editName, category: editCategory })
      editingKey = ''
      onRefresh()
    } catch (e) {
      renameStatus = '✗ ' + e.message
    } finally {
      setTimeout(() => renameStatus = '', 4000)
    }
  }

  function preview(category, name) {
    const key = `${category}/${name}`
    if (playingKey === key && currentAudio) {
      currentAudio.pause()
      currentAudio = null
      playingKey = ''
      return
    }
    if (currentAudio) currentAudio.pause()
    currentAudio = new Audio(`/api/library/${category}/${name}/preview`)
    currentAudio.onended = () => { playingKey = ''; currentAudio = null }
    currentAudio.onerror = () => { playingKey = ''; currentAudio = null }
    currentAudio.play()
    playingKey = key
  }

  async function autoNormalize(p) {
    const key = `${p.category}/${p.name}`
    gainBusyKey = key
    try {
      const analysis = await apiClient.analyzePreset(p.category, p.name)
      gainAnalysis = analysis
      gainEditValue = analysis.suggested_gain
      gainEditKey = key
      await apiClient.setPresetGain(p.category, p.name, analysis.suggested_gain)
      p.gain = analysis.suggested_gain
      toast.success(`Normalized ${p.name}: gain ${analysis.suggested_gain.toFixed(2)}x (RMS ${(analysis.rms * 100).toFixed(0)}%)`)
    } catch (e) {
      toast.error(`Normalize failed: ${e.message}`)
    } finally {
      gainBusyKey = ''
    }
  }

  async function saveGain(p) {
    try {
      await apiClient.setPresetGain(p.category, p.name, gainEditValue)
      p.gain = gainEditValue
      toast.success(`Gain set to ${gainEditValue.toFixed(2)}x for ${p.name}`)
    } catch (e) {
      toast.error(`Set gain failed: ${e.message}`)
    }
  }

  function toggleGainEdit(p) {
    const key = `${p.category}/${p.name}`
    if (gainEditKey === key) {
      gainEditKey = ''
      gainAnalysis = null
    } else {
      gainEditValue = p.gain || 1.0
      gainEditKey = key
      gainAnalysis = null
    }
  }

  const libTabs = [
    { id: 'upload', label: 'Upload file' },
    { id: 'stream', label: 'Stream' },
    { id: 'generate', label: 'Generate TTS' },
  ]
</script>

<div class="flex flex-col gap-4">
  {#if libError}<p class="text-sm text-destructive">{libError}</p>{/if}
    <div class="flex flex-wrap items-center gap-2">
      <Button onclick={openAdd}><Plus class="h-4 w-4" />{busy ? 'Adding preset…' : 'Add preset'}</Button>
      <span class="text-sm text-muted-foreground">Sort by</span>
      <Select bind:value={sortBy} class="w-32">
        <option value="name">Name</option>
        <option value="duration">Duration</option>
      </Select>
      <Button variant="outline" size="sm" onclick={() => sortOrder = sortOrder === 'asc' ? 'desc' : 'asc'} title={sortOrder === 'asc' ? 'Ascending' : 'Descending'}>
        {#if sortOrder === 'asc'}<ArrowUp class="h-4 w-4" />{:else}<ArrowDown class="h-4 w-4" />{/if}
      </Button>
    </div>
    {#if presets.length === 0}
      <p class="italic text-muted-foreground">No presets yet. Add a file, stream, or generated speech.</p>
    {:else}
      {#each Object.entries(grouped) as [cat, items]}
        <div class="mb-4">
          <h3 class="mb-2 text-sm font-semibold text-muted-foreground">{cat}</h3>
          <div class="flex flex-col gap-1.5">
            {#each items as p}
              {@const key = `${p.category}/${p.name}`}
              {@const isStream = !!p.url}
              <div class="rounded-lg border bg-card px-3 py-2">
                <div class="flex items-center justify-between">
                  <div class="flex min-w-0 flex-1 items-center gap-2.5">
                    {#if isStream}
                      <Radio class="h-4 w-4 flex-shrink-0 text-violet-500" />
                    {/if}
                    <button
                      class="font-semibold whitespace-nowrap hover:text-primary hover:underline"
                      onclick={() => startRename(p)}
                      title="Click to rename"
                    >{p.name}</button>
                    {#if isStream}
                      <span class="truncate text-xs text-muted-foreground font-mono">{p.url}</span>
                    {:else}
                      {#if p.text}<span class="truncate text-sm italic text-muted-foreground">"{p.text}"</span>{/if}
                    {/if}
                  </div>
                  <div class="flex shrink-0 gap-1">
                    {#if !isStream}
                      <span class="text-xs text-muted-foreground whitespace-nowrap self-center mr-1">{formatSeconds(p.duration)}</span>
                      {#if p.gain && p.gain !== 1.0}
                        <span class="text-xs text-amber-500 whitespace-nowrap self-center mr-1 font-mono">{p.gain.toFixed(2)}x</span>
                      {/if}
                    {:else}
                      <Button variant="outline" size="icon" class="h-8 w-8" onclick={() => preview(p.category, p.name)} title="Preview" aria-label="Preview preset">
                        {#if playingKey === key}<Pause class="h-4 w-4" />{:else}<Play class="h-4 w-4" />{/if}
                      </Button>
                    {/if}
                    {#if !isStream}
                      <Button variant="outline" size="icon" class="h-8 w-8" onclick={() => autoNormalize(p)} title="Auto-normalize gain" aria-label="Auto-normalize gain" disabled={gainBusyKey === key}>
                        {#if gainBusyKey === key}<Loader2 class="h-4 w-4 animate-spin" />{:else}<Wand2 class="h-4 w-4" />{/if}
                      </Button>
                      <Button variant={gainEditKey === key ? 'default' : 'outline'} size="icon" class="h-8 w-8" onclick={() => toggleGainEdit(p)} title="Adjust gain" aria-label="Adjust gain">
                        <Gauge class="h-4 w-4" />
                      </Button>
                    {/if}
                    <Button variant="outline" size="icon" class="h-8 w-8" onclick={() => startRename(p)} title="Rename" aria-label="Rename preset">
                      <Pencil class="h-4 w-4" />
                    </Button>
                    <Button variant="outline" size="icon" class="h-8 w-8 hover:border-destructive hover:text-destructive" onclick={() => deletePreset(p.category, p.name)} title="Delete" aria-label="Delete preset">
                      <X class="h-4 w-4" />
                    </Button>
                  </div>
                </div>
                {#if !isStream}
                  <div class="mt-1.5">
                    <AudioPlayer
                      peaksUrl={`/api/library/${encodeURIComponent(p.category)}/${encodeURIComponent(p.name)}/peaks`}
                      audioUrl={`/api/library/${encodeURIComponent(p.category)}/${encodeURIComponent(p.name)}/preview`}
                      duration={p.duration}
                      title={p.name}
                      subtitle={p.category}
                      metadata={[formatSeconds(p.duration)]}
                      vuOrientation="vertical"
                    />
                  </div>
                {/if}
                {#if gainEditKey === key}
                  <div class="mt-2 flex items-center gap-3 border-t pt-2">
                    <span class="text-xs text-muted-foreground whitespace-nowrap">Gain</span>
                    <input
                      type="range"
                      min="0.1"
                      max="5"
                      step="0.05"
                      bind:value={gainEditValue}
                      class="flex-1 accent-amber-500"
                    />
                    <span class="text-xs font-mono w-12 text-right">{gainEditValue.toFixed(2)}x</span>
                    <Button size="sm" class="h-7" onclick={() => saveGain(p)}>Set</Button>
                    <Button size="sm" variant="ghost" class="h-7" onclick={() => { gainEditKey = ''; gainAnalysis = null }}>Close</Button>
                  </div>
                {/if}
                {#if editingKey === key}
                  <div class="mt-2 flex flex-wrap items-end gap-2 border-t pt-2">
                    <label class="flex flex-col gap-0.5 text-xs text-muted-foreground">
                      Name
                      <Input bind:value={editName} class="h-8 w-40" />
                    </label>
                    <label class="flex flex-col gap-0.5 text-xs text-muted-foreground">
                      Category
                      <Input bind:value={editCategory} class="h-8 w-32" />
                    </label>
                    <Button size="sm" class="h-8" onclick={() => doRename(p.category, p.name)} disabled={editName === p.name && editCategory === p.category}>
                      Save
                    </Button>
                    <Button size="sm" variant="ghost" class="h-8" onclick={() => editingKey = ''}>Cancel</Button>
                    {#if renameStatus}<span class="text-xs text-destructive">{renameStatus}</span>{/if}
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        </div>
      {/each}
    {/if}

  <Modal bind:open={addOpen} title="Add preset">
    <div class="mb-4 flex flex-wrap gap-1" aria-label="Preset source">
      {#each libTabs as t}
        <Button variant={tab === t.id ? 'default' : 'ghost'} size="sm" aria-pressed={tab === t.id}
          disabled={busy || testBusy || !!testingCamera}
          onclick={() => { genAudioEl?.pause(); uploadAudioEl?.pause(); genPlaying = false; tab = t.id }}>{t.label}</Button>
      {/each}
    </div>
    {#if busy}<p class="mb-3 text-xs text-muted-foreground" role="status">Your preset is being prepared. You can close this dialog and reopen it to check progress; stay on the Library screen.</p>{/if}
    <fieldset disabled={busy} class="min-w-0 border-0 p-0">
  {#if tab === 'generate'}
    <div class="flex max-w-2xl flex-col gap-3">
      <h3 class="text-base font-semibold text-primary">Generate TTS Preset</h3>
      <label class="flex flex-col gap-1 text-sm text-muted-foreground">
        Text
        <Textarea bind:value={genText} rows="5" placeholder="Text to synthesize..." />
      </label>
      <label class="flex flex-col gap-1 text-sm text-muted-foreground">
        Voice
        <VoiceSelect bind:value={genVoice} {voices} />
      </label>
      <div class="flex gap-2">
        <Button onclick={generate} disabled={genBusy || !genText}>
          {#if genBusy}<Loader2 class="h-4 w-4 animate-spin" />{:else}<Sparkles class="h-4 w-4" />{/if}
          Generate audio
        </Button>
        {#if genAudio}
          <Button variant="outline" onclick={togglePreview} aria-label={genPlaying ? 'Pause generated audio' : 'Preview generated audio'}>
            {#if genPlaying}<Pause class="h-4 w-4" />{:else}<Play class="h-4 w-4" />{/if}
          </Button>
        {/if}
      </div>
      {#if genAudio}
        <div class="rounded-lg border border-dashed border-primary/40 bg-primary/5 p-3">
          <p class="mb-2 text-xs text-muted-foreground">Generated — enter name to save</p>
          <label class="flex flex-col gap-1 text-sm text-muted-foreground">
            Name
            <Input bind:value={genName} placeholder="e.g. person_detected" />
          </label>
          <label class="flex flex-col gap-1 text-sm text-muted-foreground">
            Category
            <Input bind:value={genCategory} placeholder="alerts" />
          </label>
          <Button variant="secondary" onclick={save} disabled={genBusy || !genName} class="mt-2 w-fit">
            <Save class="h-4 w-4" />
            Save
          </Button>
        </div>
      {/if}
      {#if genStatus}<p class="text-sm text-primary">{genStatus}</p>{/if}
    </div>

  {:else if tab === 'upload'}
    <div class="flex max-w-2xl flex-col gap-3">
      <h3 class="text-base font-semibold text-primary">Upload Audio File</h3>
      <p class="text-sm text-muted-foreground">Drag and drop an audio file, or click to browse. Any format — ffmpeg will convert to G.711ulaw 8kHz.</p>
      <label class="flex flex-col gap-1 text-sm text-muted-foreground">
        Name
        <Input bind:value={uploadName} placeholder="preset name" />
      </label>
      <label class="flex flex-col gap-1 text-sm text-muted-foreground">
        Category
        <Input bind:value={uploadCategory} placeholder="uploads" />
      </label>
      <label
        class="relative flex flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed p-6 transition-colors cursor-pointer {uploadDragging ? 'border-primary bg-primary/5' : 'border-input bg-background'}"
        ondragover={(e) => { e.preventDefault(); uploadDragging = true }}
        ondragleave={(e) => { e.preventDefault(); uploadDragging = false }}
        ondrop={(e) => {
          e.preventDefault()
          uploadDragging = false
          const file = e.dataTransfer.files?.[0]
          if (file) handleUploadFile(file)
        }}
      >
        <input
          type="file"
          accept="audio/*"
          aria-label="Audio file"
          class="absolute inset-0 h-full w-full cursor-pointer opacity-0"
          onchange={(e) => { handleUploadFile(e.currentTarget.files?.[0] ?? null) }}
        />
        {#if uploadFile}
          <span class="text-sm font-medium text-primary">{uploadFile.name}</span>
          <span class="text-xs text-muted-foreground">{(uploadFile.size / 1024 / 1024).toFixed(2)} MB</span>
        {:else}
          <Upload class="h-8 w-8 text-muted-foreground" />
          <span class="text-sm text-muted-foreground">Drop audio file here or click to browse</span>
        {/if}
      </label>
      {#if uploadPreview}
        <div class="rounded-lg border bg-background p-3">
          <p class="mb-2 text-xs text-muted-foreground">Preview before saving</p>
          <audio bind:this={uploadAudioEl} src={uploadPreview} controls class="w-full"></audio>
        </div>
      {/if}
      <Button onclick={upload} disabled={uploadBusy || !uploadName || !uploadFile} class="w-fit">
        {#if uploadBusy}<Loader2 class="h-4 w-4 animate-spin" />{:else}<Save class="h-4 w-4" />{/if}
        Save
      </Button>
      {#if uploadStatus}<p class="text-sm text-primary">{uploadStatus}</p>{/if}
    </div>

  {:else if tab === 'stream'}
    <div class="flex max-w-2xl flex-col gap-3">
      <h3 class="text-base font-semibold text-primary">Add Stream Preset</h3>
      <p class="text-sm text-muted-foreground">Save a live stream URL (icecast, shoutcast, .pls, .m3u) as a named preset. When played, the stream is sent live to the camera speaker via ffmpeg.</p>
      <label class="flex flex-col gap-1 text-sm text-muted-foreground">
        Stream URL
        <Input bind:value={streamURL} placeholder="http://stream.example.com:8000/live" />
      </label>
      <label class="flex flex-col gap-1 text-sm text-muted-foreground">
        Name
        <Input bind:value={streamName} placeholder="e.g. liveatc_bos" />
      </label>
      <label class="flex flex-col gap-1 text-sm text-muted-foreground">
        Category
        <Input bind:value={streamCategory} placeholder="streams" />
      </label>
      <div class="flex flex-col gap-2 rounded-lg border p-3">
        <label class="flex flex-col gap-1 text-sm text-muted-foreground">
          Test on camera
          <Select bind:value={testCamera} disabled={testBusy || !!testingCamera}>
            <option value="">Choose a camera…</option>
            {#each cameras as camera}<option value={camera.name}>{camera.name}</option>{/each}
          </Select>
        </label>
        <p class="text-xs text-muted-foreground">Testing plays aloud on the selected camera and interrupts its current audio. Saving alone does not play audio.</p>
        {#if testingCamera}
          <Button variant="destructive" onclick={stopTest} disabled={testBusy} class="w-fit"><Square class="h-4 w-4" />Stop test</Button>
        {:else}
          <Button variant="outline" onclick={testStream} disabled={testBusy || !testCamera || !streamURL.trim()} class="w-fit"><Play class="h-4 w-4" />Test stream</Button>
        {/if}
        {#if testStatus}<p class="text-sm break-words" role="status">{testStatus}</p>{/if}
      </div>
      <Button onclick={saveStream} disabled={streamBusy || testBusy || !!testingCamera || !streamName || !streamURL} class="w-fit">
        {#if streamBusy}<Loader2 class="h-4 w-4 animate-spin" />{:else}<Radio class="h-4 w-4" />{/if}
        Save Stream Preset
      </Button>
      {#if streamStatus}<p class="text-sm text-primary">{streamStatus}</p>{/if}
    </div>
  {/if}
    </fieldset>

  <!-- Upload progress dialog -->
  {#if uploadProgress}
      <div class="mt-4 rounded-lg border bg-card p-4" role="status" aria-live="polite">
        <div class="flex items-center gap-3 mb-4">
          {#if uploadProgress.step === 'done'}
            <span class="text-primary text-lg font-semibold">✓</span>
          {:else}
            <Loader2 class="h-5 w-5 animate-spin text-primary" />
          {/if}
          <span class="font-semibold">{uploadProgress.label}</span>
          {#if uploadProgress.step !== 'done' && uploadProgress.percent > 0}
            <span class="text-sm text-muted-foreground ml-auto">{uploadProgress.percent.toFixed(0)}%</span>
          {/if}
        </div>

        <!-- Progress bar -->
        <div class="w-full h-3 rounded-full bg-muted overflow-hidden">
          {#if uploadProgress.percent > 0}
            <div
              class="h-full bg-primary transition-all duration-300 ease-out"
              style="width: {uploadProgress.percent}%"
            ></div>
          {:else}
            <div class="h-full bg-primary/50 animate-pulse" style="width: 100%"></div>
          {/if}
        </div>

        <!-- Step detail -->
        <div class="mt-3 text-xs text-muted-foreground">
          {#if uploadProgress.step === 'uploading'}
            Uploading {uploadFile?.name ?? ''} to server…
          {:else if uploadProgress.step === 'transcoding'}
            Converting to G.711 µ-law 8 kHz{#if uploadProgress.percent > 0} — {uploadProgress.percent.toFixed(0)}%{/if}
          {:else if uploadProgress.step === 'done'}
            Saved as "{uploadName}" in {uploadCategory}
          {/if}
        </div>
      </div>
  {/if}
  </Modal>
</div>
