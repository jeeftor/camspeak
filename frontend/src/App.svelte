<script>
  import { onMount } from 'svelte'
  import { Radio, Loader2, Globe } from 'lucide-svelte'
  import CameraGrid from './components/CameraGrid.svelte'
  import Library from './components/Library.svelte'
  import EventLog from './components/EventLog.svelte'
  import Broadcast from './components/Broadcast.svelte'
  import Frigate from './components/Frigate.svelte'
  import Config from './components/Config.svelte'
  import RestDocs from './components/RestDocs.svelte'
  import McpDocs from './components/McpDocs.svelte'
  import HomeAssistant from './components/HomeAssistant.svelte'
  import VisionTest from './components/VisionTest.svelte'
  import { curlState, setCurlBaseUrl, resetCurlBaseUrl } from '$lib/curl.svelte'

  let tab = $state('cameras')
  let cameras = $state([])
  let voices = $state([])
  let presets = $state([])
  let version = $state('')
  let loading = $state(false)
  let loadError = $state('')
  let showUrlEditor = $state(false)
  let urlEditValue = $state('')
  let globalVisionPrompt = $state('')

  // --- Hash-based SPA routing ---
  const validTabs = ['cameras', 'library', 'events', 'broadcast', 'frigate', 'ha', 'config', 'vision-test', 'rest', 'swagger', 'mcp']

  function tabFromHash() {
    const h = window.location.hash.replace(/^#\/?/, '')
    return validTabs.includes(h) ? h : 'cameras'
  }

  function setHash(t) {
    if (window.location.hash !== `#/${t}`) {
      window.location.hash = `/${t}`
    }
  }

  // Sync tab → hash on change
  $effect(() => {
    setHash(tab)
  })

  // Sync hash → tab on back/forward
  onMount(() => {
    tab = tabFromHash()
    const onHashChange = () => { tab = tabFromHash() }
    window.addEventListener('hashchange', onHashChange)
    loadAll()
    return () => window.removeEventListener('hashchange', onHashChange)
  })

  async function loadAll() {
    loading = true
    loadError = ''
    try {
      const [camRes, voiceRes, presetRes, healthRes, visionRes] = await Promise.all([
        fetch('/api/cameras'),
        fetch('/api/voices'),
        fetch('/api/library'),
        fetch('/api/health'),
        fetch('/api/config/vision'),
      ])
      cameras = await camRes.json() ?? []
      voices = await voiceRes.json() ?? []
      presets = await presetRes.json() ?? []
      const health = await healthRes.json() ?? {}
      version = health.version ?? ''
      const v = await visionRes.json() ?? {}
      globalVisionPrompt = v.prompt ?? ''
    } catch (e) {
      loadError = 'Failed to load data: ' + e.message
    } finally {
      loading = false
    }
  }

  const tabs = [
    { id: 'cameras',     label: 'Cameras' },
    { id: 'library',     label: 'Library' },
    { id: 'events',      label: 'Events' },
    { id: 'broadcast',   label: 'Broadcast' },
    { id: 'frigate',     label: 'Frigate' },
    { id: 'ha',          label: 'Home Assistant' },
    { id: 'config',      label: 'Config' },
    { id: 'vision-test', label: 'Vision Test' },
    { id: 'rest',        label: 'REST' },
    { id: 'swagger',     label: 'Swagger' },
    { id: 'mcp',         label: 'MCP' },
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

      <!-- Curl base URL + version -->
      <div class="flex items-center gap-2 flex-shrink-0">
        <div class="relative">
          <button
            class="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground
                   bg-muted/60 border px-2 py-0.5 rounded-full transition-colors"
            onclick={() => { urlEditValue = curlState.baseUrl; showUrlEditor = !showUrlEditor }}
            title="Base URL for curl commands"
          >
            <Globe class="h-3 w-3" />
            <span class="font-mono max-w-[120px] truncate">{curlState.baseUrl.replace(/^https?:\/\//, '')}</span>
          </button>
          {#if showUrlEditor}
            <!-- svelte-ignore a11y_click_events_have_key_handlers, a11y_no_static_element_interactions -->
            <div class="fixed inset-0 z-40" onclick={() => showUrlEditor = false}></div>
            <div class="absolute right-0 top-full mt-1 z-50 w-72 rounded-lg border bg-card p-3 shadow-lg">
              <p class="text-xs text-muted-foreground mb-2">Base URL for curl commands</p>
              <input
                type="text"
                bind:value={urlEditValue}
                placeholder="http://192.168.1.100:8585"
                class="w-full rounded-md border border-input bg-transparent px-2 py-1 text-xs font-mono
                       focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
              />
              <div class="flex gap-1.5 mt-2">
                <button
                  class="flex-1 rounded-md bg-primary px-2 py-1 text-xs text-primary-foreground hover:bg-primary/90"
                  onclick={() => { setCurlBaseUrl(urlEditValue); showUrlEditor = false }}
                >Set</button>
                <button
                  class="flex-1 rounded-md border px-2 py-1 text-xs hover:bg-muted"
                  onclick={() => { resetCurlBaseUrl(); urlEditValue = curlState.baseUrl; showUrlEditor = false }}
                >Reset</button>
              </div>
            </div>
          {/if}
        </div>
        {#if version}
          <span class="text-xs text-muted-foreground font-mono hidden sm:block
                       bg-muted/60 border px-2 py-0.5 rounded-full">{version}</span>
        {/if}
      </div>
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
      {:else if tab === 'vision-test'}
        <VisionTest cameras={cameras} globalPrompt={globalVisionPrompt} onSavePrompt={async (p) => { globalVisionPrompt = p }} />
      {:else if tab === 'rest'}
        <RestDocs />
      {:else if tab === 'swagger'}
        <div class="rounded-lg border overflow-hidden">
          <iframe src="/swagger" class="w-full" style="height: calc(100vh - 120px); border: 0;" title="Swagger UI"></iframe>
        </div>
      {:else if tab === 'mcp'}
        <McpDocs />
      {/if}
    </div>
  </main>
</div>
