<script>
  import { onMount } from 'svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { Select } from '$lib/components/ui/select'
  import { Badge } from '$lib/components/ui/badge'

  let { onRefresh } = $props()

  let tab = $state('tts')
  let config = $state(null)
  let ttsPresets = $state([])
  let activeTTS = $state('')
  let cameras = $state([])
  let rules = $state([])
  let voices = $state([])
  let loading = $state(true)

  // TTS form
  let ttsName = $state('')
  let ttsEndpoint = $state('')
  let ttsModel = $state('')
  let ttsVoice = $state('')
  let ttsKey = $state('')
  let ttsDesc = $state('')
  let ttsStatus = $state('')

  // Camera form
  let camName = $state('')
  let camType = $state('hikvision')
  let camIP = $state('')
  let camUser = $state('')
  let camPass = $state('')
  let camChannel = $state(1)
  let camStatus = $state('')

  // Rule form
  let ruleTopic = $state('frigate/events')
  let ruleFilter = $state('')  // JSON string
  let ruleCameras = $state('')
  let rulePreset = $state('')
  let ruleText = $state('')
  let ruleVoice = $state('')
  let ruleEnabled = $state(true)
  let ruleStatus = $state('')

  // Test status
  let testStatus = $state({})

  async function loadConfig() {
    loading = true
    try {
      const [cfgRes, ttsRes, camRes, rulesRes, voiceRes] = await Promise.all([
        fetch('/api/config'),
        fetch('/api/config/tts'),
        fetch('/api/config/cameras'),
        fetch('/api/config/rules'),
        fetch('/api/voices'),
      ])
      config = await cfgRes.json()
      const ttsData = await ttsRes.json()
      ttsPresets = ttsData.presets ?? []
      activeTTS = ttsData.active?.url ?? ''
      cameras = await camRes.json() ?? []
      rules = await rulesRes.json() ?? []
      voices = await voiceRes.json() ?? []
    } catch (e) {
      console.error('loadConfig error:', e)
    } finally {
      loading = false
    }
  }

  onMount(loadConfig)

  // --- TTS Presets ---
  async function saveTTS() {
    if (!ttsName || !ttsEndpoint) return
    ttsStatus = ''
    try {
      const res = await fetch('/api/config/tts', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: ttsName, endpoint: ttsEndpoint, model: ttsModel,
          default_voice: ttsVoice, api_key: ttsKey, description: ttsDesc,
        }),
      })
      if (!res.ok) throw new Error(await res.text())
      ttsStatus = '✓ Saved'
      ttsName = ''; ttsEndpoint = ''; ttsModel = ''; ttsVoice = ''; ttsKey = ''; ttsDesc = ''
      loadConfig()
    } catch (e) {
      ttsStatus = '✗ ' + e.message
    } finally {
      setTimeout(() => ttsStatus = '', 4000)
    }
  }

  async function activateTTS(name) {
    await fetch(`/api/config/tts/${name}/activate`, { method: 'POST' })
    loadConfig()
  }

  async function deleteTTS(name) {
    if (!confirm(`Delete TTS preset "${name}"?`)) return
    await fetch(`/api/config/tts/${name}`, { method: 'DELETE' })
    loadConfig()
  }

  async function testTTS() {
    testStatus = { ...testStatus, tts: 'testing...' }
    try {
      const res = await fetch('/api/voices')
      if (res.ok) {
        const v = await res.json()
        testStatus = { ...testStatus, tts: `✓ Connected (${v?.length ?? 0} voices)` }
      } else {
        testStatus = { ...testStatus, tts: '✗ HTTP ' + res.status }
      }
    } catch (e) {
      testStatus = { ...testStatus, tts: '✗ ' + e.message }
    }
  }

  // --- Cameras ---
  async function saveCamera() {
    if (!camName || !camIP) return
    camStatus = ''
    try {
      const res = await fetch('/api/config/cameras', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: camName, type: camType, ip: camIP,
          user: camUser, pass: camPass, channel: parseInt(camChannel) || 1,
        }),
      })
      if (!res.ok) throw new Error(await res.text())
      camStatus = '✓ Saved'
      camName = ''; camIP = ''; camUser = ''; camPass = ''; camChannel = 1
      loadConfig()
      onRefresh?.()
    } catch (e) {
      camStatus = '✗ ' + e.message
    } finally {
      setTimeout(() => camStatus = '', 4000)
    }
  }

  async function deleteCamera(name) {
    if (!confirm(`Delete camera "${name}"?`)) return
    await fetch(`/api/config/cameras/${name}`, { method: 'DELETE' })
    loadConfig()
    onRefresh?.()
  }

  async function testCamera(name) {
    testStatus = { ...testStatus, [name]: 'testing...' }
    try {
      const res = await fetch('/api/beep', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ camera: name }),
      })
      testStatus = { ...testStatus, [name]: res.ok ? '✓ Beep sent' : '✗ HTTP ' + res.status }
    } catch (e) {
      testStatus = { ...testStatus, [name]: '✗ ' + e.message }
    }
    setTimeout(() => {
      const s = { ...testStatus }
      delete s[name]
      testStatus = s
    }, 5000)
  }

  // --- Rules ---
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
      loadConfig()
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

  function editCamera(cam) {
    camName = cam.name
    camType = cam.type
    camIP = cam.ip
    camChannel = cam.channel || 1
  }

  function editTTS(p) {
    ttsName = p.name
    ttsEndpoint = p.endpoint
    ttsModel = p.model
    ttsVoice = p.default_voice
    ttsDesc = p.description
  }

  const configTabs = [
    { id: 'tts', label: 'TTS Presets' },
    { id: 'cameras', label: 'Cameras' },
    { id: 'rules', label: 'MQTT Rules' },
    { id: 'overview', label: 'Overview' },
  ]
