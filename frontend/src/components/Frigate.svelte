<script>
  import { onMount, onDestroy } from 'svelte'
  import { Play, Loader2, Wifi, WifiOff, Radio, ChevronDown, ChevronUp, Trash2 } from 'lucide-svelte'
  // Wifi/WifiOff used in template directly (not via svelte:component)
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { Select } from '$lib/components/ui/select'

  let rules = $state([])
  let voices = $state([])
  let loading = $state(true)
  let testStatus = $state({})
  let error = $state('')

  // MQTT status
  let mqttStatus = $state('unknown') // not_configured | connected | disconnected | unknown
  let mqttBroker = $state('')

  // Live MQTT browser
  let mqttMessages = $state([])
  let mqttBrowsing = $state(false)
  let es = null

  // Rule form
  let ruleTopic = $state('frigate/events')
  let ruleFilter = $state('')
  let ruleCameras = $state('')
  let rulePreset = $state('')
  let ruleText = $state('')
  let ruleVoice = $state('')
  let ruleEnabled = $state(true)
  let ruleStatus = $state('')
  let formOpen = $state(false)

  // Common Frigate topics for datalist
  const topicSuggestions = [
    'frigate/events',
    'frigate/reviews',
    'frigate/stats',
    'frigate/+/motion',
    'frigate/+/person',
    'frigate/+/car',
    'frigate/+/dog',
    'frigate/+/cat',
    'frigate/#',
  ]

  // Filter templates: {label, filter} — filter is a flat key:value map
  const filterTemplates = [
    { label: 'New event (any label)', filter: 'type=new' },
    { label: 'New — person', filter: 'type=new,after.label=person' },
    { label: 'New — car', filter: 'type=new,after.label=car' },
    { label: 'New — dog', filter: 'type=new,after.label=dog' },
    { label: 'New — cat', filter: 'type=new,after.label=cat' },
    { label: 'Motion started', filter: 'type=start' },
    { label: 'Motion ended', filter: 'type=end' },
  ]

  // Rule templates
  const ruleTemplates = [
    {
      label: 'Person at front door',
      topic: 'frigate/events',
      filter: '{"type":"new","after.label":"person"}',
      cameras: 'frontyard',
      text: 'Person detected at the front door',
    },
    {
      label: 'Car in driveway',
      topic: 'frigate/events',
      filter: '{"type":"new","after.label":"car"}',
      cameras: '',
      text: 'Vehicle detected in the driveway',
    },
    {
      label: 'Dog in yard',
      topic: 'frigate/events',
      filter: '{"type":"new","after.label":"dog"}',
      cameras: 'backyard',
      text: 'Dog detected in the backyard',
    },
    {
      label: 'Any new detection',
      topic: 'frigate/events',
      filter: '{"type":"new"}',
      cameras: '',
      text: 'Motion detected',
    },
  ]

  async function load() {
    loading = true
    try {
      const [rulesRes, voiceRes, statusRes] = await Promise.all([
        fetch('/api/config/rules'),
        fetch('/api/voices'),
        fetch('/api/mqtt/status'),
      ])
      rules = await rulesRes.json() ?? []
      voices = await voiceRes.json() ?? []
      const s = await statusRes.json()
      mqttStatus = s.status ?? 'unknown'
      mqttBroker = s.broker ?? ''
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  onMount(load)

  onDestroy(() => {
    if (es) { es.close(); es = null }
  })

  function toggleBrowser() {
    if (mqttBrowsing) {
      es?.close(); es = null
      mqttBrowsing = false
    } else {
      mqttMessages = []
      es = new EventSource('/api/mqtt/events')
      es.onmessage = (e) => {
        try {
          const msg = JSON.parse(e.data)
          mqttMessages = [{ ...msg, id: Date.now() + Math.random() }, ...mqttMessages].slice(0, 100)
        } catch {}
      }
      es.onerror = () => {}
      mqttBrowsing = true
    }
  }

  function applyTemplate(tmpl) {
    ruleTopic = tmpl.topic
    ruleFilter = tmpl.filter
    ruleCameras = tmpl.cameras
    ruleText = tmpl.text
    formOpen = true
  }

  async function saveRule() {
    if (!ruleTopic) return
    ruleStatus = ''
    let filter = {}
    if (ruleFilter) {
      try { filter = JSON.parse(ruleFilter) }
      catch { ruleStatus = '✗ Invalid filter JSON'; return }
    }
    try {
      const res = await fetch('/api/config/rules', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          topic: ruleTopic,
          filter,
          cameras: ruleCameras.split(',').map(s => s.trim()).filter(Boolean),
          preset: rulePreset,
          text: ruleText,
          voice: ruleVoice,
          enabled: ruleEnabled,
        }),
      })
      if (!res.ok) throw new Error(await res.text())
      ruleStatus = '✓ Saved'
      ruleTopic = 'frigate/events'; ruleFilter = ''; ruleCameras = ''
      rulePreset = ''; ruleText = ''; ruleVoice = ''; ruleEnabled = true
      formOpen = false
      load()
    } catch (e) {
      ruleStatus = '✗ ' + e.message
    } finally {
      setTimeout(() => ruleStatus = '', 4000)
    }
  }

  async function testRule(rule) {
    testStatus = { ...testStatus, ['rule_' + rule.id]: 'speaking...' }
    try {
      const body = rule.preset
        ? { preset: rule.preset, camera: rule.cameras?.[0] }
        : { text: rule.text || 'Test announcement', voice: rule.voice, camera: rule.cameras?.[0] }
      const res = await fetch('/api/speak', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      testStatus = { ...testStatus, ['rule_' + rule.id]: res.ok ? '✓ Sent' : '✗ HTTP ' + res.status }
    } catch (e) {
      testStatus = { ...testStatus, ['rule_' + rule.id]: '✗ ' + e.message }
    }
    setTimeout(() => {
      const s = { ...testStatus }
      delete s['rule_' + rule.id]
      testStatus = s
    }, 5000)
  }

  function fmt(t) { return new Date(t).toLocaleTimeString() }

  const statusConfig = {
    connected:      { color: 'text-green-500',        label: 'Connected' },
    disconnected:   { color: 'text-red-400',           label: 'Disconnected' },
    not_configured: { color: 'text-muted-foreground', label: 'Not configured' },
    unknown:        { color: 'text-muted-foreground', label: '…' },
  }

  let sc = $derived(statusConfig[mqttStatus] ?? statusConfig.unknown)
  // (icon rendered inline in template to avoid deprecated svelte:component)
</script>

<!-- datalist for topic autocomplete -->
<datalist id="mqtt-topics">
  {#each topicSuggestions as t}<option value={t}></option>{/each}
</datalist>

<div class="flex flex-col gap-6 max-w-3xl">
  <!-- Header + MQTT status -->
  <div class="flex items-start justify-between gap-4 flex-wrap">
    <div>
      <h2 class="text-lg font-semibold text-primary mb-1">Frigate / MQTT</h2>
      <p class="text-sm text-muted-foreground">
        Rules trigger TTS when MQTT messages arrive matching a topic + filter.
        camspeak subscribes to topics that have active rules.
      </p>
    </div>
    <div class="flex items-center gap-2 rounded-full border px-3 py-1.5 text-sm flex-shrink-0">
      {#if mqttStatus === 'connected'}
        <Wifi class="h-4 w-4 {sc.color}" />
      {:else}
        <WifiOff class="h-4 w-4 {sc.color}" />
      {/if}
      <span class="{sc.color} font-medium">{sc.label}</span>
      {#if mqttBroker}
        <span class="text-muted-foreground font-mono text-xs">{mqttBroker}</span>
      {/if}
    </div>
  </div>

  {#if loading}
    <div class="flex items-center gap-2 text-muted-foreground text-sm">
      <Loader2 class="h-4 w-4 animate-spin" /> Loading…
    </div>
  {:else}
    {#if error}<p class="text-sm text-destructive">{error}</p>{/if}

    <!-- Live MQTT browser -->
    <div class="rounded-lg border bg-card overflow-hidden">
      <button
        class="flex w-full items-center justify-between px-4 py-3 hover:bg-muted/30 transition-colors"
        onclick={toggleBrowser}
      >
        <div class="flex items-center gap-2 text-sm font-medium">
          <Radio class="h-4 w-4 text-primary" />
          Live MQTT Browser
          {#if mqttBrowsing}
            <span class="h-2 w-2 rounded-full bg-green-500 animate-pulse"></span>
          {/if}
        </div>
        {#if mqttBrowsing}
          <span class="text-xs text-muted-foreground">click to stop</span>
        {:else}
          <span class="text-xs text-muted-foreground">click to start watching</span>
        {/if}
      </button>

      {#if mqttBrowsing}
        <div class="border-t">
          <div class="flex items-center justify-between px-4 py-2 bg-muted/20">
            <span class="text-xs text-muted-foreground">{mqttMessages.length} messages received</span>
            <button class="text-xs text-muted-foreground hover:text-foreground"
              onclick={() => mqttMessages = []}>
              <Trash2 class="h-3.5 w-3.5 inline" /> clear
            </button>
          </div>
          <div class="max-h-64 overflow-y-auto font-mono text-xs">
            {#if mqttMessages.length === 0}
              <p class="px-4 py-3 text-muted-foreground italic">Waiting for MQTT messages…</p>
            {:else}
              {#each mqttMessages as msg (msg.id)}
                <div class="flex gap-3 border-b border-border/50 px-4 py-2 hover:bg-muted/20 animate-in fade-in duration-150">
                  <span class="text-muted-foreground flex-shrink-0">{fmt(msg.at)}</span>
                  <span class="text-primary flex-shrink-0">{msg.topic}</span>
                  <span class="text-foreground/70 truncate">
                    {#if msg.payload}
                      {JSON.stringify(msg.payload)}
                    {:else}
                      {msg.raw ?? ''}
                    {/if}
                  </span>
                </div>
              {/each}
            {/if}
          </div>
        </div>
      {/if}
    </div>

    <!-- Rules list -->
    <div class="flex flex-col gap-2">
      <h3 class="text-sm font-semibold text-muted-foreground">Active Rules ({rules.length})</h3>
      {#if rules.length === 0}
        <p class="italic text-muted-foreground text-sm">No rules configured yet. Use a template below to get started.</p>
      {:else}
        {#each rules as r}
          <div class="flex items-start justify-between gap-3 rounded-lg border bg-card px-4 py-3 {!r.enabled ? 'opacity-50' : ''}">
            <div class="flex min-w-0 flex-1 flex-col gap-1">
              <code class="text-sm font-mono text-primary">{r.topic}</code>
              <div class="flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
                {#if r.cameras?.length}<span>→ {r.cameras.join(', ')}</span>{/if}
                {#if r.preset}<span>preset: <span class="text-foreground/80">{r.preset}</span></span>{/if}
                {#if r.text}<span>says: <span class="italic text-foreground/80">"{r.text}"</span></span>{/if}
                {#if r.voice}<span>voice: {r.voice}</span>{/if}
                {#if Object.keys(r.filter ?? {}).length}
                  <span>filter: <code class="bg-muted px-1 rounded">{JSON.stringify(r.filter)}</code></span>
                {/if}
                {#if !r.enabled}<span class="italic">disabled</span>{/if}
              </div>
            </div>
            <div class="flex shrink-0 items-center gap-2">
              {#if testStatus['rule_' + r.id]}
                <span class="text-xs text-primary">{testStatus['rule_' + r.id]}</span>
              {/if}
              <Button variant="outline" size="sm" class="h-7 px-2" onclick={() => testRule(r)} title="Test" aria-label="Test rule">
                <Play class="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>
        {/each}
      {/if}
    </div>

    <!-- Quick-start templates -->
    <div class="flex flex-col gap-2">
      <h3 class="text-sm font-semibold text-muted-foreground">Quick Templates</h3>
      <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
        {#each ruleTemplates as tmpl}
          <button
            class="rounded-lg border bg-card px-4 py-3 text-left hover:border-primary/50 hover:bg-muted/30 transition-colors text-sm"
            onclick={() => applyTemplate(tmpl)}
          >
            <div class="font-medium">{tmpl.label}</div>
            <div class="text-xs text-muted-foreground mt-0.5 font-mono truncate">{tmpl.topic}</div>
          </button>
        {/each}
      </div>
    </div>

    <!-- Add rule form -->
    <div>
      <Button variant="outline" size="sm" onclick={() => formOpen = !formOpen}>
        {#if formOpen}<ChevronUp class="h-4 w-4" />{:else}<ChevronDown class="h-4 w-4" />{/if}
        {formOpen ? 'Cancel' : '+ Add Rule'}
      </Button>
    </div>

    {#if formOpen}
      <div class="rounded-lg border bg-card p-5 flex flex-col gap-3">
        <h3 class="text-sm font-semibold text-primary">New Rule</h3>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            MQTT Topic
            <input
              list="mqtt-topics"
              bind:value={ruleTopic}
              placeholder="frigate/events"
              class="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm
                     placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1
                     focus-visible:ring-ring disabled:opacity-50"
            />
            <span class="text-[11px] opacity-60">Wildcards: + (one level), # (all levels)</span>
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Filter (JSON, optional)
            <Input bind:value={ruleFilter} placeholder={'{"type":"new"}'} />
            <span class="text-[11px] opacity-60">Dot-notation for nested keys: after.label</span>
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Cameras (comma-separated)
            <Input bind:value={ruleCameras} placeholder="backyard,frontyard  (blank = all)" />
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Preset (optional)
            <Input bind:value={rulePreset} placeholder="person_detected" />
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Text (if no preset)
            <Input bind:value={ruleText} placeholder="Person detected at the door" />
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Voice
            <Select bind:value={ruleVoice}>
              <option value="">default</option>
              {#each voices as v}<option>{v}</option>{/each}
            </Select>
          </label>
        </div>

        <!-- Filter quick-apply buttons -->
        <div class="flex flex-wrap gap-1.5">
          <span class="text-xs text-muted-foreground self-center">Quick filters:</span>
          {#each filterTemplates as ft}
            <button
              class="rounded-full border px-2.5 py-0.5 text-xs hover:border-primary hover:text-primary transition-colors"
              onclick={() => {
                const map = Object.fromEntries(ft.filter.split(',').map(p => {
                  const [k, v] = p.split('=')
                  return [k, v]
                }))
                ruleFilter = JSON.stringify(map)
              }}
            >{ft.label}</button>
          {/each}
        </div>

        <label class="flex items-center gap-2 text-sm text-muted-foreground">
          <input type="checkbox" bind:checked={ruleEnabled} class="h-4 w-4 rounded border-input accent-primary" />
          Enabled
        </label>
        <div class="flex items-center gap-3">
          <Button onclick={saveRule} disabled={!ruleTopic}>Save Rule</Button>
          {#if ruleStatus}<span class="text-sm {ruleStatus.startsWith('✓') ? 'text-primary' : 'text-destructive'}">{ruleStatus}</span>{/if}
        </div>
      </div>
    {/if}

    <!-- Frigate event payload reference -->
    <details class="rounded-lg border bg-card overflow-hidden">
      <summary class="cursor-pointer px-4 py-3 text-sm font-medium hover:bg-muted/30 transition-colors">
        Frigate event payload reference
      </summary>
      <div class="border-t px-4 py-3 text-xs text-muted-foreground space-y-2">
        <p>On topic <code class="bg-muted px-1 rounded font-mono">frigate/events</code>, Frigate publishes:</p>
        <pre class="bg-background border rounded p-3 overflow-x-auto text-foreground/80">{`{
  "type": "new" | "update" | "end",
  "before": { "label": "person", "camera": "backyard", "score": 0.92, ... },
  "after":  { "label": "person", "camera": "backyard", "score": 0.94,
              "entered_zones": ["driveway"], ... }
}`}</pre>
        <p>Filter keys use dot-notation to match nested fields:</p>
        <ul class="list-disc pl-4 space-y-0.5">
          <li><code class="bg-muted px-1 rounded">type</code> → <code class="bg-muted px-1 rounded">new</code>, <code class="bg-muted px-1 rounded">update</code>, <code class="bg-muted px-1 rounded">end</code></li>
          <li><code class="bg-muted px-1 rounded">after.label</code> → <code class="bg-muted px-1 rounded">person</code>, <code class="bg-muted px-1 rounded">car</code>, <code class="bg-muted px-1 rounded">dog</code>, …</li>
          <li><code class="bg-muted px-1 rounded">after.camera</code> → camera name</li>
          <li><code class="bg-muted px-1 rounded">after.stationary</code> → <code class="bg-muted px-1 rounded">false</code></li>
        </ul>
        <p>Configure Frigate's MQTT broker in your Frigate config, then set the same broker in camspeak Config → Overview (or env <code class="bg-muted px-1 rounded">CAMSPEAK_MQTT_BROKER</code>).</p>
      </div>
    </details>
  {/if}
</div>
