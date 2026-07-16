<script>
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { Select } from '$lib/components/ui/select'
  import { Card } from '$lib/components/ui/card'
  import { Badge } from '$lib/components/ui/badge'

  let { camera, voices = [], presets = [] } = $props()

  let text = $state('')
  let voice = $state('')
  let preset = $state('')
  let url = $state('')
  let busy = $state(false)
  let status = $state('')

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
      await post('/api/speak', { camera: camera.name, text, voice })
      status = '✓'
      text = ''
    } catch (e) {
      status = '✗ ' + e.message
    } finally {
      busy = false
      setTimeout(() => (status = ''), 3000)
    }
  }

  async function play() {
    if (!preset) return
    busy = true; status = ''
    try {
      await post('/api/play', { camera: camera.name, preset })
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
      await post('/api/play-url', { camera: camera.name, url })
      status = '✓'
      url = ''
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
</script>

<Card class="flex flex-col gap-2.5 p-4 transition-colors hover:border-primary/50 {!camera.online ? 'opacity-60' : ''}">
  <div class="flex items-center justify-between">
    <div class="flex items-center gap-2">
      <span class="h-2 w-2 rounded-full {camera.online ? 'bg-green-500 shadow-[0_0_6px_rgba(34,197,94,0.5)]' : 'bg-muted-foreground/40'}"></span>
      <span class="font-semibold">{camera.name}</span>
      <Badge variant="secondary" class="text-xs">{camera.type}</Badge>
    </div>
    <Button variant="outline" size="icon" onclick={beep} disabled={busy} title="Test beep" class="h-7 w-7">🔔</Button>
  </div>

  <div class="flex gap-1.5">
    <Input
      bind:value={text}
      placeholder="Say something..."
      onkeydown={e => e.key === 'Enter' && speak()}
      disabled={busy}
      class="flex-1 text-sm"
    />
    <Select bind:value={voice} disabled={busy} class="max-w-[120px] text-sm">
      <option value="">default</option>
      {#each voices as v}
        <option>{v}</option>
      {/each}
    </Select>
    <Button size="sm" onclick={speak} disabled={busy || !text}>▶</Button>
  </div>

  {#if presets.length > 0}
    <div class="flex gap-1.5">
      <Select bind:value={preset} disabled={busy} class="flex-1 text-sm">
        <option value="">— play preset —</option>
        {#each presets as p}
          <option value={p.name}>{p.category}/{p.name}</option>
        {/each}
      </Select>
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
</Card>
