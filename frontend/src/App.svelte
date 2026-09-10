<script>
  import { onMount } from 'svelte'
  import { Loader2, Globe, Square } from 'lucide-svelte'
  import CameraGrid from './components/CameraGrid.svelte'
  import Library from './components/Library.svelte'
  import EventLog from './components/EventLog.svelte'
  import Config from './components/Config.svelte'
  import RestDocs from './components/RestDocs.svelte'
  import McpDocs from './components/McpDocs.svelte'
  import HomeAssistant from './components/HomeAssistant.svelte'
  import { curlState, setCurlBaseUrl, resetCurlBaseUrl } from '$lib/curl.svelte'
  import { Toaster, toast } from '$lib/components/ui/toast'
  import Announce from './components/Announce.svelte'
  import Diagnostics from './components/Diagnostics.svelte'
  import { apiClient } from '$lib/api'

  let tab = $state('cameras')
  let routeReady = $state(false)
  let cameras = $state([])
  let voices = $state([])
  let presets = $state([])
  let version = $state('')
  let loading = $state(false)
  let loadError = $state('')
  let showUrlEditor = $state(false)
  let urlEditValue = $state('')
  let stoppingAll = $state(false)
  let showApiMenu = $state(false)
  let apiMenuAnchor = $state({ bottom: 0, left: 0 })

  async function stopAll() {
    stoppingAll = true
    try {
      await apiClient.stopAll()
      toast.success('All camera audio stopped')
    } catch (e) {
      toast.error('Could not stop all cameras: ' + e.message)
    } finally {
      stoppingAll = false
    }
  }

  // --- Hash-based SPA routing ---
  const validTabs = ['cameras', 'library', 'events', 'announce', 'diagnostics', 'ha', 'config', 'rest', 'swagger', 'mcp']

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
    if (routeReady) setHash(tab)
  })

  // Sync hash → tab on back/forward
  onMount(() => {
    tab = tabFromHash()
    routeReady = true
    const onHashChange = () => { tab = tabFromHash() }
    window.addEventListener('hashchange', onHashChange)
    loadAll()
    let refreshing = false
    let active = true
    const cameraRefresh = setInterval(async () => {
      if (refreshing || document.hidden) return
      refreshing = true
      try {
        const data = await apiClient.getCameras()
        if (active) cameras = data ?? []
      } catch {
        // Retain the last metadata snapshot during a transient disconnect.
      } finally { refreshing = false }
    }, 10000)
    return () => {
      active = false
      clearInterval(cameraRefresh)
      window.removeEventListener('hashchange', onHashChange)
    }
  })

  async function loadAll() {
    loading = true
    loadError = ''
    const resources = [
      ['Cameras', () => apiClient.getCameras().then(data => cameras = data ?? [])],
      ['Voices', () => apiClient.getVoices().then(data => voices = data ?? [])],
      ['Library', () => apiClient.getPresets().then(data => presets = data ?? [])],
      ['Server version', () => apiClient.health().then(data => version = data.version ?? '')],
    ]
    const results = await Promise.allSettled(resources.map(([, load]) => load()))
    loadError = results.flatMap((result, i) => result.status === 'rejected'
      ? [`${resources[i][0]}: ${result.reason?.message ?? result.reason}`] : []).join(' · ')
    loading = false
  }

  const tabs = [
    { id: 'cameras', label: 'Cameras' },
    { id: 'library', label: 'Library' },
    { id: 'events',  label: 'Events' },
    { id: 'announce', label: 'Announce' },
    { id: 'config',  label: 'Config' },
  ]

  // API sub-menu items (shown in dropdown under "API" button)
  const apiTabs = [
    { id: 'diagnostics', label: 'Diagnostics' },
    { id: 'ha',      label: 'Home Assistant' },
    { id: 'rest',    label: 'REST Docs' },
    { id: 'mcp',     label: 'MCP' },
  ]

  const apiTabIds = new Set(apiTabs.map(t => t.id))
</script>

