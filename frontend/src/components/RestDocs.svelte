<script>
  import JsonCode from '$lib/components/JsonCode.svelte'

  let expanded = $state({})
  function toggle(id) { expanded[id] = !expanded[id] }

  const endpoints = [
    {
      id: 'speak',
      method: 'POST',
      path: '/api/speak',
      summary: 'Text-to-speech on a single camera',
      body: { camera: 'backyard', text: 'Hello world', voice: 'af_sky', gain: 3.0 },
      response: { status: 'ok' },
    },
    {
      id: 'broadcast',
      method: 'POST',
      path: '/api/broadcast',
      summary: 'TTS or preset to all cameras simultaneously',
      body: { text: 'Announcement text', voice: 'af_sky', gain: 3.0 },
      response: { status: 'ok' },
    },
    {
      id: 'play',
      method: 'POST',
      path: '/api/play',
      summary: 'Play a saved library preset on a camera',
      body: { camera: 'backyard', preset: 'person_detected', category: 'alerts', gain: 3.0 },
      response: { status: 'ok' },
    },
    {
      id: 'play-url',
      method: 'POST',
      path: '/api/play-url',
      summary: 'Download audio from URL, transcode, and play on camera',
      body: { camera: 'backyard', url: 'http://host/audio.wav', gain: 3.0 },
      response: { status: 'ok' },
    },
    {
      id: 'beep',
      method: 'POST',
      path: '/api/beep',
      summary: 'Play an 800 Hz test beep on a camera',
      body: { camera: 'backyard' },
      response: { status: 'ok' },
    },
    {
      id: 'stop',
      method: 'POST',
      path: '/api/stop',
      summary: 'Stop audio on a specific camera or all cameras',
      body: { camera: 'backyard' },
      response: { status: 'stopped', camera: 'backyard' },
    },
    {
      id: 'snapshot',
      method: 'GET',
      path: '/api/snapshot/:camera',
      summary: 'Fetch a JPEG snapshot from the camera',
      body: null,
      response: '<binary JPEG>',
    },
    {
      id: 'vision',
      method: 'POST',
      path: '/api/vision',
      summary: 'Snapshot → vision model → returns text description only',
      body: { camera: 'backyard', prompt: 'How many people do you see?' },
      response: { description: 'There are 2 people in the driveway.' },
    },
    {
      id: 'describe',
      method: 'POST',
      path: '/api/describe',
      summary: 'Snapshot → vision model → TTS → speak on camera',
      body: { camera: 'backyard', voice: 'af_sky', gain: 3.0, prompt: 'Describe what you see.' },
      response: { description: 'A car is parked in the driveway.', status: 'ok' },
    },
    {
      id: 'cameras',
      method: 'GET',
      path: '/api/cameras',
      summary: 'List all cameras with online status',
      body: null,
      response: [{ name: 'backyard', type: 'hikvision', online: true }],
    },
    {
      id: 'voices',
      method: 'GET',
      path: '/api/voices',
      summary: 'List available TTS voices from the active TTS preset',
      body: null,
      response: ['af_sky', 'af_bella', 'af_heart'],
    },
    {
      id: 'library-list',
      method: 'GET',
      path: '/api/library',
      summary: 'List all saved audio presets',
      body: null,
      response: [{ name: 'person_detected', category: 'alerts', duration: 1.4, text: 'Person detected' }],
    },
    {
      id: 'library-gen',
      method: 'POST',
      path: '/api/library',
      summary: 'Generate a TTS clip and save as a preset',
      body: { name: 'person_detected', text: 'Person detected', category: 'alerts', voice: 'af_sky' },
      response: { name: 'person_detected', category: 'alerts', duration: 1.4 },
    },
    {
      id: 'library-upload',
      method: 'POST',
      path: '/api/library/upload',
      summary: 'Upload an audio file (any format, ffmpeg transcodes to G.711)',
      body: 'multipart/form-data: name, category, file',
      response: { name: 'my_clip', category: 'uploads', duration: 3.2 },
    },
    {
      id: 'library-preview',
      method: 'GET',
      path: '/api/library/:category/:name/preview',
      summary: 'Stream the audio for a preset (for in-browser playback)',
      body: null,
      response: '<binary audio>',
    },
    {
      id: 'library-delete',
      method: 'DELETE',
      path: '/api/library/:category/:name',
      summary: 'Delete a library preset',
      body: null,
      response: { status: 'ok' },
    },
    {
      id: 'tts-preview',
      method: 'POST',
      path: '/api/tts/preview',
      summary: 'Generate a TTS preview (audio blob, not saved)',
      body: { text: 'Hello world', voice: 'af_sky' },
      response: '<binary audio>',
    },
    {
      id: 'events',
      method: 'GET',
      path: '/api/events',
      summary: 'Server-Sent Events stream of speak/play/beep/broadcast/describe actions',
      body: null,
      response: 'data: {"camera":"backyard","action":"speak","text":"Hello","at":"2024-01-01T12:00:00Z"}',
    },
    {
      id: 'health',
      method: 'GET',
      path: '/api/health',
      summary: 'Health check — returns version and status',
      body: null,
      response: { status: 'ok', version: 'v1.6.1' },
    },
  ]

  const methodColor = {
    GET: 'text-green-400 bg-green-500/10 border-green-500/30',
    POST: 'text-blue-400 bg-blue-500/10 border-blue-500/30',
    DELETE: 'text-red-400 bg-red-500/10 border-red-500/30',
    PATCH: 'text-yellow-400 bg-yellow-500/10 border-yellow-500/30',
  }
