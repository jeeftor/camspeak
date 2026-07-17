<script>
  import { onMount, onDestroy } from 'svelte'
  import { Play, Loader2, Wifi, WifiOff, Radio, ChevronDown, ChevronUp, Trash2, ChevronRight } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { Select } from '$lib/components/ui/select'
  import JsonCode from '$lib/components/JsonCode.svelte'

  let rules = $state([])
  let voices = $state([])
  let loading = $state(true)
  let testStatus = $state({})
  let error = $state('')

  // MQTT status
  let mqttStatus = $state('unknown')
  let mqttBroker = $state('')

  // Live MQTT browser
  let mqttMessages = $state([])
  let mqttBrowsing = $state(false)
  let es = null

  // Topic tree state
  // topicMap: { [fullTopic]: { count, lastPayload, lastAt } }
  let topicMap = $state({})
  // expandedNodes: Set of topic-path prefixes that are expanded (e.g. "frigate", "frigate/backyard")
  let expandedNodes = $state(new Set(['frigate']))
  // selectedTopic: filter message feed to this topic (null = all)
  let selectedTopic = $state(null)

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
  const topicSuggestions = [
    'frigate/events',
    'frigate/reviews',
    'frigate/tracked_object_update',
    'frigate/available',
    'frigate/stats',
    'frigate/camera_activity',
    'frigate/+/motion',
    'frigate/+/person',
    'frigate/+/car',
    'frigate/+/dog',
    'frigate/+/cat',
    'frigate/+/person/active',
    'frigate/+/car/active',
    'frigate/+/review_status',
    'frigate/+/status/detect',
    'frigate/+/status/record',
    'frigate/+/audio/+',
    'frigate/+/classification/+',
    'frigate/#',
  ]

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

  // --- Topic tree builder (ported from mqtt-viewer/build-tree.ts) ---
  // Builds a nested tree from the flat topicMap keyed by full topic strings.
  // Returns a flat array of rows with indentation levels for rendering.

  /**
   * @typedef {{ level: number, segment: string, fullPath: string, count: number, lastPayload: string, hasChildren: boolean, isExpanded: boolean }} TreeRow
   */

  function buildTopicTree(topicMap, expandedNodes) {
    // Build nested structure: nested[segment] = { children: {}, count, fullPath, lastPayload }
    const nested = {}
    for (const [topic, info] of Object.entries(topicMap)) {
      const parts = topic.split('/')
      let node = nested
      let path = ''
      for (let i = 0; i < parts.length; i++) {
        const seg = parts[i]
        path = path ? path + '/' + seg : seg
        if (!node[seg]) {
          node[seg] = { children: {}, count: 0, fullPath: path, lastPayload: '' }
        }
        if (i === parts.length - 1) {
          node[seg].count = info.count
          node[seg].lastPayload = info.lastPayload
        }
        node = node[seg].children
      }
    }

    // Flatten to rows with level
    const rows = []
    function walk(node, level) {
      const keys = Object.keys(node).sort()
      for (const seg of keys) {
        const n = node[seg]
        const hasChildren = Object.keys(n.children).length > 0
        const isExpanded = expandedNodes.has(n.fullPath)
        rows.push({
          level,
          segment: seg,
          fullPath: n.fullPath,
          count: n.count,
          lastPayload: n.lastPayload,
          hasChildren,
          isExpanded,
        })
        if (isExpanded && hasChildren) {
          walk(n.children, level + 1)
        }
      }
    }
    walk(nested, 0)
    return rows
  }

  let treeRows = $derived(buildTopicTree(topicMap, expandedNodes))

  // Combined datalist: hardcoded + seen
  let allTopicSuggestions = $derived([
    ...topicSuggestions,
    ...Object.keys(topicMap).filter(t => !topicSuggestions.includes(t)),
  ])

  // Filtered messages
  let filteredMessages = $derived(
    selectedTopic
      ? mqttMessages.filter(m => m.topic === selectedTopic)
      : mqttMessages
  )

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

  async function startBrowsing() {
    mqttMessages = []
    topicMap = {}

    // Open SSE stream
    es = new EventSource('/api/mqtt/events')
    es.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data)
        const payload = msg.payload ? JSON.stringify(msg.payload) : (msg.raw ?? '')
        // Update topicMap
        topicMap = {
          ...topicMap,
          [msg.topic]: {
            count: (topicMap[msg.topic]?.count ?? 0) + 1,
            lastPayload: payload,
            lastAt: msg.at,
          }
        }
        mqttMessages = [{ ...msg, id: Date.now() + Math.random() }, ...mqttMessages].slice(0, 200)
      } catch {}
    }
    es.onerror = () => {}
    mqttBrowsing = true

    // Auto-subscribe to frigate/# for discovery (best-effort)
    if (mqttStatus === 'connected') {
      try {
        await fetch('/api/mqtt/subscribe', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ topic: 'frigate/#' }),
        })
      } catch {}
    }

    // Also load any topics already accumulated server-side
    try {
      const res = await fetch('/api/mqtt/topics')
      const seenTopics = await res.json()
      if (Array.isArray(seenTopics)) {
        const newMap = { ...topicMap }
        for (const st of seenTopics) {
          if (!newMap[st.topic]) {
            newMap[st.topic] = {
              count: st.count,
              lastPayload: st.payload ? JSON.stringify(st.payload) : (st.raw ?? ''),
              lastAt: st.at,
            }
          }
        }
        topicMap = newMap
      }
    } catch {}
  }

  function stopBrowsing() {
    es?.close(); es = null
    mqttBrowsing = false
  }

  function toggleBrowser() {
    if (mqttBrowsing) {
      stopBrowsing()
    } else {
      startBrowsing()
    }
  }

  function toggleNode(fullPath) {
    const next = new Set(expandedNodes)
    if (next.has(fullPath)) {
      next.delete(fullPath)
    } else {
      next.add(fullPath)
    }
    expandedNodes = next
  }

  function selectTopic(fullPath) {
    selectedTopic = selectedTopic === fullPath ? null : fullPath
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

  function fmtPayload(msg) {
    if (msg.payload) {
      try { return JSON.stringify(msg.payload, null, 0) }
      catch {}
    }
    return msg.raw ?? ''
  }
</script>

<!-- datalist for topic autocomplete -->
<datalist id="mqtt-topics">
  {#each allTopicSuggestions as t}<option value={t}></option>{/each}
</datalist>

<div class="flex flex-col gap-6 max-w-4xl">
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
      <!-- Header bar -->
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
          <span class="text-xs text-muted-foreground">click to start — auto-subscribes to frigate/#</span>
        {/if}
      </button>

      {#if mqttBrowsing}
        <div class="border-t">
          <!-- Toolbar -->
          <div class="flex items-center justify-between px-4 py-2 bg-muted/20 gap-3 flex-wrap">
            <div class="flex items-center gap-3 text-xs text-muted-foreground">
              <span>{mqttMessages.length} messages</span>
              <span>·</span>
              <span>{Object.keys(topicMap).length} topics seen</span>
              {#if selectedTopic}
                <span>·</span>
                <span>
                  filtering: <code class="bg-muted px-1 rounded text-foreground">{selectedTopic}</code>
                  <button class="ml-1 hover:text-foreground" onclick={() => selectedTopic = null}>×</button>
                </span>
              {/if}
            </div>
            <button class="text-xs text-muted-foreground hover:text-foreground flex items-center gap-1"
              onclick={() => { mqttMessages = []; topicMap = {}; selectedTopic = null }}>
              <Trash2 class="h-3.5 w-3.5 inline" /> clear
            </button>
          </div>

          <!-- Split layout: tree left, feed right -->
          <!-- On mobile: stacked (flex-col). On sm+: side by side (flex-row) -->
          <div class="flex flex-col sm:flex-row" style="min-height: 280px; max-height: 420px;">

            <!-- Topic tree pane -->
            <div class="sm:w-56 sm:min-w-56 border-b sm:border-b-0 sm:border-r overflow-y-auto bg-background/50">
              <div class="px-2 py-1.5 text-[10px] font-semibold text-muted-foreground uppercase tracking-wide border-b">
                Topics
              </div>
              {#if treeRows.length === 0}
                <p class="px-3 py-3 text-xs text-muted-foreground italic">Waiting for messages…</p>
              {:else}
                <div class="py-1">
                  {#each treeRows as row (row.fullPath)}
                    <div
                      class="flex items-center gap-0.5 px-1 py-0.5 cursor-pointer select-none
                             hover:bg-muted/40 transition-colors
                             {selectedTopic === row.fullPath ? 'bg-primary/10 text-primary' : ''}"
                      style="padding-left: {4 + row.level * 14}px"
                    >
                      <!-- Expand toggle -->
                      <button
                        class="w-4 h-4 flex items-center justify-center flex-shrink-0 text-muted-foreground hover:text-foreground"
                        onclick={(e) => { e.stopPropagation(); row.hasChildren && toggleNode(row.fullPath) }}
                      >
                        {#if row.hasChildren}
                          <ChevronRight class="h-3 w-3 {row.isExpanded ? 'rotate-90' : ''} transition-transform" />
                        {/if}
                      </button>
                      <!-- Row content -->
                      <button
                        class="flex-1 flex items-center gap-1 min-w-0 text-left"
                        onclick={() => !row.hasChildren && selectTopic(row.fullPath)}
                        title={row.fullPath}
                      >
                        <span class="font-mono text-xs truncate {row.hasChildren ? 'font-medium' : ''}">{row.segment}</span>
                        {#if row.count > 0}
                          <span class="text-[10px] text-muted-foreground flex-shrink-0">({row.count})</span>
                        {/if}
                      </button>
                    </div>
                    {#if !row.hasChildren && row.lastPayload}
                      <!-- Last value preview — subtle, truncated -->
                      <button
                        class="w-full text-left px-2 py-0.5 text-[10px] font-mono text-muted-foreground truncate hover:text-foreground"
                        style="padding-left: {22 + row.level * 14}px"
                        onclick={() => selectTopic(row.fullPath)}
                        title={row.lastPayload}
                      >
                        {row.lastPayload}
                      </button>
                    {/if}
                  {/each}
                </div>
              {/if}
            </div>

            <!-- Message feed pane -->
            <div class="flex-1 overflow-y-auto font-mono text-xs">
              {#if filteredMessages.length === 0}
                <p class="px-4 py-3 text-muted-foreground italic">
                  {selectedTopic ? `No messages on ${selectedTopic} yet` : 'Waiting for MQTT messages…'}
                </p>
              {:else}
                {#each filteredMessages as msg (msg.id)}
                  <div class="flex gap-3 border-b border-border/50 px-4 py-2 hover:bg-muted/20 animate-in fade-in duration-150">
                    <span class="text-muted-foreground flex-shrink-0 w-16">{fmt(msg.at)}</span>
                    <span class="text-primary flex-shrink-0 truncate max-w-32" title={msg.topic}>{msg.topic}</span>
                    <span class="text-foreground/70 truncate">
                      {fmtPayload(msg)}
                    </span>
                  </div>
                {/each}
              {/if}
            </div>
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

        <div>
          <p class="font-semibold text-foreground mb-1">frigate/events — object lifecycle</p>
          <p class="mb-1.5">Published on every tracked object create/update/end. Most useful for TTS rules.</p>
          <JsonCode code={`{
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
}`} />
          <p class="mt-1.5 font-medium text-foreground">Filter keys (dot-notation into above):</p>
          <div class="grid grid-cols-2 gap-x-4 gap-y-0.5 mt-1">
            <span><code class="bg-muted px-1 rounded">type</code> — new · update · end</span>
            <span><code class="bg-muted px-1 rounded">after.label</code> — person · car · dog · cat · bird…</span>
            <span><code class="bg-muted px-1 rounded">after.camera</code> — camera name string</span>
            <span><code class="bg-muted px-1 rounded">after.stationary</code> — true · false</span>
          </div>
        </div>

        <div>
          <p class="font-semibold text-foreground mb-1">frigate/reviews — review items</p>
          <JsonCode code={`{
  "type": "new" | "update" | "end",
  "after": {
    "severity": "alert" | "detection",
    "camera": "backyard",
    "data": { "objects": ["person"], "zones": ["driveway"], ... }
  }
}`} />
          <p class="mt-1">Filter: <code class="bg-muted px-1 rounded">{`{"type":"new","after.severity":"alert"}`}</code></p>
        </div>

        <div>
          <p class="font-semibold text-foreground mb-1">frigate/&lt;camera&gt;/&lt;label&gt; — object count</p>
          <p>Payload is a plain integer (e.g. <code class="bg-muted px-1 rounded">2</code>). No filter needed — fires every time the count changes.</p>
          <p class="mt-0.5">Examples: <code class="bg-muted px-1 rounded">frigate/backyard/person</code> · <code class="bg-muted px-1 rounded">frigate/frontyard/car</code></p>
          <p class="mt-0.5">Add <code class="bg-muted px-1 rounded">/active</code> for non-stationary count only.</p>
        </div>

        <div>
          <p class="font-semibold text-foreground mb-1">frigate/&lt;camera&gt;/motion — motion state</p>
          <p>Payload: <code class="bg-muted px-1 rounded">ON</code> or <code class="bg-muted px-1 rounded">OFF</code>. Includes the configured <code class="bg-muted px-1 rounded">mqtt_off_delay</code> buffer.</p>
        </div>

        <div>
          <p class="font-semibold text-foreground mb-1">frigate/&lt;camera&gt;/audio/&lt;type&gt; — audio detection</p>
          <p>Types: bark · scream · speech · yell · glass_breaking · etc. Payload: <code class="bg-muted px-1 rounded">ON</code> / <code class="bg-muted px-1 rounded">OFF</code>.</p>
          <p class="mt-0.5">Example topic: <code class="bg-muted px-1 rounded">frigate/backyard/audio/bark</code></p>
        </div>

        <div class="rounded-md bg-muted/40 border px-3 py-2">
          <p class="font-medium text-foreground mb-0.5">Setup</p>
          <p>Set the same MQTT broker in Frigate (<code class="bg-muted px-1 rounded">mqtt.host</code>) and camspeak
            (<code class="bg-muted px-1 rounded">CAMSPEAK_MQTT_BROKER=tcp://192.168.1.x:1883</code>).
            Opening the Live Browser automatically subscribes to <code class="bg-muted px-1 rounded">frigate/#</code> for full topic discovery.
          </p>
        </div>
      </div>
    </details>
  {/if}
</div>