<div class="flex min-h-dvh flex-col bg-background">
  <!-- Header -->
  <header class="sticky top-0 z-50 border-b bg-card/95 backdrop-blur supports-[backdrop-filter]:bg-card/80 shadow-sm">
    <!-- Top row: logo, stop, url/version -->
    <div class="flex items-center gap-3 px-4 py-2 sm:px-6">
      <!-- Logo -->
      <div class="flex items-center gap-2 font-bold tracking-wide text-primary flex-shrink-0">
        <img src="/camspeak-mark.svg" alt="camspeak" class="h-6 w-auto dark:hidden" />
        <img src="/camspeak-mark-dark.svg" alt="camspeak" class="h-6 w-auto hidden dark:block" />
        <span class="text-base">camspeak</span>
      </div>

      <!-- Desktop-only: inline tabs -->
      <nav class="hidden md:flex gap-0.5 flex-1">
        {#each tabs as t}
          <button
            class="px-2.5 py-1.5 text-sm rounded-md font-medium whitespace-nowrap transition-colors
              {tab === t.id
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:text-foreground hover:bg-muted'}"
            onclick={() => tab = t.id}
          >
            {t.label}
          </button>
        {/each}

        <!-- API dropdown trigger -->
        <div class="relative">
          <button
            class="px-2.5 py-1.5 text-sm rounded-md font-medium whitespace-nowrap transition-colors
              {apiTabIds.has(tab)
                ? 'bg-primary text-primary-foreground'
                : 'text-muted-foreground hover:text-foreground hover:bg-muted'}"
            onclick={(e) => { apiMenuAnchor = e.currentTarget.getBoundingClientRect(); showApiMenu = !showApiMenu }}
          >
            Tools ▾
          </button>
        </div>
      </nav>

      <div class="flex-1 md:hidden"></div>

      <!-- STOP ALL button -->
      <button
        class="flex items-center gap-1.5 flex-shrink-0 rounded-md bg-destructive px-3 py-1.5 text-sm font-bold
               text-destructive-foreground hover:bg-destructive/90 transition-colors
               disabled:opacity-50"
        onclick={stopAll}
        disabled={stoppingAll}
        title="Immediately stop all audio playback on all cameras"
      >
        {#if stoppingAll}
          <Loader2 class="h-3.5 w-3.5 animate-spin" />
        {:else}
          <Square class="h-3.5 w-3.5 fill-current" />
        {/if}
        <span class="hidden sm:inline">STOP</span>
      </button>

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
            <span class="font-mono max-w-[120px] truncate hidden sm:inline">{curlState.baseUrl.replace(/^https?:\/\//, '')}</span>
          </button>
          {#if showUrlEditor}
            <button class="fixed inset-0 z-40" aria-label="Close URL editor" onclick={() => showUrlEditor = false}></button>
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

    <!-- Mobile tab bar (second row, scrollable) -->
    <nav class="flex md:hidden gap-0.5 px-4 pb-2 overflow-x-auto" style="scrollbar-width:none;-webkit-overflow-scrolling:touch;">
      {#each tabs as t}
        <button
          class="px-2.5 py-1.5 text-xs rounded-md font-medium whitespace-nowrap transition-colors flex-shrink-0
            {tab === t.id
              ? 'bg-primary text-primary-foreground'
              : 'text-muted-foreground hover:text-foreground hover:bg-muted'}"
          onclick={() => tab = t.id}
        >
          {t.label}
        </button>
      {/each}
      <!-- API dropdown trigger (mobile) -->
      <button
        class="px-2.5 py-1.5 text-xs rounded-md font-medium whitespace-nowrap transition-colors flex-shrink-0
          {apiTabIds.has(tab)
            ? 'bg-primary text-primary-foreground'
            : 'text-muted-foreground hover:text-foreground hover:bg-muted'}"
        onclick={(e) => { apiMenuAnchor = e.currentTarget.getBoundingClientRect(); showApiMenu = !showApiMenu }}
      >
        Tools ▾
      </button>
    </nav>
  </header>

  <!-- API dropdown menu (portaled to body level, fixed position) -->
  {#if showApiMenu}
    <button class="fixed inset-0 z-[60]" aria-label="Close tools menu" onclick={() => showApiMenu = false}></button>
    <div
      class="fixed z-[61] min-w-[10rem] rounded-lg border bg-card shadow-lg py-1"
      style="top: {apiMenuAnchor.bottom + 4}px; left: {apiMenuAnchor.left}px;"
    >
      {#each apiTabs as t}
        <button
          class="w-full text-left px-3 py-1.5 text-sm transition-colors
            {tab === t.id
              ? 'bg-primary/10 text-primary font-medium'
              : 'text-foreground hover:bg-muted'}"
          onclick={() => { tab = t.id; showApiMenu = false }}
        >
          {t.label}
        </button>
      {/each}
      <div class="my-1 border-t"></div>
      <a
        href="/swagger"
        target="_blank"
        class="flex w-full items-center gap-1 px-3 py-1.5 text-sm text-foreground hover:bg-muted"
        onclick={() => showApiMenu = false}
      >
        Swagger UI ↗
      </a>
    </div>
  {/if}

  <!-- Main content -->
  <main class="flex-1 px-4 py-6 sm:px-6">
    <div class="mx-auto w-full max-w-6xl">
      {#if loading && cameras.length === 0 && presets.length === 0}
        <div class="flex items-center gap-2 text-muted-foreground">
          <Loader2 class="h-4 w-4 animate-spin" />
          Loading…
        </div>
      {/if}
      {#if loadError}
        <div role="alert" class="mb-4 flex flex-wrap items-center gap-3 rounded-lg border border-destructive/40 p-3 text-sm">
          <p class="text-destructive">Some data could not load. {loadError}</p>
          <button class="rounded-md border px-3 py-1 hover:bg-muted disabled:opacity-50" onclick={loadAll} disabled={loading}>Retry</button>
        </div>
      {/if}
      {#if tab === 'cameras'}
        <CameraGrid {cameras} {voices} {presets} onRefresh={loadAll} />
      {:else if tab === 'library'}
        <Library {presets} {voices} onRefresh={loadAll} />
      {:else if tab === 'events'}
        <EventLog />
      {:else if tab === 'announce'}
        <Announce {cameras} />
      {:else if tab === 'diagnostics'}
        <Diagnostics {cameras} />
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

<Toaster />
