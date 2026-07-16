<script>
  import { onMount, onDestroy } from 'svelte'
  import { Button } from '$lib/components/ui/button'

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

<div class="flex flex-col gap-4">
  <div class="flex items-center justify-between">
    <h2 class="text-lg font-semibold text-primary">Live Event Log</h2>
    {#if events.length > 0}
      <Button variant="outline" size="sm" onclick={() => events = []}>Clear</Button>
    {/if}
  </div>

  {#if events.length === 0}
    <p class="italic text-muted-foreground">Waiting for events…</p>
  {:else}
    <div class="flex flex-col gap-1 font-mono">
      {#each events as ev (ev.id)}
        <div class="flex items-baseline gap-2.5 rounded-md border-l-4 border-primary bg-card px-3 py-1.5 text-sm animate-in fade-in slide-in-from-top-1 duration-200">
          <span class="text-base">{icons[ev.action] ?? '•'}</span>
          <span class="whitespace-nowrap text-muted-foreground">{fmt(ev.at)}</span>
          <span class="font-semibold text-primary">{ev.camera}</span>
          <span class="text-muted-foreground">{ev.action}</span>
          {#if ev.text}<span class="truncate italic text-foreground/80">"{ev.text}"</span>{/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