</script>

<div class="flex flex-col gap-4 max-w-3xl">
  <div>
    <h2 class="text-lg font-semibold text-primary mb-1">REST API</h2>
    <p class="text-sm text-muted-foreground">
      All endpoints are served at the same host:port as the UI. JSON body, JSON response unless noted.
    </p>
  </div>

  <div class="flex flex-col gap-2">
    {#each endpoints as ep}
      <div class="rounded-lg border bg-card overflow-hidden">
        <!-- Header row (always visible) -->
        <button
          class="flex w-full items-center gap-3 px-4 py-3 text-left hover:bg-muted/30 transition-colors"
          onclick={() => toggle(ep.id)}
        >
          <span class="inline-flex items-center rounded border px-2 py-0.5 text-xs font-mono font-bold flex-shrink-0 {methodColor[ep.method] ?? 'text-muted-foreground'}">
            {ep.method}
          </span>
          <code class="text-sm font-mono text-foreground flex-shrink-0">{ep.path}</code>
          <span class="text-sm text-muted-foreground truncate">{ep.summary}</span>
          <span class="ml-auto text-xs text-muted-foreground flex-shrink-0">{expanded[ep.id] ? '▲' : '▼'}</span>
        </button>

        <!-- Expanded detail -->
        {#if expanded[ep.id]}
          <div class="border-t px-4 py-3 flex flex-col gap-3 bg-muted/20">
            {#if ep.body !== null}
              <div>
                <p class="text-xs font-semibold text-muted-foreground mb-1">Request body</p>
                <JsonCode code={typeof ep.body === 'string' ? ep.body : JSON.stringify(ep.body, null, 2)} class="text-xs" />
              </div>
            {/if}
            <div>
              <p class="text-xs font-semibold text-muted-foreground mb-1">Response</p>
              <JsonCode code={typeof ep.response === 'string' ? ep.response : JSON.stringify(ep.response, null, 2)} class="text-xs" />
            </div>
          </div>
        {/if}
      </div>
    {/each}
  </div>

  <div class="rounded-lg border bg-card px-4 py-3 text-sm text-muted-foreground">
    <p>Config endpoints: <code class="font-mono text-xs">/api/config</code>, <code class="font-mono text-xs">/api/config/tts</code>, <code class="font-mono text-xs">/api/config/cameras</code>, <code class="font-mono text-xs">/api/config/rules</code> — see the Config tab to manage these.</p>
    <p class="mt-1">Rate limit: 10 req/s per IP, burst 20.</p>
  </div>
</div>
