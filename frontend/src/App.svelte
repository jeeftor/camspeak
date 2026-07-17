<script>
  import { onMount } from 'svelte'
  import { Radio, Loader2 } from 'lucide-svelte'
  import CameraGrid from './components/CameraGrid.svelte'
  import Library from './components/Library.svelte'
  import EventLog from './components/EventLog.svelte'
  import Broadcast from './components/Broadcast.svelte'
  import Frigate from './components/Frigate.svelte'
  import Config from './components/Config.svelte'
  import RestDocs from './components/RestDocs.svelte'
  import McpDocs from './components/McpDocs.svelte'
  import HomeAssistant from './components/HomeAssistant.svelte'

  let tab = $state('cameras')
  let cameras = $state([])
  let voices = $state([])
  let presets = $state([])
  let version = $state('')
  let loading = $state(false)
  let loadError = $state('')

  async function loadAll() {
    loading = true
    loadError = ''
    try {
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
    } catch (e) {
      loadError = 'Failed to load data: ' + e.message
    } finally {
      loading = false
    }
  }

  onMount(loadAll)

  const tabs = [
    { id: 'cameras',   label: 'Cameras' },
    { id: 'library',   label: 'Library' },
    { id: 'events',    label: 'Events' },
    { id: 'broadcast', label: 'Broadcast' },
    { id: 'frigate',   label: 'Frigate' },
    { id: 'ha',        label: 'Home Assistant' },
    { id: 'config',    label: 'Config' },
    { id: 'rest',      label: 'REST' },
    { id: 'mcp',       label: 'MCP' },
  ]
</script>

<div class="flex min-h-dvh flex-col bg-background">
  <!-- Header -->
  <header class="sticky top-0 z-50 border-b bg-card/95 backdrop-blur supports-[backdrop-filter]:bg-card/80 shadow-sm">
    <div class="flex items-center gap-3 px-4 py-2 sm:px-6">
      <!-- Logo -->
      <div class="flex items-center gap-2 font-bold tracking-wide text-primary flex-shrink-0">
        <Radio class="h-4 w-4" />
        <span class="text-base">camspeak</span>
      </div>

      <!-- Tab nav — horizontally scrollable on mobile -->
      <nav class="flex gap-0.5 flex-1 overflow-x-auto" style="scrollbar-width:none;-webkit-overflow-scrolling:touch;">
        {#each tabs as t}
          <button
            class="px-2.5 py-1.5 text-sm rounded-md font-medium whitespace-nowrap transition-colors flex-shrink-0
              {tab === t.id
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:text-foreground hover:bg-muted'}"
            onclick={() => tab = t.id}
          >
            {t.label}
          </button>
        {/each}
      </nav>

      <!-- Version badge -->
      {#if version}
        <span class="flex-shrink-0 text-xs text-muted-foreground font-mono hidden sm:block
                     bg-muted/60 border px-2 py-0.5 rounded-full">{version}</span>
      {/if}
    </div>
  </header>

  <!-- Main content -->
  <main class="flex-1 px-4 py-6 sm:px-6">
    <div class="mx-auto w-full max-w-6xl">
      {#if loading && cameras.length === 0 && presets.length === 0}
        <div class="flex items-center gap-2 text-muted-foreground">
          <Loader2 class="h-4 w-4 animate-spin" />
          Loading…
        </div>
      {:else if loadError}
        <p class="text-sm text-destructive">{loadError}</p>
      {:else if tab === 'cameras'}
        <CameraGrid {cameras} {voices} {presets} onRefresh={loadAll} />
      {:else if tab === 'library'}
        <Library {presets} {voices} onRefresh={loadAll} />
      {:else if tab === 'events'}
        <EventLog />
      {:else if tab === 'broadcast'}
        <Broadcast {voices} {presets} />
      {:else if tab === 'frigate'}
        <Frigate />
      {:else if tab === 'ha'}
        <HomeAssistant />
      {:else if tab === 'config'}
        <Config onRefresh={loadAll} />
      {:else if tab === 'rest'}
        <RestDocs />
      {:else if tab === 'mcp'}
        <McpDocs />
      {/if}
    </div>
  </main>
</div>
