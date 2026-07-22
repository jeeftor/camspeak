<script>
  import { onMount } from 'svelte'
  import { Bell, Pencil, X, Check, Loader2 } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { Select } from '$lib/components/ui/select'
  import { Badge } from '$lib/components/ui/badge'
  import JsonCode from '$lib/components/JsonCode.svelte'
  import Modal from '$lib/components/Modal.svelte'

  let { onRefresh } = $props()

  let tab = $state('settings')
  let config = $state(null)
  let ttsPresets = $state([])
  let activeTTS = $state('')
  let cameras = $state([])
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

  // Camera modal
  let camFormOpen = $state(false)
  let testCamStatus = $state('')
  let testCamBusy = $state(false)

  // TTS modal
  let ttsFormOpen = $state(false)
  let camName = $state('')
  let camType = $state('hikvision')
  let camIP = $state('')
  let camUser = $state('')
  let camPass = $state('')
  let camChannel = $state(1)
  let camStream = $state('')
  let camEnabled = $state(false)
  let camAirPlayName = $state('')
  let camVisionPrompt = $state('')
  let camStatus = $state('')

  // Vision form
  let visionURL = $state('')
  let visionModel = $state('')
  let visionAPIKey = $state('')
  let visionPrompt = $state('')
  let visionStatus = $state('')
  let visionTestStatus = $state('')
  let visionTestBusy = $state(false)

  // Camera pending-enabled batch state (checkbox → local, Save commits)
  let pendingEnabled = $state({}) // name → bool
  let camerasDirty = $derived(Object.keys(pendingEnabled).length > 0)

  // General settings
  let frigateURL = $state('')
  let go2rtcURL = $state('')
  let advertiseIP = $state('')
  let settingsStatus = $state('')
  let frigateTestStatus = $state('')
  let frigateTestBusy = $state(false)
  let go2rtcTestStatus = $state('')
  let go2rtcTestBusy = $state(false)

  // AirPlay form
  let airplayEnabled = $state(false)
  let airplayBasePort = $state(5000)
  let airplayPrimeSilenceMs = $state(500)
  let airplayStatus = $state('')
  let airplayCameras = $state([])   // [{name, airplay_enabled, airplay_running}]
  let airplayToggling = $state({})  // camera name → true while toggling

  // Test status
  let testStatus = $state({})
  let configError = $state('')

  async function loadConfig() {
    loading = true
    try {
      const [cfgRes, ttsRes, camRes, voiceRes, visionRes, airplayRes, settingsRes] = await Promise.all([
        fetch('/api/config'),
        fetch('/api/config/tts'),
        fetch('/api/config/cameras'),
        fetch('/api/voices'),
        fetch('/api/config/vision'),
        fetch('/api/config/airplay'),
        fetch('/api/config/settings'),
      ])
      config = await cfgRes.json()
      const ttsData = await ttsRes.json()
      ttsPresets = ttsData.presets ?? []
      activeTTS = ttsData.active?.url ?? ''
      cameras = await camRes.json() ?? []
      voices = await voiceRes.json() ?? []
      const v = await visionRes.json()
      visionURL = v.url ?? ''
      visionModel = v.model ?? ''
      visionAPIKey = v.api_key ?? ''
      visionPrompt = v.prompt ?? ''
      const ap = await airplayRes.json()
      airplayEnabled = ap.enabled ?? false
      airplayBasePort = ap.base_port ?? 5000
      airplayPrimeSilenceMs = ap.prime_silence_ms ?? 500
      airplayCameras = (ap.per_camera ?? []).sort((a, b) => a.name.localeCompare(b.name))
      const st = await settingsRes.json()
      frigateURL = st.frigate_url ?? ''
      go2rtcURL = st.go2rtc_url ?? ''
      advertiseIP = st.advertise_ip ?? ''
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
      ttsFormOpen = false
      ttsName = ''; ttsEndpoint = ''; ttsModel = ''; ttsVoice = ''; ttsKey = ''; ttsDesc = ''
      loadConfig()
    } catch (e) {
      ttsStatus = '✗ ' + e.message
    } finally {
      setTimeout(() => ttsStatus = '', 4000)
    }
  }

  async function activateTTS(name) {
    try {
      const res = await fetch(`/api/config/tts/${name}/activate`, { method: 'POST' })
      if (!res.ok) throw new Error(await res.text())
      loadConfig()
    } catch (e) {
      configError = '✗ ' + e.message
    }
  }

  async function deleteTTS(name) {
    if (!confirm(`Delete TTS preset "${name}"?`)) return
    try {
      const res = await fetch(`/api/config/tts/${name}`, { method: 'DELETE' })
      if (!res.ok) throw new Error(await res.text())
      loadConfig()
    } catch (e) {
      configError = '✗ ' + e.message
    }
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
          stream: camStream, enabled: camEnabled,
          airplay_name: camAirPlayName, vision_prompt: camVisionPrompt,
        }),
      })
      if (!res.ok) throw new Error(await res.text())
      camStatus = '✓ Saved'
      camFormOpen = false
      camName = ''; camIP = ''; camUser = ''; camPass = ''; camChannel = 1; camStream = ''; camVisionPrompt = ''
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
    try {
      const res = await fetch(`/api/config/cameras/${name}`, { method: 'DELETE' })
      if (!res.ok) throw new Error(await res.text())
      loadConfig()
      onRefresh?.()
    } catch (e) {
      configError = '✗ ' + e.message
    }
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

  function openAddCamera() {
    camName = ''; camType = 'hikvision'; camIP = ''; camUser = ''; camPass = ''
    camChannel = 1; camStream = ''; camEnabled = false; camAirPlayName = ''; camVisionPrompt = ''
    camStatus = ''; testCamStatus = ''
    camFormOpen = true
  }

  function editCamera(cam) {
    camName = cam.name
    camType = cam.type
    camIP = cam.ip
    camUser = cam.user ?? ''
    camPass = cam.pass ?? ''
    camChannel = cam.channel || 1
    camStream = cam.stream || ''
    camEnabled = cam.enabled ?? false
    camAirPlayName = cam.airplay_name ?? ''
    camVisionPrompt = cam.vision_prompt ?? ''
    camStatus = ''; testCamStatus = ''
    camFormOpen = true
  }

  async function pingCamera(name) {
    testCamBusy = true
    testCamStatus = ''
    try {
      const res = await fetch(`/api/cameras/${encodeURIComponent(name)}/ping`, { method: 'POST' })
      const data = await res.json()
      testCamStatus = data.ok ? '✓ Reachable' : '✗ Unreachable'
    } catch (e) {
      testCamStatus = '✗ ' + e.message
    } finally {
      testCamBusy = false
      setTimeout(() => testCamStatus = '', 5000)
    }
  }

  async function toggleCamera(cam) {
    try {
      const res = await fetch(`/api/config/cameras/${encodeURIComponent(cam.name)}/toggle`, {
        method: 'PATCH',
      })
      if (!res.ok) throw new Error(await res.text())
      loadConfig()
      onRefresh?.()
    } catch (e) {
      console.error('toggle error:', e)
    }
  }

  function openAddTTS() {
    ttsName = ''; ttsEndpoint = ''; ttsModel = ''; ttsVoice = ''; ttsKey = ''; ttsDesc = ''
    ttsStatus = ''
    ttsFormOpen = true
  }

  function editTTS(p) {
    ttsName = p.name
    ttsEndpoint = p.endpoint
    ttsModel = p.model
    ttsVoice = p.default_voice
    ttsKey = ''
    ttsDesc = p.description
    ttsStatus = ''
    ttsFormOpen = true
  }

  // --- Cameras (batch enable) ---
  function getCamEnabled(cam) {
    return cam.name in pendingEnabled ? pendingEnabled[cam.name] : cam.enabled
  }

  function localToggleCamera(cam) {
    const current = getCamEnabled(cam)
    // If toggled back to original, remove from pending
    if (current === cam.enabled) {
      pendingEnabled = { ...pendingEnabled, [cam.name]: !current }
    } else {
      const { [cam.name]: _, ...rest } = pendingEnabled
      pendingEnabled = rest
    }
  }

  let camerasSaving = $state(false)
  let camerasStatus = $state('')

  async function saveCamerasEnabled() {
    camerasSaving = true
    camerasStatus = ''
    try {
      const changed = Object.entries(pendingEnabled)
      for (const [name, enabled] of changed) {
        const cam = cameras.find(c => c.name === name)
        if (!cam) continue
        const res = await fetch('/api/config/cameras', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            name, type: cam.type, ip: cam.ip, user: cam.user,
            pass: cam.pass, channel: cam.channel, stream: cam.stream || '',
            enabled, airplay_name: cam.airplay_name || '', vision_prompt: cam.vision_prompt || '',
          }),
        })
        if (!res.ok) throw new Error(await res.text())
      }
      pendingEnabled = {}
      camerasStatus = '✓ Saved'
      loadConfig()
      onRefresh?.()
    } catch (e) {
      camerasStatus = '✗ ' + e.message
    } finally {
      camerasSaving = false
      setTimeout(() => camerasStatus = '', 4000)
    }
  }

  // --- Vision ---
  async function saveVision() {
    visionStatus = ''
    try {
      const res = await fetch('/api/config/vision', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          url: visionURL,
          model: visionModel,
          api_key: visionAPIKey,
          prompt: visionPrompt,
        }),
      })
      if (!res.ok) throw new Error(await res.text())
      visionStatus = '✓ Saved'
      loadConfig()
    } catch (e) {
      visionStatus = '✗ ' + e.message
    } finally {
      setTimeout(() => visionStatus = '', 4000)
    }
  }

  async function testVision() {
    visionTestBusy = true
    visionTestStatus = ''
    try {
      const res = await fetch('/api/config/vision/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url: visionURL, api_key: visionAPIKey }),
      })
      const data = await res.json()
      if (data.ok) {
        visionTestStatus = `✓ Connected (${data.models} model${data.models === 1 ? '' : 's'})`
      } else {
        visionTestStatus = '✗ ' + data.message
      }
    } catch (e) {
      visionTestStatus = '✗ ' + e.message
    } finally {
      visionTestBusy = false
      setTimeout(() => visionTestStatus = '', 8000)
    }
  }

  // --- AirPlay ---
  async function toggleCameraAirPlay(cam) {
    airplayToggling = { ...airplayToggling, [cam.name]: true }
    try {
      const res = await fetch(`/api/config/airplay/${encodeURIComponent(cam.name)}/toggle`, {
        method: 'PATCH',
      })
      if (!res.ok) throw new Error(await res.text())
      loadConfig()
    } catch (e) {
      configError = '✗ ' + e.message
    } finally {
      airplayToggling = { ...airplayToggling, [cam.name]: false }
    }
  }

  let discoverStatus = $state('')
  let discoverBusy = $state(false)

  async function discoverCameras() {
    discoverBusy = true
    discoverStatus = ''
    try {
      const res = await fetch('/api/config/cameras/discover', { method: 'POST' })
      if (!res.ok) throw new Error(await res.text())
      const data = await res.json()
      discoverStatus = `✓ Found ${data.discovered} camera${data.discovered === 1 ? '' : 's'}`
      if (data.discovered > 0) loadConfig()
    } catch (e) {
      discoverStatus = '✗ ' + e.message
    } finally {
      discoverBusy = false
      setTimeout(() => discoverStatus = '', 8000)
    }
  }

  async function saveAirPlay() {
    airplayStatus = ''
    try {
      const res = await fetch('/api/config/airplay', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          enabled: airplayEnabled,
          base_port: parseInt(airplayBasePort) || 5000,
          prime_silence_ms: parseInt(airplayPrimeSilenceMs) || 500,
        }),
      })
      if (!res.ok) throw new Error(await res.text())
      airplayStatus = '✓ Saved'
      loadConfig()
    } catch (e) {
      airplayStatus = '✗ ' + e.message
    } finally {
      setTimeout(() => airplayStatus = '', 6000)
    }
  }

  // Normalize a bare host/IP to a full URL (add http:// if no scheme)
  function normalizeURL(url) {
    if (!url) return url
    url = url.trim()
    if (!/^https?:\/\//i.test(url)) url = 'http://' + url
    return url
  }

  async function testFrigate() {
    if (!frigateURL) { frigateTestStatus = '✗ No URL set'; return }
    frigateTestBusy = true
    frigateTestStatus = ''
    try {
      const url = normalizeURL(frigateURL)
      const res = await fetch('/api/config/settings/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ type: 'frigate', url }),
      })
      const data = await res.json()
      if (data.ok) {
        frigateTestStatus = `✓ Frigate ${data.data?.version ?? 'connected'}`
      } else {
        frigateTestStatus = '✗ ' + data.message
      }
    } catch (e) {
      frigateTestStatus = '✗ ' + (e.message ?? 'unreachable')
    } finally {
      frigateTestBusy = false
      setTimeout(() => frigateTestStatus = '', 6000)
    }
  }

  async function testGo2rtc() {
    if (!go2rtcURL) { go2rtcTestStatus = '✗ No URL set'; return }
    go2rtcTestBusy = true
    go2rtcTestStatus = ''
    try {
      const url = normalizeURL(go2rtcURL)
      const res = await fetch('/api/config/settings/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ type: 'go2rtc', url }),
      })
      const data = await res.json()
      if (data.ok) {
        const count = Object.keys(data.data ?? {}).length
        go2rtcTestStatus = `✓ go2rtc (${count} stream${count === 1 ? '' : 's'})`
      } else {
        go2rtcTestStatus = '✗ ' + data.message
      }
    } catch (e) {
      go2rtcTestStatus = '✗ ' + (e.message ?? 'unreachable')
    } finally {
      go2rtcTestBusy = false
      setTimeout(() => go2rtcTestStatus = '', 6000)
    }
  }

  function inferGo2rtcFromFrigate() {
    if (!frigateURL) return
    try {
      const u = new URL(normalizeURL(frigateURL))
      go2rtcURL = `${u.protocol}//${u.hostname}:1984`
    } catch {}
  }

  async function saveSettings() {
    settingsStatus = ''
    try {
      const res = await fetch('/api/config/settings', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ frigate_url: normalizeURL(frigateURL), go2rtc_url: normalizeURL(go2rtcURL), advertise_ip: advertiseIP }),
      })
      if (!res.ok) throw new Error(await res.text())
      settingsStatus = '✓ Saved'
      loadConfig()
    } catch (e) {
      settingsStatus = '✗ ' + e.message
    } finally {
      setTimeout(() => settingsStatus = '', 4000)
    }
  }

  const configTabs = [
    { id: 'settings', label: 'Settings' },
    { id: 'tts', label: 'TTS Presets' },
    { id: 'cameras', label: 'Cameras' },
    { id: 'vision', label: 'Vision' },
    { id: 'airplay', label: 'AirPlay' },
    { id: 'overview', label: 'Overview' },
  ]
