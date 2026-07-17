<script>
  import { onMount, onDestroy } from 'svelte'
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
  let snapshot = $state('')
  let description = $state('')

  onDestroy(() => {
    if (snapshot) URL.revokeObjectURL(snapshot)
  })

  async function post(path, body) {
    const res = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) throw new Error(await res.text())
  }

  async function speak() {
    if (!text) return
    busy = true; status = ''
    try {
      await post('/api/speak', { camera: camera.name, text, voice, gain })
      status = '✓'
    } catch (e) {
      status = '✗ ' + e.message
    } finally {
      busy = false
      setTimeout(() => (status = ''), 3000)
    }
  }

  async function play() {
    if (!preset) {
      status = '⚠ select a preset first'
      setTimeout(() => (status = ''), 3000)
      return
    }
    busy = true; status = ''
    try {
      await post('/api/play', { camera: camera.name, preset, gain })
      status = '✓'
    } catch (e) {
      status = '✗ ' + e.message
    } finally {
      busy = false
      setTimeout(() => (status = ''), 3000)
    }
  }

  async function playUrl() {
    if (!url) return
    busy = true; status = ''
    try {
      await post('/api/play-url', { camera: camera.name, url, gain })
      status = '✓'
    } catch (e) {
      status = '✗ ' + e.message
    } finally {
      busy = false
      setTimeout(() => (status = ''), 3000)
    }
  }

  async function beep() {
    busy = true; status = ''
    try {
      await post('/api/beep', { camera: camera.name })
      status = '✓ beep'
    } catch (e) {
      status = '✗ ' + e.message
    } finally {
      busy = false
      setTimeout(() => (status = ''), 3000)
    }
  }

  async function describe() {
    busy = true; status = '👁 fetching snapshot…'
    if (snapshot) URL.revokeObjectURL(snapshot)
    snapshot = ''; description = ''
    try {
      // Fetch snapshot from Frigate directly for instant display
      const snapRes = await fetch(`/api/snapshot/${camera.name}`)
      if (!snapRes.ok) throw new Error('snapshot fetch failed')
      const snapBlob = await snapRes.blob()
      snapshot = URL.createObjectURL(snapBlob)
      status = '👁 analyzing…'

      // Now run the full describe pipeline (vision → TTS → camera)
      const res = await fetch('/api/describe', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ camera: camera.name, gain }),
      })
      if (!res.ok) throw new Error(await res.text())
      const data = await res.json()
      description = data.description || ''
      text = description  // load into TTS box for replay
      status = '✓ described — text loaded for replay'
    } catch (e) {
      status = '✗ ' + e.message
    } finally {
      busy = false
      setTimeout(() => (status = ''), 3000)
    }
  }
</script>

<Card class="flex flex-col gap-2.5 p-4 transition-colors hover:border-primary/50 {!camera.online ? 'opacity-60' : ''}">
  <div class="flex items-center justify-between">
    <div class="flex items-center gap-2">
      <span class="h-2 w-2 rounded-full {camera.online ? 'bg-green-500 shadow-[0_0_6px_rgba(34,197,94,0.5)]' : 'bg-muted-foreground/40'}"></span>
      <span class="font-semibold">{camera.name}</span>
      <Badge variant="secondary" class="text-xs">{camera.type}</Badge>
    </div>
    <div class="flex gap-1">
      <Button variant="outline" size="icon" onclick={describe} disabled={busy} title="Describe & speak" class="h-7 w-7">👁</Button>
      <Button variant="outline" size="icon" onclick={beep} disabled={busy} title="Test beep" class="h-7 w-7">🔔</Button>
    </div>
  </div>

  <div class="flex gap-1.5">
    <Input
      bind:value={text}
      placeholder="Say something..."
      onkeydown={e => e.key === 'Enter' && speak()}
      disabled={busy}
      class="flex-1 text-sm"
    />
    <select bind:value={voice} disabled={busy} class="max-w-[120px] rounded-md border border-input bg-transparent px-3 py-1 text-sm disabled:opacity-50">
      <option value="">default</option>
      {#each voices as v}
        <option>{v}</option>
      {/each}
    </select>
    <Button size="sm" onclick={speak} disabled={busy || !text}>▶</Button>
  </div>

  <div class="flex items-center gap-2 px-1">
    <span class="text-xs text-muted-foreground whitespace-nowrap">gain</span>
    <input type="range" min="1" max="10" step="0.5" bind:value={gain} disabled={busy} class="flex-1 accent-primary" />
    <span class="text-xs text-muted-foreground font-mono w-8">{gain}x</span>
  </div>

  {#if presets.length > 0}
    <div class="flex gap-1.5">
      <select bind:value={preset} disabled={busy} class="flex-1 rounded-md border border-input bg-transparent px-3 py-1 text-sm disabled:opacity-50">
        <option value="">— play preset —</option>
        {#each presets as p}
          <option value={p.name}>{p.category}/{p.name}</option>
        {/each}
      </select>
      <Button size="sm" onclick={play} disabled={busy || !preset}>▶</Button>
    </div>
  {/if}

  <div class="flex gap-1.5">
    <Input
      bind:value={url}
      placeholder="Play from URL..."
      onkeydown={e => e.key === 'Enter' && playUrl()}
      disabled={busy}
      class="flex-1 text-sm"
    />
    <Button size="sm" onclick={playUrl} disabled={busy || !url}>▶</Button>
  </div>

  {#if status}<div class="text-sm text-primary">{status}</div>{/if}

  {#if snapshot}
    <div class="rounded-lg border border-primary/30 overflow-hidden">
      <img src={snapshot} alt="Camera snapshot" class="w-full" />
      {#if description}
        <p class="p-2 text-xs text-muted-foreground bg-muted/30">{description}</p>
      {/if}
    </div>
  {/if}
</Card>
