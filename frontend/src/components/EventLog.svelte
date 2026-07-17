<script>
  import { onMount, onDestroy } from 'svelte'
  import { Mic, Play, Bell, Radio, Trash2, Eye } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import CopyButton from '$lib/components/CopyButton.svelte'
  import { buildCurl } from '$lib/curl'

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

  const iconMap = { speak: Mic, play: Play, beep: Bell, broadcast: Radio, describe: Eye }

  // Reconstruct a curl command from an event
  function eventCurl(ev) {
    const actionMap = {
      speak: { method: 'POST', path: '/api/speak', body: { camera: ev.camera, text: ev.text } },
      describe: { method: 'POST', path: '/api/describe', body: { camera: ev.camera } },
      play: { method: 'POST', path: '/api/play', body: { camera: ev.camera } },
      beep: { method: 'POST', path: '/api/beep', body: { camera: ev.camera } },
      broadcast: { method: 'POST', path: '/api/broadcast', body: { text: ev.text } },
    }
    const a = actionMap[ev.action]
    if (!a) return ''
    return buildCurl(a.method, a.path, a.body)
  }
</script>

<div class="flex flex-col gap-4">
  <div class="flex items-center justify-between">
    <h2 class="text-lg font-semibold text-primary">Live Event Log</h2>
    {#if events.length > 0}
      <Button variant="outline" size="sm" onclick={() => events = []}>
        <Trash2 class="h-4 w-4" />
        Clear
      </Button>
    {/if}
  </div>

  {#if events.length === 0}
    <p class="italic text-muted-foreground">Waiting for events…</p>
  {:else}
    <div class="flex flex-col gap-1 font-mono">
      {#each events as ev (ev.id)}
        {@const Icon = iconMap[ev.action] ?? Mic}
        <div class="flex items-baseline gap-2.5 rounded-md border-l-4 border-primary bg-card px-3 py-1.5 text-sm animate-in fade-in slide-in-from-top-1 duration-200">
          <Icon class="h-4 w-4 text-primary flex-shrink-0 self-center" />
          <span class="whitespace-nowrap text-muted-foreground flex-shrink-0">{fmt(ev.at)}</span>
          <span class="font-semibold text-primary flex-shrink-0">{ev.camera}</span>
          <span class="text-muted-foreground flex-shrink-0">{ev.action}</span>
          {#if ev.text}
            <span class="break-words italic text-foreground/80 flex-1 min-w-0">"{ev.text}"</span>
          {/if}
          <CopyButton
            text={eventCurl(ev)}
            label="Copy curl command"
            class="h-6 w-6 flex-shrink-0 self-center"
          />
        </div>
      {/each}
    </div>
  {/if}
</div>