</script>

{#if loading}
  <p class="flex items-center gap-2 text-muted-foreground"><Loader2 class="h-4 w-4 animate-spin" /> Loading config…</p>
{:else}
  <div class="flex flex-col gap-4">
    {#if configError}<p class="text-sm text-destructive">{configError}</p>{/if}
    <div class="flex gap-1">
      {#each configTabs as t}
        <Button
          variant={tab === t.id ? 'default' : 'ghost'}
          size="sm"
          onclick={() => tab = t.id}
        >
          {t.label}
        </Button>
      {/each}
    </div>

    <!-- TTS Presets -->
    {#if tab === 'settings'}
      <section class="rounded-lg border bg-card p-5">
        <div class="mb-4 flex items-center justify-between gap-4">
          <h3 class="text-base font-semibold text-primary">General Settings</h3>
          <div class="flex items-center gap-2">
            {#if settingsStatus}<span class="text-sm text-primary">{settingsStatus}</span>{/if}
            <Button onclick={saveSettings} size="sm">Save</Button>
          </div>
        </div>
        <p class="mb-4 text-sm text-muted-foreground">
          Integration URLs for Frigate NVR, go2rtc, and network advertising.
        </p>
        <div class="grid grid-cols-1 gap-4 max-w-lg">
          <div class="flex flex-col gap-1 text-xs text-muted-foreground">
            <span class="font-medium">Frigate URL</span>
            <div class="flex gap-2">
              <Input bind:value={frigateURL} placeholder="http://10.0.0.x:5000" class="flex-1" />
              <Button variant="outline" size="sm" onclick={testFrigate} disabled={frigateTestBusy} class="shrink-0">
                {frigateTestBusy ? 'Testing…' : 'Test'}
              </Button>
            </div>
            {#if frigateTestStatus}<span class="text-[11px] text-primary">{frigateTestStatus}</span>{/if}
            <span class="text-[11px] opacity-60">Used for camera discovery and snapshot thumbnails.</span>
          </div>
          <div class="flex flex-col gap-1 text-xs text-muted-foreground">
            <span class="font-medium">go2rtc URL</span>
            <div class="flex gap-2">
              <Input bind:value={go2rtcURL} placeholder="http://10.0.0.x:1984" class="flex-1" />
              <Button variant="outline" size="sm" onclick={inferGo2rtcFromFrigate} disabled={!frigateURL} title="Infer from Frigate URL (same host, port 1984)" class="shrink-0">
                Infer
              </Button>
              <Button variant="outline" size="sm" onclick={testGo2rtc} disabled={go2rtcTestBusy} class="shrink-0">
                {go2rtcTestBusy ? 'Testing…' : 'Test'}
              </Button>
            </div>
            {#if go2rtcTestStatus}<span class="text-[11px] text-primary">{go2rtcTestStatus}</span>{/if}
            <span class="text-[11px] opacity-60">Required for go2rtc-type cameras. "Infer" fills this from the Frigate host.</span>
          </div>
          <div class="flex flex-col gap-1 text-xs text-muted-foreground">
            <span class="font-medium">AirPlay Advertise IP</span>
            <Input bind:value={advertiseIP} placeholder="auto-detect" />
            <span class="text-[11px] opacity-60">Force a specific LAN IP for AirPlay mDNS (useful in Docker with host networking).</span>
          </div>
        </div>
      </section>

    {:else if tab === 'tts'}
      <section class="rounded-lg border bg-card p-5">
        <div class="mb-3 flex items-center justify-between">
          <h3 class="text-base font-semibold text-primary">TTS Presets</h3>
          <div class="flex items-center gap-2">
            {#if testStatus.tts}<span class="text-sm text-primary">{testStatus.tts}</span>{/if}
            <Button variant="outline" size="sm" onclick={testTTS}>Test Connection</Button>
            <Button size="sm" onclick={openAddTTS}>Add Preset</Button>
          </div>
        </div>

        <div class="flex flex-col gap-1.5">
          {#each ttsPresets as p}
            <div class="flex items-center justify-between rounded-lg border bg-background px-3 py-2 {p.is_active ? 'border-primary bg-primary/5' : ''}">
              <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
                <span class="font-semibold">{p.name}</span>
                {#if p.is_active}<Badge>ACTIVE</Badge>{/if}
                <span class="text-sm text-muted-foreground">{p.model}</span>
                <span class="text-sm text-muted-foreground">{p.default_voice}</span>
              </div>
              <div class="flex shrink-0 gap-1">
                <Button variant="outline" size="sm" class="h-7 px-2" onclick={() => editTTS(p)} title="Edit" aria-label="Edit TTS preset"><Pencil class="h-4 w-4" /></Button>
                {#if !p.is_active}
                  <Button variant="outline" size="sm" class="h-7 px-2" onclick={() => activateTTS(p.name)} title="Activate" aria-label="Activate TTS preset"><Check class="h-4 w-4" /></Button>
                {/if}
                <Button variant="outline" size="sm" class="h-7 px-2 hover:border-destructive hover:text-destructive" onclick={() => deleteTTS(p.name)} title="Delete" aria-label="Delete TTS preset"><X class="h-4 w-4" /></Button>
              </div>
            </div>
          {/each}
          {#if ttsPresets.length === 0}
            <p class="italic text-muted-foreground">No TTS presets configured.</p>
          {/if}
        </div>
      </section>

      <!-- TTS Edit Modal -->
      <Modal bind:open={ttsFormOpen} title={ttsName ? `Edit TTS Preset — ${ttsName}` : 'Add TTS Preset'}>
        <div class="grid grid-cols-2 gap-2.5 max-sm:grid-cols-1">
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Name
            <Input bind:value={ttsName} placeholder="lemonade-local" />
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Endpoint
            <Input bind:value={ttsEndpoint} placeholder="http://10.0.0.x:13305/v1/audio/speech" />
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
        <div class="mt-4 flex flex-wrap items-center gap-2 border-t pt-4">
          <Button onclick={saveTTS} disabled={!ttsName || !ttsEndpoint}>Save Preset</Button>
          <Button variant="outline" onclick={testTTS}>Test Connection</Button>
          <Button variant="ghost" onclick={() => ttsFormOpen = false}>Cancel</Button>
          {#if ttsStatus}<span class="text-sm text-primary">{ttsStatus}</span>{/if}
          {#if testStatus.tts}<span class="text-sm text-primary">{testStatus.tts}</span>{/if}
        </div>
      </Modal>

    <!-- Cameras -->
    {:else if tab === 'cameras'}
      <section class="rounded-lg border bg-card p-5">
        <div class="mb-3 flex items-center justify-between gap-4">
          <h3 class="text-base font-semibold text-primary">Cameras</h3>
          <div class="flex items-center gap-2">
            {#if camerasStatus}<span class="text-sm text-primary">{camerasStatus}</span>{/if}
            {#if camerasDirty}
              <Button size="sm" onclick={saveCamerasEnabled} disabled={camerasSaving}>
                {camerasSaving ? 'Saving…' : 'Save'}
              </Button>
            {/if}
            <Button size="sm" variant="outline" onclick={discoverCameras} disabled={discoverBusy} title={frigateURL ? 'Discover cameras from Frigate NVR' : 'Set a Frigate URL in Settings first'}>
              {#if discoverBusy}Discovering…{:else}Discover{/if}
            </Button>
            <Button size="sm" onclick={openAddCamera}>Add</Button>
          </div>
        </div>
        {#if discoverStatus}<p class="mb-2 text-sm text-primary">{discoverStatus}</p>{/if}

        <div class="flex flex-col gap-1.5">
          {#each cameras as cam}
            <div class="flex items-center justify-between rounded-lg border bg-background px-3 py-2 {!getCamEnabled(cam) ? 'opacity-50' : ''}">
              <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
                <input
                  type="checkbox"
                  checked={getCamEnabled(cam)}
                  onchange={() => localToggleCamera(cam)}
                  class="h-4 w-4 cursor-pointer rounded border-input accent-primary"
                  title={getCamEnabled(cam) ? 'Disable (click Save to commit)' : 'Enable (click Save to commit)'}
                />
                <span class="font-semibold">{cam.name}</span>
                <span class="text-sm text-muted-foreground">{cam.type}</span>
                <span class="text-sm text-muted-foreground">{cam.ip}</span>
                <span class="text-sm text-muted-foreground">ch{cam.channel}</span>
                {#if !cam.enabled}<span class="text-xs text-muted-foreground italic">disabled</span>{/if}
              </div>
              <div class="flex shrink-0 items-center gap-1">
                {#if testStatus[cam.name]}<span class="mr-1 text-sm text-primary">{testStatus[cam.name]}</span>{/if}
                <Button variant="outline" size="sm" class="h-7 px-2" onclick={() => testCamera(cam.name)} title="Test beep" aria-label="Test beep" disabled={!getCamEnabled(cam)}><Bell class="h-4 w-4" /></Button>
                <Button variant="outline" size="sm" class="h-7 px-2" onclick={() => editCamera(cam)} title="Edit camera settings" aria-label="Edit camera"><Pencil class="h-4 w-4" /></Button>
                <Button variant="outline" size="sm" class="h-7 px-2 hover:border-destructive hover:text-destructive" onclick={() => deleteCamera(cam.name)} title="Delete" aria-label="Delete camera"><X class="h-4 w-4" /></Button>
              </div>
            </div>
          {/each}
          {#if cameras.length === 0}
            <p class="italic text-muted-foreground text-sm">No cameras configured. Use Discover or Add Camera.</p>
          {/if}
        </div>
      </section>

      <!-- Camera Edit Modal -->
      <Modal bind:open={camFormOpen} title={camName ? `Edit Camera — ${camName}` : 'Add Camera'}>
        <div class="grid grid-cols-2 gap-2.5 max-sm:grid-cols-1">
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Name
            <Input bind:value={camName} placeholder="backyard" />
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Type
            <Select bind:value={camType}>
              <option value="hikvision">hikvision</option>
              <option value="reolink">reolink</option>
              <option value="go2rtc">go2rtc</option>
              <option value="onvif">onvif</option>
            </Select>
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            IP
            <Input bind:value={camIP} placeholder="10.0.0.x" />
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
          {#if camType === 'go2rtc' || camType === 'onvif'}
          <label class="flex flex-col gap-1 text-xs text-muted-foreground col-span-2">
            {camType === 'go2rtc' ? 'go2rtc Stream Name' : 'RTSP URL'}
            <Input bind:value={camStream} placeholder={camType === 'go2rtc' ? 'garage_2way' : 'rtsp://user:pass@ip:554/stream0'} />
          </label>
          {/if}
        </div>
        <label class="mt-3 flex flex-col gap-1 text-xs text-muted-foreground">
          AirPlay Name (optional)
          <Input bind:value={camAirPlayName} placeholder={camName ? camName.replace(/[-_]/g, ' ').replace(/\b\w/g, c => c.toUpperCase()) + ' Camera' : 'Backyard Camera'} />
          <span class="text-[11px] opacity-60">Name shown in iOS AirPlay picker. Auto-derived from camera name if left empty. Saving restarts the AirPlay receiver.</span>
        </label>
        <label class="mt-3 flex flex-col gap-1 text-xs text-muted-foreground">
          Vision Prompt (optional)
          <textarea
            bind:value={camVisionPrompt}
            rows="2"
            placeholder="Describe what you see. Focus on people, vehicles, and animals."
            class="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm
                   placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1
                   focus-visible:ring-ring disabled:opacity-50 resize-none"
          ></textarea>
          <span class="text-[11px] opacity-60">Used when clicking Describe on this camera. Can be overridden per-session.</span>
        </label>
        <label class="mt-3 flex items-center gap-2 text-sm text-muted-foreground">
          <input type="checkbox" bind:checked={camEnabled} class="h-4 w-4 cursor-pointer rounded border-input accent-primary" />
          Enabled (camera will receive speak/broadcast)
        </label>
        <div class="mt-4 flex flex-wrap items-center gap-2 border-t pt-4">
          <Button onclick={saveCamera} disabled={!camName || !camIP}>Save Camera</Button>
          <Button variant="outline" onclick={() => pingCamera(camName)} disabled={!camName || testCamBusy}>
            {#if testCamBusy}Testing…{:else}Test Connection{/if}
          </Button>
          <Button variant="ghost" onclick={() => camFormOpen = false}>Cancel</Button>
          {#if camStatus}<span class="text-sm text-primary">{camStatus}</span>{/if}
          {#if testCamStatus}<span class="text-sm text-primary">{testCamStatus}</span>{/if}
        </div>
      </Modal>

    <!-- Vision -->
    {:else if tab === 'vision'}
      <section class="rounded-lg border bg-card p-5">
        <div class="mb-4 flex items-center justify-between gap-4">
          <h3 class="text-base font-semibold text-primary">Vision Model</h3>
          <div class="flex items-center gap-2">
            {#if visionTestStatus}<span class="text-sm text-primary">{visionTestStatus}</span>{/if}
            {#if visionStatus}<span class="text-sm text-primary">{visionStatus}</span>{/if}
            <Button variant="outline" size="sm" onclick={testVision} disabled={visionTestBusy}>
              {visionTestBusy ? 'Testing…' : 'Test'}
            </Button>
            <Button onclick={saveVision} size="sm">Save</Button>
          </div>
        </div>
        <p class="mb-4 text-sm text-muted-foreground">
          OpenAI-compatible vision endpoint for the Describe and Vision endpoints.
          The default prompt is used when neither the request nor the camera specifies one.
        </p>
        <div class="grid grid-cols-2 gap-2.5 max-sm:grid-cols-1">
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Endpoint URL
            <Input bind:value={visionURL} placeholder="http://10.0.0.x:8080/v1/chat/completions" />
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Model
            <Input bind:value={visionModel} placeholder="llama3.2-vision" />
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground col-span-2">
            API Key (optional)
            <Input bind:value={visionAPIKey} type="password" placeholder="sk-..." />
          </label>
        </div>
        <label class="flex flex-col gap-1 text-xs text-muted-foreground col-span-2 mt-3">
          Default Vision Prompt
          <textarea
            bind:value={visionPrompt}
            rows="3"
            placeholder="Describe what you see in one or two sentences. Be concise and factual."
            class="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm
                   placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1
                   focus-visible:ring-ring disabled:opacity-50 resize-none"
          ></textarea>
          <span class="text-[11px] opacity-60">
            Fallback chain: request prompt → camera's vision_prompt → this global default → hardcoded default.
            Leave empty to use the hardcoded default.
          </span>
        </label>
      </section>

    <!-- AirPlay -->
    {:else if tab === 'airplay'}
      <section class="rounded-lg border bg-card p-5">
        <div class="mb-4 flex items-center justify-between gap-4">
          <h3 class="text-base font-semibold text-primary">AirPlay Receivers</h3>
          <div class="flex items-center gap-2">
            {#if airplayStatus}<span class="text-sm text-primary">{airplayStatus}</span>{/if}
            <Button onclick={saveAirPlay} size="sm">Save</Button>
          </div>
        </div>
        <p class="mb-4 text-sm text-muted-foreground">
          When enabled, each camera appears as a separate AirPlay target in the iOS AirPlay picker.
          Audio from your iPhone is decoded by shairport-sync and sent to the camera speaker.
        </p>

        <!-- Global toggle + port -->
        <div class="flex flex-wrap items-center gap-4">
          <label class="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              bind:checked={airplayEnabled}
              class="h-4 w-4 cursor-pointer rounded border-input accent-primary"
            />
            Enable AirPlay globally
          </label>
          <label class="flex items-center gap-2 text-xs text-muted-foreground">
            Base port
            <Input bind:value={airplayBasePort} type="number" min="1024" max="65535" class="w-24" />
          </label>
          <label class="flex items-center gap-2 text-xs text-muted-foreground">
            Prime silence
            <Input bind:value={airplayPrimeSilenceMs} type="number" min="0" max="5000" class="w-24" />
            <span class="text-xs text-muted-foreground/70">ms</span>
          </label>
        </div>

        <!-- Per-camera toggles -->
        {#if airplayCameras.length > 0}
          <div class="mt-4 flex flex-col gap-1.5">
            <p class="text-xs font-medium text-muted-foreground">Per-camera AirPlay</p>
            {#each airplayCameras as cam}
              <div class="flex items-center justify-between rounded-lg border bg-background px-3 py-2">
                <div class="flex items-center gap-2">
                  <input
                    type="checkbox"
                    checked={cam.airplay_enabled}
                    disabled={airplayToggling[cam.name]}
                    onchange={() => toggleCameraAirPlay(cam)}
                    class="h-4 w-4 cursor-pointer rounded border-input accent-primary disabled:cursor-wait"
                    title={cam.airplay_enabled ? 'Disable AirPlay for this camera' : 'Enable AirPlay for this camera'}
                  />
                  <span class="font-semibold">{cam.name}</span>
                  {#if cam.airplay_name}
                    <span class="text-xs text-muted-foreground">→ "{cam.airplay_name}"</span>
                  {/if}
                </div>
                <div class="flex items-center gap-2">
                  {#if cam.airplay_running}
                    <span class="inline-flex items-center gap-1 rounded-full bg-green-500/10 px-2 py-0.5 text-xs font-medium text-green-600">
                      <span class="h-1.5 w-1.5 rounded-full bg-green-500"></span>
                      running
                    </span>
                  {:else if cam.airplay_enabled}
                    <span class="text-xs text-muted-foreground italic">stopped</span>
                  {:else}
                    <span class="text-xs text-muted-foreground italic">disabled</span>
                  {/if}
                </div>
              </div>
            {/each}
          </div>
        {:else}
          <p class="mt-4 text-sm text-muted-foreground italic">No cameras configured yet.</p>
        {/if}
      </section>

    <!-- Overview -->
    {:else if tab === 'overview'}
      <section class="rounded-lg border bg-card p-5">
        <h3 class="mb-3 text-base font-semibold text-primary">Runtime Configuration</h3>
        <JsonCode code={JSON.stringify(config, null, 2)} class="max-h-[600px] overflow-auto text-sm" />
      </section>
    {/if}
  </div>
{/if}
