<script>
  import { onMount, onDestroy } from 'svelte'

  let events = $state([])
  let es

  onMount(() => {
    es = new EventSource('/api/events')
    es.onmessage = (e) => {
      try {
        const ev = JSON.parse(e.data)
        events = [{ ...ev, id: Date.now() }, ...events].slice(0, 100)
      } catch {}
    }
    es.onerror = () => {}
  })

  onDestroy(() => es?.close())

  function fmt(t) {
    return new Date(t).toLocaleTimeString()
  }

  const icons = { speak: '🗣', play: '▶', beep: '🔔', broadcast: '📢' }
</script>

<div class="log-wrap">
  <div class="log-header">
    <h2>Live Event Log</h2>
    {#if events.length > 0}
      <button onclick={() => events = []}>Clear</button>
    {/if}
  </div>

  {#if events.length === 0}
    <p class="empty">Waiting for events…</p>
  {:else}
    <div class="log">
      {#each events as ev (ev.id)}
        <div class="ev">
          <span class="icon">{icons[ev.action] ?? '•'}</span>
          <span class="time">{fmt(ev.at)}</span>
          <span class="camera">{ev.camera}</span>
          <span class="action">{ev.action}</span>
          {#if ev.text}<span class="evtext">"{ev.text}"</span>{/if}
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .log-wrap { display: flex; flex-direction: column; gap: 1rem; }
  .log-header { display: flex; align-items: center; justify-content: space-between; }
  h2 { font-size: 1.1rem; color: #a78bfa; }
  .log-header button {
    padding: 0.3rem 0.7rem;
    background: transparent;
    border: 1px solid #3a3a50;
    border-radius: 6px;
    color: #888;
    font-size: 0.85rem;
  }
  .log { display: flex; flex-direction: column; gap: 0.3rem; font-family: 'Menlo', 'Consolas', monospace; }
  .ev {
    display: flex;
    align-items: baseline;
    gap: 0.6rem;
    padding: 0.35rem 0.7rem;
    background: #1a1a24;
    border-radius: 6px;
    border-left: 3px solid #4c1d95;
    font-size: 0.85rem;
    animation: fadeIn 0.2s ease;
  }
  @keyframes fadeIn { from { opacity: 0; transform: translateY(-4px); } to { opacity: 1; } }
  .icon { font-size: 1rem; }
  .time { color: #666; white-space: nowrap; }
  .camera { color: #a78bfa; font-weight: 600; }
  .action { color: #888; }
  .evtext { color: #ccc; font-style: italic; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .empty { color: #555; font-style: italic; }
</style>
