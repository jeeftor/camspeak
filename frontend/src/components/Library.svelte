<script>
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { Select } from '$lib/components/ui/select'
  import { Textarea } from '$lib/components/ui/textarea'

  let { presets = [], voices = [], onRefresh } = $props()

  let tab = $state('browse')
  let genName = $state('')
  let genText = $state('')
  let genCategory = $state('alerts')
  let genVoice = $state('')
  let genBusy = $state(false)
  let genStatus = $state('')
  let genAudio = $state(null)      // holds the generated WAV blob URL
  let genPlaying = $state(false)

  let uploadName = $state('')
  let uploadCategory = $state('uploads')
  let uploadFile = $state(null)
  let uploadBusy = $state(false)
  let uploadStatus = $state('')

  // Group presets by category
  let grouped = $derived(
    presets.reduce((acc, p) => {
      ;(acc[p.category] ??= []).push(p)
      return acc
    }, {})
  )

  async function generate() {
    if (!genText) return
    genBusy = true; genStatus = ''
    // Clear previous audio
    if (genAudio) { URL.revokeObjectURL(genAudio); genAudio = null }
    genPlaying = false
    try {
      const res = await fetch('/api/tts/preview', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text: genText, voice: genVoice }),
      })
      if (!res.ok) throw new Error(await res.text())
      const blob = await res.blob()
      genAudio = URL.createObjectURL(blob)
      genStatus = '✓ Playing…'
      // Autoplay immediately
      genAudioEl = new Audio(genAudio)
      genAudioEl.onended = () => { genPlaying = false }
      genAudioEl.play()
      genPlaying = true
    } catch (e) {
      genStatus = '✗ ' + e.message
    } finally {
      genBusy = false
      setTimeout(() => (genStatus = ''), 4000)
    }
  }

  function togglePreview() {
    if (!genAudio || !genAudioEl) return
    if (genPlaying) {
      genAudioEl.pause()
      genPlaying = false
    } else {
      genAudioEl.play()
      genPlaying = true
    }
  }

  async function save() {
    if (!genName || !genText || !genAudio) return
    genBusy = true; genStatus = ''
    try {
      const res = await fetch('/api/library', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: genName, text: genText, category: genCategory, voice: genVoice }),
      })
      if (!res.ok) throw new Error(await res.text())
      genStatus = '✓ Saved'
      genName = ''; genText = ''
      if (genAudio) { URL.revokeObjectURL(genAudio); genAudio = null }
      genPlaying = false
      onRefresh()
    } catch (e) {
      genStatus = '✗ ' + e.message
    } finally {
      genBusy = false
      setTimeout(() => (genStatus = ''), 4000)
    }
  }

  async function upload() {
    if (!uploadName || !uploadFile) return
    uploadBusy = true; uploadStatus = ''
    try {
      const fd = new FormData()
      fd.append('name', uploadName)
      fd.append('category', uploadCategory)
      fd.append('file', uploadFile)
      const res = await fetch('/api/library/upload', { method: 'POST', body: fd })
      if (!res.ok) throw new Error(await res.text())
      uploadStatus = '✓ Uploaded'
      uploadName = ''; uploadFile = null
      onRefresh()
    } catch (e) {
      uploadStatus = '✗ ' + e.message
    } finally {
      uploadBusy = false
      setTimeout(() => (uploadStatus = ''), 4000)
    }
  }

  async function deletePreset(category, name) {
    if (!confirm(`Delete ${category}/${name}?`)) return
    await fetch(`/api/library/${category}/${name}`, { method: 'DELETE' })
    onRefresh()
  }

  let currentAudio = $state(null)
  let playingKey = $state('')

  function preview(category, name) {
    const key = `${category}/${name}`
    // If already playing this, stop it
    if (playingKey === key && currentAudio) {
      currentAudio.pause()
      currentAudio = null
      playingKey = ''
      return
    }
    // Stop any existing audio
    if (currentAudio) {
      currentAudio.pause()
    }
    // Play in-browser via HTML5 audio
    currentAudio = new Audio(`/api/library/${category}/${name}/preview`)
    currentAudio.onended = () => {
      playingKey = ''
      currentAudio = null
    }
    currentAudio.onerror = () => {
      playingKey = ''
      currentAudio = null
    }
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
  <div class="flex gap-1">
    {#each libTabs as t}
      <Button
        variant={tab === t.id ? 'default' : 'outline'}
        size="sm"
        onclick={() => tab = t.id}
      >
        {t.label}
      </Button>
    {/each}
  </div>

  {#if tab === 'browse'}
    {#if presets.length === 0}
      <p class="italic text-muted-foreground">No presets yet. Generate or upload one.</p>
    {:else}
      {#each Object.entries(grouped) as [cat, items]}
        <div class="mb-4">
          <h3 class="mb-2 text-xs uppercase tracking-widest text-muted-foreground">{cat}</h3>
          <div class="flex flex-col gap-1.5">
            {#each items as p}
              <div class="flex items-center justify-between rounded-lg border bg-card px-3 py-2">
                <div class="flex min-w-0 flex-1 items-center gap-2.5">
                  <span class="font-semibold whitespace-nowrap">{p.name}</span>
                  <span class="text-xs text-muted-foreground whitespace-nowrap">{p.duration?.toFixed(1)}s</span>
                  {#if p.text}<span class="truncate text-sm italic text-muted-foreground">"{p.text}"</span>{/if}
                </div>
                <div class="flex shrink-0 gap-1">
                  <Button variant="outline" size="sm" class="h-7 px-2" onclick={() => preview(p.category, p.name)} title="Preview">
                    {playingKey === `${p.category}/${p.name}` ? '⏸' : '▶'}
                  </Button>
                  <Button variant="outline" size="sm" class="h-7 px-2 hover:border-destructive hover:text-destructive" onclick={() => deletePreset(p.category, p.name)} title="Delete">✕</Button>
                </div>
              </div>
            {/each}
          </div>
        </div>
      {/each}
    {/if}

  {:else if tab === 'generate'}
    <div class="flex max-w-md flex-col gap-3">
      <h3 class="text-base font-semibold text-primary">Generate TTS Preset</h3>
      <label class="flex flex-col gap-1 text-sm text-muted-foreground">
        Text
        <Textarea bind:value={genText} rows="3" placeholder="Text to synthesize..." />
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
          {genBusy ? 'Generating…' : '✨ Generate & Preview'}
        </Button>
        {#if genAudio}
          <Button variant="outline" onclick={togglePreview}>
            {genPlaying ? '⏸' : '▶'}
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
            {genBusy ? 'Saving…' : '💾 Save'}
          </Button>
        </div>
      {/if}
      {#if genStatus}<p class="text-sm text-primary">{genStatus}</p>{/if}
    </div>

  {:else}
    <div class="flex max-w-md flex-col gap-3">
      <h3 class="text-base font-semibold text-primary">Upload Audio File</h3>
      <p class="text-sm text-muted-foreground">Any format — ffmpeg will convert to G.711ulaw 8kHz</p>
      <label class="flex flex-col gap-1 text-sm text-muted-foreground">
        Name
        <Input bind:value={uploadName} placeholder="preset name" />
      </label>
      <label class="flex flex-col gap-1 text-sm text-muted-foreground">
        Category
        <Input bind:value={uploadCategory} placeholder="uploads" />
      </label>
      <label class="flex flex-col gap-1 text-sm text-muted-foreground">
        File
        <input
          type="file"
          accept="audio/*"
          class="flex w-full rounded-md border border-dashed border-input bg-transparent p-2 text-sm file:mr-3 file:rounded file:border-0 file:bg-primary file:px-3 file:py-1 file:text-primary-foreground hover:file:bg-primary/90"
          onchange={(e) => { uploadFile = e.currentTarget.files?.[0] ?? null }}
        />
      </label>
      <Button onclick={upload} disabled={uploadBusy || !uploadName || !uploadFile} class="w-fit">
        {uploadBusy ? 'Uploading…' : '⬆ Upload'}
      </Button>
      {#if uploadStatus}<p class="text-sm text-primary">{uploadStatus}</p>{/if}
    </div>
  {/if}
</div>
