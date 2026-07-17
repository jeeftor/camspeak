<script>
  import { onDestroy } from 'svelte'
  import { Eye, Bell, Play, Volume2, Loader2, MessageSquare, FileAudio, X } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { Card } from '$lib/components/ui/card'
  import { Badge } from '$lib/components/ui/badge'

  let { camera, voices = [], presets = [] } = $props()

  let text = $state('')
  let voice = $state('')
  let preset = $state('')
  let url = $state('')
  let gain = $state(3.0)
  let busy = $state(false)
  let status = $state('')
  let statusType = $state('ok')
  let snapshot = $state('')
  let description = $state('')
  // Pre-fill from saved camera default; user can override per-session
  const savedPrompt = camera.vision_prompt ?? ''
  let visionPrompt = $state(savedPrompt)
  let showPrompt = $state(false)
  let isDragOver = $state(false)
  let statusTimeout

  onDestroy(() => {
    if (snapshot) URL.revokeObjectURL(snapshot)
    clearTimeout(statusTimeout)
  })

  async function post(path, body) {
    const res = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) throw new Error(await res.text())
  }

  function setStatus(msg, type = 'ok') {
    status = msg
    statusType = type
    if (!busy) {
      clearTimeout(statusTimeout)
      statusTimeout = setTimeout(() => (status = ''), 4000)
    }
  }

  async function speak() {
    if (!text) return
    busy = true; status = ''
    try {
      await post('/api/speak', { camera: camera.name, text, voice, gain })
      setStatus('✓ sent')
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    } finally {
      busy = false
    }
  }

  async function play() {
    if (!preset) {
      setStatus('select a preset first', 'warn')
      return
    }
    busy = true; status = ''
    try {
      await post('/api/play', { camera: camera.name, preset, gain })
      setStatus('✓ playing')
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    } finally {
      busy = false
    }
  }

  async function playUrl() {
    if (!url) return
    busy = true; status = ''
    try {
      await post('/api/play-url', { camera: camera.name, url, gain })
      setStatus('✓ playing')
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    } finally {
      busy = false
    }
  }

  async function beep() {
    busy = true; status = ''
    try {
      await post('/api/beep', { camera: camera.name })
      setStatus('✓ beep')
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    } finally {
      busy = false
    }
  }

  async function describe() {
    busy = true; status = ''
    if (snapshot) URL.revokeObjectURL(snapshot)
    snapshot = ''; description = ''
    try {
      setStatus('Capturing screenshot…')
      const snapRes = await fetch(`/api/snapshot/${camera.name}`)
      if (!snapRes.ok) throw new Error('snapshot fetch failed')
      const snapBlob = await snapRes.blob()
      snapshot = URL.createObjectURL(snapBlob)

      setStatus('Describing → speaking…')
      const body = { camera: camera.name, gain }
      if (visionPrompt) body.prompt = visionPrompt
      const res = await fetch('/api/describe', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      if (!res.ok) throw new Error(await res.text())
      const data = await res.json()
      description = data.description || ''
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    } finally {
      busy = false
      if (description) setStatus('✓ described & spoken')
    }
  }

  async function replayDescription() {
    if (!description) return
    busy = true; status = ''
    try {
      await post('/api/speak', { camera: camera.name, text: description, voice, gain })
      setStatus('✓ replaying')
    } catch (e) {
      setStatus('✗ ' + e.message, 'err')
    } finally {
      busy = false
    }
  }

  function clearSnapshot() {
    if (snapshot) URL.revokeObjectURL(snapshot)
    snapshot = ''
    description = ''
    status = ''
  }

  // WAV / audio file drag-and-drop
  function onDragOver(e) {
    const hasAudio = [...(e.dataTransfer?.items ?? [])].some(
      it => it.kind === 'file' && (it.type.startsWith('audio/') || it.type === 'application/octet-stream')
    )
    if (hasAudio) {
      e.preventDefault()
      isDragOver = true
    }
  }

  function onDragLeave() {
    isDragOver = false
  }

  async function onDrop(e) {
    e.preventDefault()
    isDragOver = false
    const file = e.dataTransfer?.files?.[0]
    if (!file) return
    if (!file.name.match(/\.(wav|mp3|m4a|aac|flac|ogg|opus)$/i) && !file.type.startsWith('audio/')) {
      setStatus('Drop an audio file', 'warn')
      return
    }

    busy = true
    setStatus('Uploading…')
    try {
      const dropName = `drop_${Date.now()}`
      const fd = new FormData()
      fd.append('name', dropName)
      fd.append('category', 'drops')
      fd.append('file', file)

      const upRes = await fetch('/api/library/upload', { method: 'POST', body: fd })
      if (!upRes.ok) throw new Error(await upRes.text())

      setStatus('Playing…')
      await post('/api/play', { camera: camera.name, preset: dropName, category: 'drops', gain })
      setStatus(`✓ playing ${file.name}`)
    } catch (err) {
      setStatus('✗ ' + err.message, 'err')
    } finally {
      busy = false
    }
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  ondragover={onDragOver}
  ondragleave={onDragLeave}
  ondrop={onDrop}
>
  <Card class="flex flex-col gap-2.5 p-4 transition-colors
    {!camera.online ? 'opacity-50' : ''}
    {isDragOver ? 'border-primary border-dashed bg-primary/5 scale-[1.01]' : 'hover:border-primary/50'}">

    <!-- Camera header -->
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-2 min-w-0">
        <span class="h-2.5 w-2.5 rounded-full flex-shrink-0 {camera.online
          ? 'bg-green-500 shadow-[0_0_6px_rgba(34,197,94,0.5)]'
          : 'bg-muted-foreground/40'}"></span>
        <span class="font-semibold truncate">{camera.name}</span>
        <Badge variant="secondary" class="text-xs flex-shrink-0">{camera.type}</Badge>
      </div>
      <div class="flex gap-1 flex-shrink-0">
        <Button
          variant="outline" size="icon"
          onclick={describe} disabled={busy}
          title="Describe (vision)" aria-label="Describe"
          class="h-8 w-8"
        >
          {#if busy && status.toLowerCase().includes('describ')}
            <Loader2 class="h-4 w-4 animate-spin" />
          {:else}
            <Eye class="h-4 w-4" />
          {/if}
        </Button>
        <Button
          variant={showPrompt ? 'default' : 'outline'} size="icon"
          onclick={() => showPrompt = !showPrompt}
          title="Custom vision prompt" aria-label="Vision prompt"
          class="h-8 w-8"
        >
          <MessageSquare class="h-4 w-4" />
        </Button>
        <Button
          variant="outline" size="icon"
          onclick={beep} disabled={busy}
          title="Test beep" aria-label="Test beep"
          class="h-8 w-8"
        >
          <Bell class="h-4 w-4" />
        </Button>
      </div>
    </div>

    <!-- Vision prompt (collapsible) -->
    {#if showPrompt}
      <Input
        bind:value={visionPrompt}
        placeholder="Custom vision prompt — e.g. How many deer do you see?"
        class="text-xs"
        disabled={busy}
      />
    {/if}

    <!-- TTS row -->
    <div class="flex gap-1.5">
      <Input
        bind:value={text}
        placeholder="Say something..."
        onkeydown={e => e.key === 'Enter' && speak()}
        disabled={busy}
        class="flex-1 text-sm min-w-0"
      />
      <select bind:value={voice} disabled={busy}
        class="w-[100px] flex-shrink-0 rounded-md border border-input bg-transparent px-2 py-1 text-sm disabled:opacity-50">
        <option value="">default</option>
        {#each voices as v}
          <option>{v}</option>
        {/each}
      </select>
      <Button size="sm" onclick={speak} disabled={busy || !text} aria-label="Speak" class="flex-shrink-0">
        <Play class="h-4 w-4" />
      </Button>
    </div>

    <!-- Volume row -->
    <div class="flex items-center gap-2 px-1">
      <Volume2 class="h-3.5 w-3.5 text-muted-foreground flex-shrink-0" />
      <input type="range" min="1" max="10" step="0.5" bind:value={gain} disabled={busy}
        class="flex-1 accent-primary" />
      <span class="text-xs text-muted-foreground font-mono w-8 text-right flex-shrink-0">{gain}×</span>
    </div>

    <!-- Preset row -->
    {#if presets.length > 0}
      <div class="flex gap-1.5">
        <select bind:value={preset} disabled={busy}
          class="flex-1 min-w-0 rounded-md border border-input bg-transparent px-3 py-1 text-sm disabled:opacity-50">
          <option value="">— play preset —</option>
          {#each presets as p}
            <option value={p.name}>{p.category}/{p.name}</option>
          {/each}
        </select>
        <Button size="sm" onclick={play} disabled={busy || !preset} aria-label="Play preset" class="flex-shrink-0">
          <Play class="h-4 w-4" />
        </Button>
      </div>
    {/if}

    <!-- URL row -->
    <div class="flex gap-1.5">
      <Input
        bind:value={url}
        placeholder="Play from URL..."
        onkeydown={e => e.key === 'Enter' && playUrl()}
        disabled={busy}
        class="flex-1 text-sm min-w-0"
      />
      <Button size="sm" onclick={playUrl} disabled={busy || !url} aria-label="Play URL" class="flex-shrink-0">
        <Play class="h-4 w-4" />
      </Button>
    </div>

    <!-- Drag overlay hint -->
    {#if isDragOver}
      <div class="flex items-center justify-center gap-2 rounded-lg border border-dashed border-primary py-3 text-sm text-primary">
        <FileAudio class="h-4 w-4" />
        Drop to play on {camera.name}
      </div>
    {/if}

    <!-- Status -->
    {#if status}
      <div class="text-sm {statusType === 'err' ? 'text-destructive' : statusType === 'warn' ? 'text-yellow-500' : 'text-primary'}">
        {status}
      </div>
    {/if}

    <!-- Snapshot + description -->
    {#if snapshot}
      <div class="rounded-lg border border-primary/30 overflow-hidden relative">
        <img src={snapshot} alt="Camera snapshot" class="w-full" />
        <Button
          variant="outline" size="icon"
          onclick={clearSnapshot} disabled={busy}
          title="Clear" aria-label="Clear snapshot"
          class="absolute top-2 right-2 h-7 w-7 bg-background/80 backdrop-blur"
        >
          <X class="h-4 w-4" />
        </Button>
        {#if description}
          <div class="flex items-start gap-2 p-2 bg-muted/30">
            <p class="text-xs text-muted-foreground flex-1">{description}</p>
            <Button
              variant="outline" size="icon"
              onclick={replayDescription} disabled={busy}
              title="Re-play description" aria-label="Re-play"
              class="h-6 w-6 flex-shrink-0"
            >
              <Play class="h-3 w-3" />
            </Button>
          </div>
        {/if}
      </div>
    {/if}
  </Card>
</div>
