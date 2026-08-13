<script>
  import { onDestroy } from 'svelte'
  import { Sparkles, Save, Upload, Play, Pause, X, Loader2, Pencil, ArrowUpDown, ArrowUp, ArrowDown } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { Select } from '$lib/components/ui/select'
  import { Textarea } from '$lib/components/ui/textarea'
  import { toast } from '$lib/components/ui/toast'
  import { apiClient } from '$lib/api'
  import { formatMs, formatTimingSummary } from '$lib/utils'

  let { presets = [], voices = [], onRefresh } = $props()

  let tab = $state('browse')
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
    clearTimeout(uploadPollTimer)
  })

  let uploadName = $state('')
  let uploadCategory = $state('uploads')
  let uploadFile = $state(null)
  let uploadPreview = $state(null)
  let uploadDragging = $state(false)
  let uploadBusy = $state(false)
  let uploadStatus = $state('')
  let libError = $state('')
  let statusTimeout
  let uploadTimeout

  // Upload progress dialog state
  let uploadProgress = $state(null) // null = no dialog, {step, percent, label} = active
  let uploadPollTimer = null

  let sortBy = $state('name')
  let sortOrder = $state('asc')

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
    if (genAudio) { URL.revokeObjectURL(genAudio); genAudio = null }
    genPlaying = false
    try {
      const res = await apiClient.ttsPreview({ text: genText, voice: genVoice })
      const ttsMs = res.headers.get('X-TTS-Ms')
      const blob = await res.blob()
      genAudio = URL.createObjectURL(blob)
      const ms = ttsMs ? formatMs(Number(ttsMs)) : ''
      genStatus = ms ? `✓ Playing… (${ms})` : '✓ Playing…'
      genAudioEl = new Audio(genAudio)
      genAudioEl.onended = () => { genPlaying = false }
      genAudioEl.play()
      genPlaying = true
    } catch (e) {
      genStatus = '✗ ' + e.message
    } finally {
      genBusy = false
      clearTimeout(genTimeout); genTimeout = setTimeout(() => (genStatus = ''), 4000)
    }
  }

  function togglePreview() {
    if (!genAudio || !genAudioEl) return
    if (genPlaying) { genAudioEl.pause(); genPlaying = false }
    else { genAudioEl.play(); genPlaying = true }
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
      const fd = new FormData()
      fd.append('name', uploadName)
      fd.append('category', uploadCategory)
      fd.append('file', uploadFile)

      // Phase 1: upload the file (XHR gives us byte-level progress).
      const res = await apiClient.uploadPresetWithProgress(fd, (pct) => {
        uploadProgress = { step: 'uploading', percent: pct, label: 'Uploading' }
      })

      // Phase 2: poll the transcoding job.
      const jobId = res.job_id
      uploadProgress = { step: 'transcoding', percent: 0, label: 'Converting' }

      await new Promise((resolve, reject) => {
        const poll = async () => {
          try {
            const job = await apiClient.getUploadJob(jobId)
            if (job.status === 'done') {
              uploadProgress = { step: 'done', percent: 100, label: 'Done' }
              resolve(job)
            } else if (job.status === 'error') {
              reject(new Error(job.error || 'transcoding failed'))
            } else {
              uploadProgress = {
                step: 'transcoding',
                percent: job.percent < 0 ? 0 : job.percent,
                label: job.step || 'Converting',
              }
              uploadPollTimer = setTimeout(poll, 500)
            }
          } catch (e) {
            reject(e)
          }
        }
        poll()
      })

      uploadStatus = '✓ Uploaded'
      toast.success(`Preset "${uploadName}" uploaded`)
      clearUpload()
      onRefresh()
    } catch (e) {
      uploadStatus = '✗ ' + e.message
      toast.error(`Upload failed: ${e.message}`)
    } finally {
      // Keep the dialog visible briefly so the user sees "Done", then close.
      setTimeout(() => { uploadProgress = null }, 800)
      uploadBusy = false
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

  const libTabs = [
    { id: 'browse', label: 'Browse' },
    { id: 'generate', label: 'Generate TTS' },
    { id: 'upload', label: 'Upload' },
  ]
</script>

<div class="flex flex-col gap-4">
  {#if libError}<p class="text-sm text-destructive">{libError}</p>{/if}
  <div class="flex gap-1 overflow-x-auto" style="scrollbar-width:none;">
    {#each libTabs as t}
      <Button
        variant={tab === t.id ? 'default' : 'ghost'}
        size="sm"
        onclick={() => tab = t.id}
        class="flex-shrink-0"
      >
        {t.label}
      </Button>
    {/each}
  </div>

  {#if tab === 'browse'}
    <div class="flex flex-wrap items-center gap-2">
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
      <p class="italic text-muted-foreground">No presets yet. Generate or upload one.</p>
    {:else}
      {#each Object.entries(grouped) as [cat, items]}
        <div class="mb-4">
          <h3 class="mb-2 text-sm font-semibold text-muted-foreground">{cat}</h3>
          <div class="flex flex-col gap-1.5">
            {#each items as p}
              {@const key = `${p.category}/${p.name}`}
              <div class="rounded-lg border bg-card px-3 py-2">
                <div class="flex items-center justify-between">
                  <div class="flex min-w-0 flex-1 items-center gap-2.5">
                    <button
                      class="font-semibold whitespace-nowrap hover:text-primary hover:underline"
                      onclick={() => startRename(p)}
                      title="Click to rename"
                    >{p.name}</button>
                    <span class="text-xs text-muted-foreground whitespace-nowrap">{p.duration?.toFixed(1)}s</span>
                    {#if p.text}<span class="truncate text-sm italic text-muted-foreground">"{p.text}"</span>{/if}
                  </div>
                  <div class="flex shrink-0 gap-1">
                    <Button variant="outline" size="icon" class="h-8 w-8" onclick={() => preview(p.category, p.name)} title="Preview" aria-label="Preview preset">
                      {#if playingKey === key}<Pause class="h-4 w-4" />{:else}<Play class="h-4 w-4" />{/if}
                    </Button>
                    <Button variant="outline" size="icon" class="h-8 w-8" onclick={() => startRename(p)} title="Rename" aria-label="Rename preset">
                      <Pencil class="h-4 w-4" />
                    </Button>
                    <Button variant="outline" size="icon" class="h-8 w-8 hover:border-destructive hover:text-destructive" onclick={() => deletePreset(p.category, p.name)} title="Delete" aria-label="Delete preset">
                      <X class="h-4 w-4" />
                    </Button>
                  </div>
                </div>
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

  {:else if tab === 'generate'}
    <div class="flex max-w-2xl flex-col gap-3">
      <h3 class="text-base font-semibold text-primary">Generate TTS Preset</h3>
      <label class="flex flex-col gap-1 text-sm text-muted-foreground">
        Text
        <Textarea bind:value={genText} rows="5" placeholder="Text to synthesize..." />
      </label>
      <label class="flex flex-col gap-1 text-sm text-muted-foreground">
        Voice
        <Select bind:value={genVoice}>
          <option value="">default</option>
          {#each voices as v}
            <option>{v}</option>
          {/each}
        </Select>
      </label>
      <div class="flex gap-2">
        <Button onclick={generate} disabled={genBusy || !genText}>
          {#if genBusy}<Loader2 class="h-4 w-4 animate-spin" />{:else}<Sparkles class="h-4 w-4" />{/if}
          Generate & Preview
        </Button>
        {#if genAudio}
          <Button variant="outline" onclick={togglePreview}>
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

  {:else}
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
          class="hidden"
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
          <audio src={uploadPreview} controls class="w-full"></audio>
        </div>
      {/if}
      <Button onclick={upload} disabled={uploadBusy || !uploadName || !uploadFile} class="w-fit">
        {#if uploadBusy}<Loader2 class="h-4 w-4 animate-spin" />{:else}<Save class="h-4 w-4" />{/if}
        Save
      </Button>
      {#if uploadStatus}<p class="text-sm text-primary">{uploadStatus}</p>{/if}
    </div>
  {/if}

  <!-- Upload progress dialog -->
  {#if uploadProgress}
    <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
      <div class="rounded-lg border bg-card p-6 shadow-lg w-96 max-w-[90vw]">
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
    </div>
  {/if}
</div>
