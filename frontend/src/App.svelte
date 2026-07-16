<script>
  import { onMount } from 'svelte'
  import CameraGrid from './components/CameraGrid.svelte'
  import Library from './components/Library.svelte'
  import EventLog from './components/EventLog.svelte'
  import BroadcastBar from './components/BroadcastBar.svelte'
  import Config from './components/Config.svelte'
  import { Button } from '$lib/components/ui/button'

  let tab = $state('cameras')
  let cameras = $state([])
  let voices = $state([])
  let presets = $state([])
  let version = $state('')

  async function loadAll() {
    const [camRes, voiceRes, presetRes, healthRes] = await Promise.all([
      fetch('/api/cameras'),
      fetch('/api/voices'),
      fetch('/api/library'),
      fetch('/api/health'),
    ])
    cameras = await camRes.json() ?? []
    voices = await voiceRes.json() ?? []
    presets = await presetRes.json() ?? []
    const health = await healthRes.json() ?? {}
    version = health.version ?? ''
  }

  onMount(loadAll)

  const tabs = [
    { id: 'cameras', label: 'Cameras' },
    { id: 'library', label: 'Library' },
    { id: 'events', label: 'Events' },
    { id: 'config', label: 'Config' },
  ]
</script>

<div class="flex min-h-screen flex-col">
  <header class="flex items-center gap-8 border-b px-6 py-3 bg-card">
    <div class="text-lg font-bold tracking-wide text-primary">📢 camspeak</div>
    <nav class="flex gap-1">
      {#each tabs as t}
        <Button
          variant={tab === t.id ? 'default' : 'ghost'}
          size="sm"
          onclick={() => tab = t.id}
        >
          {t.label}
        </Button>
      {/each}
    </nav>
    {#if version}
      <span class="ml-auto text-xs text-muted-foreground font-mono">{version}</span>
    {/if}
  </header>

  <BroadcastBar {voices} {presets} />

  <main class="mx-auto w-full max-w-6xl flex-1 p-6">
    {#if tab === 'cameras'}
      <CameraGrid {cameras} {voices} {presets} onRefresh={loadAll} />
    {:else if tab === 'library'}
      <Library {presets} {voices} onRefresh={loadAll} />
    {:else if tab === 'config'}
      <Config onRefresh={loadAll} />
    {:else}
      <EventLog />
    {/if}
  </main>
</div>