</script>

{#if loading}
  <p class="italic text-muted-foreground">Loading config…</p>
{:else}
  <div class="flex flex-col gap-4">
    <div class="flex gap-1">
      {#each configTabs as t}
        <Button
          variant={tab === t.id ? 'default' : 'outline'}
          size="sm"
          onclick={() => tab = t.id}
        >
          {t.label}
        </Button>
      {/each}
    </div>

    <!-- TTS Presets -->
    {#if tab === 'tts'}
      <section class="rounded-lg border bg-card p-5">
        <div class="mb-3 flex items-center justify-between">
          <h3 class="text-base font-semibold text-primary">TTS Presets</h3>
          <Button variant="outline" size="sm" onclick={testTTS}>Test Connection</Button>
        </div>
        {#if testStatus.tts}<span class="mr-2 text-sm text-primary">{testStatus.tts}</span>{/if}

        <div class="mb-4 flex flex-col gap-1.5">
          {#each ttsPresets as p}
            <div class="flex items-center justify-between rounded-lg border bg-background px-3 py-2 {p.is_active ? 'border-primary bg-primary/5' : ''}">
              <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
                <span class="font-semibold">{p.name}</span>
                {#if p.is_active}<Badge>ACTIVE</Badge>{/if}
                <span class="text-sm text-muted-foreground">{p.model}</span>
                <span class="text-sm text-muted-foreground">{p.default_voice}</span>
              </div>
              <div class="flex shrink-0 gap-1">
                <Button variant="outline" size="sm" class="h-7 px-2" onclick={() => editTTS(p)} title="Edit">✎</Button>
                {#if !p.is_active}
                  <Button variant="outline" size="sm" class="h-7 px-2" onclick={() => activateTTS(p.name)} title="Activate">●</Button>
                {/if}
                <Button variant="outline" size="sm" class="h-7 px-2 hover:border-destructive hover:text-destructive" onclick={() => deleteTTS(p.name)} title="Delete">✕</Button>
              </div>
            </div>
          {/each}
          {#if ttsPresets.length === 0}
            <p class="italic text-muted-foreground">No TTS presets configured.</p>
          {/if}
        </div>

        <details class="border-t pt-3">
          <summary class="cursor-pointer py-1.5 text-sm text-primary hover:text-primary/80">{ttsName ? 'Edit' : 'Add'} TTS Preset</summary>
          <div class="mt-3 grid grid-cols-2 gap-2.5 max-sm:grid-cols-1">
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Name
              <Input bind:value={ttsName} placeholder="lemonade-local" />
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Endpoint
              <Input bind:value={ttsEndpoint} placeholder="http://192.168.1.91:13305/v1/audio/speech" />
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Model
              <Input bind:value={ttsModel} placeholder="kokoro" />
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Default Voice
              <Select bind:value={ttsVoice}>
                <option value="">default</option>
                {#each voices as v}<option>{v}</option>{/each}
              </Select>
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              API Key (optional)
              <Input bind:value={ttsKey} type="password" placeholder="sk-..." />
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Description
              <Input bind:value={ttsDesc} placeholder="Local Lemonade instance" />
            </label>
          </div>
          <Button onclick={saveTTS} disabled={!ttsName || !ttsEndpoint} class="mt-3">
            Save Preset
          </Button>
          {#if ttsStatus}<span class="ml-2 text-sm text-primary">{ttsStatus}</span>{/if}
        </details>
      </section>

    <!-- Cameras -->
    {:else if tab === 'cameras'}
      <section class="rounded-lg border bg-card p-5">
        <h3 class="mb-3 text-base font-semibold text-primary">Cameras</h3>
        <div class="mb-4 flex flex-col gap-1.5">
          {#each cameras as cam}
            <div class="flex items-center justify-between rounded-lg border bg-background px-3 py-2">
              <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
                <span class="font-semibold">{cam.name}</span>
                <span class="text-sm text-muted-foreground">{cam.type}</span>
                <span class="text-sm text-muted-foreground">{cam.ip}</span>
                <span class="text-sm text-muted-foreground">ch{cam.channel}</span>
              </div>
              <div class="flex shrink-0 items-center gap-1">
                {#if testStatus[cam.name]}<span class="mr-1 text-sm text-primary">{testStatus[cam.name]}</span>{/if}
                <Button variant="outline" size="sm" class="h-7 px-2" onclick={() => testCamera(cam.name)} title="Test beep">🔔</Button>
                <Button variant="outline" size="sm" class="h-7 px-2" onclick={() => editCamera(cam)} title="Edit">✎</Button>
                <Button variant="outline" size="sm" class="h-7 px-2 hover:border-destructive hover:text-destructive" onclick={() => deleteCamera(cam.name)} title="Delete">✕</Button>
              </div>
            </div>
          {/each}
          {#if cameras.length === 0}
            <p class="italic text-muted-foreground">No cameras configured. Run <code class="not-italic text-muted-foreground/80">camspeak discover --frigate &lt;url&gt;</code> or add one below.</p>
          {/if}
        </div>

        <details class="border-t pt-3">
          <summary class="cursor-pointer py-1.5 text-sm text-primary hover:text-primary/80">{camName ? 'Edit' : 'Add'} Camera</summary>
          <div class="mt-3 grid grid-cols-2 gap-2.5 max-sm:grid-cols-1">
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Name
              <Input bind:value={camName} placeholder="backyard" />
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Type
              <Select bind:value={camType}>
                <option value="hikvision">hikvision</option>
                <option value="reolink">reolink</option>
              </Select>
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              IP
              <Input bind:value={camIP} placeholder="192.168.1.181" />
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Username
              <Input bind:value={camUser} placeholder="Operator" />
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Password
              <Input bind:value={camPass} type="password" placeholder="password" />
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Channel
              <Input bind:value={camChannel} type="number" min="1" />
            </label>
          </div>
          <Button onclick={saveCamera} disabled={!camName || !camIP} class="mt-3">
            Save Camera
          </Button>
          {#if camStatus}<span class="ml-2 text-sm text-primary">{camStatus}</span>{/if}
        </details>
      </section>

    <!-- MQTT Rules -->
    {:else if tab === 'rules'}
      <section class="rounded-lg border bg-card p-5">
        <h3 class="mb-1 text-base font-semibold text-primary">MQTT Rules</h3>
        <p class="mb-3 text-sm text-muted-foreground">Rules trigger TTS announcements when MQTT messages match the topic + filter.</p>
        <div class="mb-4 flex flex-col gap-1.5">
          {#each rules as r}
            <div class="flex items-center justify-between rounded-lg border bg-background px-3 py-2 {!r.enabled ? 'opacity-50' : ''}">
              <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
                <span class="font-mono text-sm text-primary">{r.topic}</span>
                {#if r.preset}<span class="text-xs text-muted-foreground">preset: {r.preset}</span>{/if}
                {#if r.text}<span class="text-xs italic text-muted-foreground">"{r.text}"</span>{/if}
                {#if r.cameras?.length}<span class="text-xs text-muted-foreground">→ {r.cameras.join(', ')}</span>{/if}
                {#if r.voice}<span class="text-xs text-muted-foreground">voice: {r.voice}</span>{/if}
                {#if Object.keys(r.filter ?? {}).length}
                  <span class="text-xs text-muted-foreground">filter: {JSON.stringify(r.filter)}</span>
                {/if}
              </div>
              <div class="flex shrink-0 items-center gap-1">
                {#if testStatus['rule_' + r.id]}<span class="mr-1 text-sm text-primary">{testStatus['rule_' + r.id]}</span>{/if}
                <Button variant="outline" size="sm" class="h-7 px-2" onclick={() => testRule(r)} title="Test speak">▶</Button>
              </div>
            </div>
          {/each}
          {#if rules.length === 0}
            <p class="italic text-muted-foreground">No rules configured.</p>
          {/if}
        </div>

        <details class="border-t pt-3">
          <summary class="cursor-pointer py-1.5 text-sm text-primary hover:text-primary/80">Add Rule</summary>
          <div class="mt-3 grid grid-cols-2 gap-2.5 max-sm:grid-cols-1">
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              MQTT Topic
              <Input bind:value={ruleTopic} placeholder="frigate/events" />
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Filter (JSON)
              <Input bind:value={ruleFilter} placeholder={'{"type":"person"}'} />
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Cameras (comma-sep)
              <Input bind:value={ruleCameras} placeholder="backyard,frontyard" />
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Preset (optional)
              <Input bind:value={rulePreset} placeholder="person_detected" />
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Text (if no preset)
              <Input bind:value={ruleText} placeholder="Person detected" />
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Voice
              <Select bind:value={ruleVoice}>
                <option value="">default</option>
                {#each voices as v}<option>{v}</option>{/each}
              </Select>
            </label>
            <label class="flex flex-row items-center gap-2 text-xs text-muted-foreground">
              <input type="checkbox" bind:checked={ruleEnabled} class="h-4 w-4 rounded border-input" /> Enabled
            </label>
          </div>
          <Button onclick={saveRule} disabled={!ruleTopic} class="mt-3">
            Save Rule
          </Button>
          {#if ruleStatus}<span class="ml-2 text-sm text-primary">{ruleStatus}</span>{/if}
        </details>
      </section>

    <!-- Overview -->
    {:else if tab === 'overview'}
      <section class="rounded-lg border bg-card p-5">
        <h3 class="mb-3 text-base font-semibold text-primary">Runtime Configuration</h3>
        <pre class="max-h-[600px] overflow-auto rounded-lg border bg-background p-4 text-sm text-foreground/80">{JSON.stringify(config, null, 2)}</pre>
      </section>
    {/if}
  </div>
{/if}
