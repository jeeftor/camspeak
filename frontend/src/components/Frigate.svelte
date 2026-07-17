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

  // Accurate Frigate MQTT topics (from docs.frigate.video/integrations/mqtt)
  // Global topics
  const topicSuggestions = [
    // --- Global ---
    'frigate/events',            // tracked object lifecycle: new/update/end
    'frigate/reviews',           // review item lifecycle: new/update/end
    'frigate/tracked_object_update', // AI enrichment: description/face/lpr
    'frigate/available',         // "online" / "offline"
    'frigate/stats',             // stats snapshot (matches /api/stats)
    'frigate/camera_activity',   // camera feature/activity overview
    // --- Per-camera: use real camera name or + wildcard ---
    'frigate/+/motion',          // motion state: ON / OFF
    'frigate/+/person',          // person count (integer)
    'frigate/+/car',             // car count (integer)
    'frigate/+/dog',             // dog count (integer)
    'frigate/+/cat',             // cat count (integer)
    'frigate/+/person/active',   // active (non-stationary) person count
    'frigate/+/car/active',      // active car count
    'frigate/+/review_status',   // NONE / DETECTION / ALERT
    'frigate/+/status/detect',   // detection service: online/offline/disabled
    'frigate/+/status/record',   // recording service: online/offline/disabled
    'frigate/+/audio/+',         // audio detection (bark/scream/etc): ON/OFF
    'frigate/+/classification/+',// state classification (open/closed/on/off)
    // --- Catch-all for browsing ---
    'frigate/#',
  ]

  // Filter templates for frigate/events — uses dot-notation for nested fields
  // Frigate event payload: { type, before: {...}, after: { label, camera, score,
  //   stationary, entered_zones, current_zones, ... } }
  const filterTemplates = [
    { label: 'New event',          filter: { type: 'new' } },
    { label: 'New — person',       filter: { type: 'new', 'after.label': 'person' } },
    { label: 'New — car',          filter: { type: 'new', 'after.label': 'car' } },
    { label: 'New — dog',          filter: { type: 'new', 'after.label': 'dog' } },
    { label: 'New — cat',          filter: { type: 'new', 'after.label': 'cat' } },
    { label: 'Moving (not stationary)', filter: { type: 'new', 'after.stationary': 'false' } },
    { label: 'Event ended',        filter: { type: 'end' } },
    { label: 'Alert review',       filter: { type: 'new', severity: 'alert' } },
  ]

  // Rule templates — one-click to populate the form
  const ruleTemplates = [
    {
      label: 'Person detected',
      topic: 'frigate/events',
      filter: '{"type":"new","after.label":"person"}',
      cameras: '',
      text: 'Person detected',
    },
    {
      label: 'Car detected',
      topic: 'frigate/events',
      filter: '{"type":"new","after.label":"car"}',
      cameras: '',
      text: 'Vehicle detected',
    },
    {
      label: 'Dog detected',
      topic: 'frigate/events',
      filter: '{"type":"new","after.label":"dog"}',
      cameras: '',
      text: 'Dog detected',
    },
    {
      label: 'Motion on any camera',
      topic: 'frigate/+/motion',
      filter: '',
      cameras: '',
      text: 'Motion detected',
    },
    {
      label: 'Alert-level review',
      topic: 'frigate/reviews',
      filter: '{"type":"new","after.severity":"alert"}',
      cameras: '',
      text: 'Security alert',
    },
    {
      label: 'Any new detection',
      topic: 'frigate/events',
      filter: '{"type":"new"}',
      cameras: '',
      text: 'Object detected',
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
              onclick={() => { ruleFilter = JSON.stringify(ft.filter) }}
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

    <!-- Frigate MQTT reference (source: docs.frigate.video/integrations/mqtt) -->
    <details class="rounded-lg border bg-card overflow-hidden">
      <summary class="cursor-pointer px-4 py-3 text-sm font-medium hover:bg-muted/30 transition-colors">
        Frigate MQTT topic & payload reference
      </summary>
      <div class="border-t px-4 py-3 text-xs text-muted-foreground flex flex-col gap-4">

        <!-- frigate/events -->
        <div>
          <p class="font-semibold text-foreground mb-1">frigate/events — object lifecycle</p>
          <p class="mb-1.5">Published on every tracked object create/update/end. Most useful for TTS rules.</p>
          <pre class="bg-background border rounded p-3 overflow-x-auto text-foreground/80">{`{
  "type": "new" | "update" | "end",
  "before": {
    "label": "person",       // object class
    "camera": "backyard",
    "score": 0.87,
    "stationary": false,
    "current_zones": [],
    "entered_zones": []
  },
  "after": {                 // updated state (same fields as before)
    "label": "person",
    "camera": "backyard",
    "score": 0.94,
    "stationary": false,
    "current_zones": ["driveway"],
    "entered_zones": ["driveway"]
  }
}`}</pre>
          <p class="mt-1.5 font-medium text-foreground">Filter keys (dot-notation into above):</p>
          <div class="grid grid-cols-2 gap-x-4 gap-y-0.5 mt-1">
            <span><code class="bg-muted px-1 rounded">type</code> — new · update · end</span>
            <span><code class="bg-muted px-1 rounded">after.label</code> — person · car · dog · cat · bird…</span>
            <span><code class="bg-muted px-1 rounded">after.camera</code> — camera name string</span>
            <span><code class="bg-muted px-1 rounded">after.stationary</code> — true · false</span>
          </div>
        </div>

        <!-- frigate/reviews -->
        <div>
          <p class="font-semibold text-foreground mb-1">frigate/reviews — review items</p>
          <pre class="bg-background border rounded p-3 overflow-x-auto text-foreground/80">{`{
  "type": "new" | "update" | "end",
  "after": {
    "severity": "alert" | "detection",
    "camera": "backyard",
    "data": { "objects": ["person"], "zones": ["driveway"], ... }
  }
}`}</pre>
          <p class="mt-1">Filter: <code class="bg-muted px-1 rounded">{`{"type":"new","after.severity":"alert"}`}</code></p>
        </div>

        <!-- Per-camera count topics -->
        <div>
          <p class="font-semibold text-foreground mb-1">frigate/&lt;camera&gt;/&lt;label&gt; — object count</p>
          <p>Payload is a plain integer (e.g. <code class="bg-muted px-1 rounded">2</code>). No filter needed — fires every time the count changes.</p>
          <p class="mt-0.5">Examples: <code class="bg-muted px-1 rounded">frigate/backyard/person</code> · <code class="bg-muted px-1 rounded">frigate/frontyard/car</code></p>
          <p class="mt-0.5">Add <code class="bg-muted px-1 rounded">/active</code> for non-stationary count only.</p>
        </div>

        <!-- Motion -->
        <div>
          <p class="font-semibold text-foreground mb-1">frigate/&lt;camera&gt;/motion — motion state</p>
          <p>Payload: <code class="bg-muted px-1 rounded">ON</code> or <code class="bg-muted px-1 rounded">OFF</code>. Includes the configured <code class="bg-muted px-1 rounded">mqtt_off_delay</code> buffer.</p>
        </div>

        <!-- Audio -->
        <div>
          <p class="font-semibold text-foreground mb-1">frigate/&lt;camera&gt;/audio/&lt;type&gt; — audio detection</p>
          <p>Types: bark · scream · speech · yell · glass_breaking · etc. Payload: <code class="bg-muted px-1 rounded">ON</code> / <code class="bg-muted px-1 rounded">OFF</code>.</p>
          <p class="mt-0.5">Example topic: <code class="bg-muted px-1 rounded">frigate/backyard/audio/bark</code></p>
        </div>

        <!-- Setup note -->
        <div class="rounded-md bg-muted/40 border px-3 py-2">
          <p class="font-medium text-foreground mb-0.5">Setup</p>
          <p>Set the same MQTT broker in Frigate (<code class="bg-muted px-1 rounded">mqtt.host</code>) and camspeak
            (<code class="bg-muted px-1 rounded">CAMSPEAK_MQTT_BROKER=tcp://192.168.1.x:1883</code>).
            camspeak only subscribes to topics that have active rules — use the Live Browser above to verify messages are arriving.
          </p>
        </div>
      </div>
    </details>
  {/if}
</div>
