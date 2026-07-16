<script>
  import { onMount } from 'svelte'
  import CameraGrid from './components/CameraGrid.svelte'
  import Library from './components/Library.svelte'
  import EventLog from './components/EventLog.svelte'
  import BroadcastBar from './components/BroadcastBar.svelte'
  import Config from './components/Config.svelte'

  let tab = $state('cameras')
  let cameras = $state([])
  let voices = $state([])
  let presets = $state([])

  async function loadAll() {
    const [camRes, voiceRes, presetRes] = await Promise.all([
      fetch('/api/cameras'),
      fetch('/api/voices'),
      fetch('/api/library'),
    ])
    cameras = await camRes.json() ?? []
    voices = await voiceRes.json() ?? []
    presets = await presetRes.json() ?? []
  }

  onMount(loadAll)
</script>

<div class="app">
  <header>
    <div class="logo">📢 camspeak</div>
    <nav>
      <button class:active={tab === 'cameras'} onclick={() => tab = 'cameras'}>Cameras</button>
      <button class:active={tab === 'library'} onclick={() => tab = 'library'}>Library</button>
      <button class:active={tab === 'events'} onclick={() => tab = 'events'}>Events</button>
      <button class:active={tab === 'config'} onclick={() => tab = 'config'}>Config</button>
    </nav>
  </header>

  <BroadcastBar {voices} {presets} />

  <main>
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

<style>
  :global(*, *::before, *::after) { box-sizing: border-box; margin: 0; padding: 0; }
  :global(body) {
    font-family: system-ui, -apple-system, sans-serif;
    background: #0f0f13;
    color: #e8e8f0;
    min-height: 100vh;
  }
  :global(button) { cursor: pointer; }

  .app { display: flex; flex-direction: column; min-height: 100vh; }

  header {
    display: flex;
    align-items: center;
    gap: 2rem;
    padding: 0.75rem 1.5rem;
    background: #1a1a24;
    border-bottom: 1px solid #2a2a3a;
  }

  .logo {
    font-size: 1.2rem;
    font-weight: 700;
    color: #a78bfa;
    letter-spacing: 0.05em;
  }

  nav { display: flex; gap: 0.25rem; }

  nav button {
    padding: 0.4rem 1rem;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: #888;
    font-size: 0.9rem;
    transition: all 0.15s;
  }
  nav button:hover { background: #2a2a3a; color: #ccc; }
  nav button.active { background: #4c1d95; color: #e8e8f0; }

  main { flex: 1; padding: 1.5rem; max-width: 1200px; margin: 0 auto; width: 100%; }
</style>
