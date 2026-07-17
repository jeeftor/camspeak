<script>
  let expanded = $state({})
  function toggle(id) { expanded[id] = !expanded[id] }
  let copied = $state(false)

  const mcpUrl = typeof window !== 'undefined'
    ? `${window.location.protocol}//${window.location.host}/mcp`
    : '/mcp'

  function copyUrl() {
    navigator.clipboard.writeText(mcpUrl).then(() => {
      copied = true
      setTimeout(() => copied = false, 2000)
    })
  }

  const tools = [
    {
      id: 'speak',
      name: 'speak',
      description: 'Send text-to-speech audio to a named camera speaker.',
      params: [
        { name: 'camera', type: 'string', required: true, desc: 'Camera name (e.g. backyard, frontyard)' },
        { name: 'text', type: 'string', required: true, desc: 'Text to speak aloud' },
        { name: 'voice', type: 'string', required: false, desc: 'TTS voice (e.g. af_sky, af_bella)' },
      ],
      example: { camera: 'backyard', text: 'A package has arrived', voice: 'af_sky' },
    },
    {
      id: 'broadcast',
      name: 'broadcast',
      description: 'Send TTS or a preset to all cameras simultaneously.',
      params: [
        { name: 'text', type: 'string', required: false, desc: 'Text to speak (required if no preset)' },
        { name: 'preset', type: 'string', required: false, desc: 'Library preset name to play' },
        { name: 'voice', type: 'string', required: false, desc: 'TTS voice' },
      ],
      example: { text: 'Please come inside, dinner is ready' },
    },
    {
      id: 'play_preset',
      name: 'play_preset',
      description: 'Play a saved audio preset on a specific camera speaker.',
      params: [
        { name: 'camera', type: 'string', required: true, desc: 'Camera name' },
        { name: 'preset', type: 'string', required: true, desc: 'Preset name from the library' },
        { name: 'category', type: 'string', required: false, desc: 'Preset category (optional, helps with ambiguous names)' },
      ],
      example: { camera: 'frontyard', preset: 'person_detected', category: 'alerts' },
    },
    {
      id: 'list_cameras',
      name: 'list_cameras',
      description: 'List all configured cameras and their current online status.',
      params: [],
      example: {},
    },
    {
      id: 'list_presets',
      name: 'list_presets',
      description: 'List all saved audio presets in the library.',
      params: [],
      example: {},
    },
    {
      id: 'generate_preset',
      name: 'generate_preset',
      description: 'Generate a TTS audio clip and save it as a reusable library preset.',
      params: [
        { name: 'name', type: 'string', required: true, desc: 'Preset name (e.g. dog_warning)' },
        { name: 'text', type: 'string', required: true, desc: 'Text to synthesize' },
        { name: 'category', type: 'string', required: false, desc: 'Category (default: alerts)' },
        { name: 'voice', type: 'string', required: false, desc: 'TTS voice' },
      ],
      example: { name: 'dog_warning', text: 'Please keep your dog on a leash', category: 'alerts' },
    },
    {
      id: 'beep',
      name: 'beep',
      description: 'Play an 800 Hz test beep on a camera to verify audio is working.',
      params: [
        { name: 'camera', type: 'string', required: true, desc: 'Camera name' },
      ],
      example: { camera: 'backyard' },
    },
  ]
</script>

<div class="flex flex-col gap-6 max-w-3xl">
  <div>
    <h2 class="text-lg font-semibold text-primary mb-1">MCP Tools</h2>
    <p class="text-sm text-muted-foreground">
      camspeak exposes an MCP (Model Context Protocol) endpoint so AI assistants like Claude can speak
      through your cameras directly.
    </p>
  </div>

  <!-- Connection info -->
  <div class="rounded-lg border bg-card p-4 flex flex-col gap-3">
    <h3 class="text-sm font-semibold text-foreground">Connecting</h3>
    <p class="text-sm text-muted-foreground">
      Add camspeak as an MCP server in your AI tool's config (e.g. Claude Desktop,
      <code class="font-mono text-xs bg-muted px-1 rounded">claude_desktop_config.json</code>):
    </p>
    <pre class="text-xs bg-background rounded-md border p-3 overflow-x-auto text-foreground/80">{JSON.stringify({
  mcpServers: {
    camspeak: {
      url: mcpUrl,
      transport: 'http-stream',
    }
  }
}, null, 2)}</pre>
    <div class="flex items-center gap-3">
      <div class="flex items-center gap-2 rounded-md border bg-muted px-3 py-1.5 font-mono text-sm flex-1 truncate">
        {mcpUrl}
      </div>
      <button
        onclick={copyUrl}
        class="flex-shrink-0 rounded-md border px-3 py-1.5 text-sm hover:bg-muted transition-colors"
      >
        {copied ? '✓ Copied' : 'Copy URL'}
      </button>
    </div>
  </div>

  <!-- Tools list -->
  <div class="flex flex-col gap-2">
    <h3 class="text-sm font-semibold text-muted-foreground">Available Tools ({tools.length})</h3>
    {#each tools as tool}
      <div class="rounded-lg border bg-card overflow-hidden">
        <button
          class="flex w-full items-center gap-3 px-4 py-3 text-left hover:bg-muted/30 transition-colors"
          onclick={() => toggle(tool.id)}
        >
          <code class="text-sm font-mono text-primary">{tool.name}</code>
          <span class="text-sm text-muted-foreground truncate">{tool.description}</span>
          <span class="ml-auto text-xs text-muted-foreground flex-shrink-0">{expanded[tool.id] ? '▲' : '▼'}</span>
        </button>

        {#if expanded[tool.id]}
          <div class="border-t px-4 py-3 flex flex-col gap-3 bg-muted/20">
            {#if tool.params.length > 0}
              <div>
                <p class="text-xs font-semibold text-muted-foreground mb-2">Parameters</p>
                <div class="flex flex-col gap-1.5">
                  {#each tool.params as p}
                    <div class="flex flex-wrap items-start gap-2 text-sm">
                      <code class="font-mono text-xs text-foreground">{p.name}</code>
                      <span class="text-xs text-muted-foreground bg-muted px-1.5 rounded">{p.type}</span>
                      {#if p.required}
                        <span class="text-xs text-primary bg-primary/10 px-1.5 rounded">required</span>
                      {:else}
                        <span class="text-xs text-muted-foreground">optional</span>
                      {/if}
                      <span class="text-xs text-muted-foreground">{p.desc}</span>
                    </div>
                  {/each}
                </div>
              </div>
            {:else}
              <p class="text-xs text-muted-foreground">No parameters.</p>
            {/if}

            {#if Object.keys(tool.example).length > 0}
              <div>
                <p class="text-xs font-semibold text-muted-foreground mb-1">Example call</p>
                <pre class="text-xs bg-background rounded-md border p-3 overflow-x-auto text-foreground/80">{JSON.stringify(tool.example, null, 2)}</pre>
              </div>
            {/if}
          </div>
        {/if}
      </div>
    {/each}
  </div>

  <p class="text-xs text-muted-foreground">
    Protocol: MCP Streamable HTTP. Endpoint: <code class="font-mono">POST /mcp</code>. No authentication required on LAN.
  </p>
</div>
