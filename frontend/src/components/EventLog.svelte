<script lang="ts">
  import { onMount } from 'svelte'
  import CopyButton from '$lib/components/CopyButton.svelte'
  import CurlCode from '$lib/components/CurlCode.svelte'
  import JsonCode from '$lib/components/JsonCode.svelte'
  import { buildCurl } from '$lib/curl.svelte'
  import { eventKey, eventRequest, mergePlaybackEvent, type PlaybackEvent } from '$lib/playback-events'

  let events = $state<PlaybackEvent[]>([])
  let connected = $state(false)
  onMount(() => {
    const source = new EventSource('/api/events')
    source.onopen = () => connected = true
    source.onerror = () => connected = false
    source.onmessage = message => {
      try { events = mergePlaybackEvent(events, JSON.parse(message.data)) } catch { /* Ignore malformed event frames. */ }
    }
    return () => source.close()
  })
</script>

<section class="activity flex flex-col gap-4">
  <div>
    <h2 class="text-lg font-semibold text-primary">Playback activity</h2>
    <p class="text-sm text-muted-foreground">Recent playback and controls—not settings or vision-only tests. Commands are copied, never run automatically.</p>
    <p class="text-xs text-muted-foreground" role="status">{connected ? 'Live · recent history restored on connection' : 'Connecting to activity stream…'}</p>
  </div>
  {#if !events.length}<p class="text-sm text-muted-foreground">No recent playback activity.</p>{/if}
  {#each events as event (eventKey(event))}
    {@const request = eventRequest(event)}
    {@const curl = request ? buildCurl(request.method, request.path, request.body) : ''}
    {@const json = request ? JSON.stringify(request.body, null, 2) : ''}
    <article class="min-w-0 rounded-lg border bg-card p-3 text-sm">
      <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
        <strong>{event.camera || 'All cameras'}</strong><span class="text-primary">{event.action}</span>
        <time class="text-xs text-muted-foreground" datetime={event.at}>{new Date(event.at).toLocaleString()}</time>
      </div>
      {#if event.text}<p class="mt-2 whitespace-pre-wrap break-words">{event.text}</p>{/if}
      {#if event.voice}<p class="mt-1 text-xs text-muted-foreground">Voice: {event.voice}</p>{/if}
      {#if request}
        <div class="mt-3 flex flex-wrap items-center gap-3">
          <code class="break-all text-xs text-muted-foreground">{request.method} {request.path}</code>
          <span class="flex items-center gap-1"><CopyButton text={curl} label="Copy cURL" preview previewType="curl" class="h-8 w-8" />cURL</span>
          <span class="flex items-center gap-1"><CopyButton text={json} label="Copy JSON body" preview class="h-8 w-8" />JSON body</span>
        </div>
        {#if request.redacted}<p class="mt-2 text-xs text-amber-500">URL credentials and query parameters were removed. Supply them privately before running this command.</p>{/if}
        <details class="mt-2 min-w-0">
          <summary class="cursor-pointer text-xs text-muted-foreground">View REST command</summary>
          <div class="mt-2 overflow-x-auto rounded border p-2"><CurlCode code={curl} /></div>
          <div class="mt-2 overflow-x-auto rounded border p-2"><JsonCode code={json} /></div>
        </details>
      {:else}
        <p class="mt-2 text-xs text-muted-foreground">This older event did not save enough request options to reproduce it safely.</p>
      {/if}
    </article>
  {/each}
</section>

<style>
  .activity :global(.curl-tooltip) {
    min-width: min(320px, calc(100vw - 48px));
    max-width: min(520px, calc(100vw - 48px));
  }
</style>
