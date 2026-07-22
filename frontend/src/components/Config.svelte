<script>
  import { onMount } from 'svelte'
  import { Bell, Pencil, X, Check, Loader2 } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { Select } from '$lib/components/ui/select'
  import { Badge } from '$lib/components/ui/badge'
  import JsonCode from '$lib/components/JsonCode.svelte'

  let { onRefresh } = $props()

  let tab = $state('tts')
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

  // Camera form
  let camFormOpen = $state(false)
  let testCamStatus = $state('')
  let testCamBusy = $state(false)
  let camName = $state('')
  let camType = $state('hikvision')
  let camIP = $state('')
  let camUser = $state('')
  let camPass = $state('')
  let camChannel = $state(1)
  let camStream = $state('')
  let camEnabled = $state(false)
  let camVisionPrompt = $state('')
  let camStatus = $state('')

  // Vision form
  let visionURL = $state('')
  let visionModel = $state('')
  let visionAPIKey = $state('')
  let visionPrompt = $state('')
  let visionStatus = $state('')

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
      const [cfgRes, ttsRes, camRes, voiceRes, visionRes, airplayRes] = await Promise.all([
        fetch('/api/config'),
        fetch('/api/config/tts'),
        fetch('/api/config/cameras'),
        fetch('/api/voices'),
        fetch('/api/config/vision'),
        fetch('/api/config/airplay'),
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
          stream: camStream, enabled: camEnabled, vision_prompt: camVisionPrompt,
        }),
      })
      if (!res.ok) throw new Error(await res.text())
      camStatus = '✓ Saved'
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

  function editCamera(cam) {
    camName = cam.name
    camType = cam.type
    camIP = cam.ip
    camUser = cam.user ?? ''
    camPass = cam.pass ?? ''
    camChannel = cam.channel || 1
    camStream = cam.stream || ''
    camEnabled = cam.enabled ?? false
    camVisionPrompt = cam.vision_prompt ?? ''
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

  function editTTS(p) {
    ttsName = p.name
    ttsEndpoint = p.endpoint
    ttsModel = p.model
    ttsVoice = p.default_voice
    ttsDesc = p.description
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

  const configTabs = [
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

        <details class="border-t pt-3">
          <summary class="cursor-pointer py-1.5 text-sm text-primary hover:text-primary/80">{ttsName ? 'Edit' : 'Add'} TTS Preset</summary>
          <div class="mt-3 grid grid-cols-2 gap-2.5 max-sm:grid-cols-1">
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
            <div class="flex items-center justify-between rounded-lg border bg-background px-3 py-2 {!cam.enabled ? 'opacity-50' : ''}">
              <div class="flex min-w-0 flex-1 flex-wrap items-center gap-2">
                <input
                  type="checkbox"
                  checked={cam.enabled}
                  onchange={() => toggleCamera(cam)}
                  class="h-4 w-4 cursor-pointer rounded border-input accent-primary"
                  title={cam.enabled ? 'Disable' : 'Enable'}
                />
                <span class="font-semibold">{cam.name}</span>
                <span class="text-sm text-muted-foreground">{cam.type}</span>
                <span class="text-sm text-muted-foreground">{cam.ip}</span>
                <span class="text-sm text-muted-foreground">ch{cam.channel}</span>
                {#if !cam.enabled}<span class="text-xs text-muted-foreground italic">disabled</span>{/if}
              </div>
              <div class="flex shrink-0 items-center gap-1">
                {#if testStatus[cam.name]}<span class="mr-1 text-sm text-primary">{testStatus[cam.name]}</span>{/if}
                <Button variant="outline" size="sm" class="h-7 px-2" onclick={() => testCamera(cam.name)} title="Test beep" aria-label="Test beep" disabled={!cam.enabled}><Bell class="h-4 w-4" /></Button>
                <Button variant="outline" size="sm" class="h-7 px-2" onclick={() => editCamera(cam)} title="Edit camera settings" aria-label="Edit camera"><Pencil class="h-4 w-4" /></Button>
                <Button variant="outline" size="sm" class="h-7 px-2 hover:border-destructive hover:text-destructive" onclick={() => deleteCamera(cam.name)} title="Delete" aria-label="Delete camera"><X class="h-4 w-4" /></Button>
              </div>
            </div>
          {/each}
          <div class="mb-2 flex flex-wrap items-center gap-3">
            <Button size="sm" variant="outline" onclick={discoverCameras} disabled={discoverBusy} title={config?.frigate_url ? 'Discover cameras from Frigate NVR' : 'Set a Frigate URL in Settings first'}>
              {#if discoverBusy}Discovering…{:else}Discover from Frigate{/if}
            </Button>
            {#if discoverStatus}<span class="text-sm text-primary">{discoverStatus}</span>{/if}
          </div>
          {#if cameras.length === 0}
            <p class="italic text-muted-foreground text-sm">No cameras configured. Use Discover or add one below.</p>
          {/if}
        </div>

        <details class="border-t pt-3" bind:open={camFormOpen}>
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
          <label class="flex flex-col gap-1 text-xs text-muted-foreground col-span-2 mt-1">
            Vision Prompt (optional default for Describe)
            <textarea
              bind:value={camVisionPrompt}
              rows="2"
              placeholder="Describe what you see. Focus on people, vehicles, and animals."
              class="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm
                     placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1
                     focus-visible:ring-ring disabled:opacity-50 resize-none"
            ></textarea>
            <span class="text-[11px] opacity-60">Used when clicking Describe on this camera. Can be overridden per-session in the camera card.</span>
          </label>
          <label class="mt-3 flex items-center gap-2 text-sm text-muted-foreground">
            <input type="checkbox" bind:checked={camEnabled} class="h-4 w-4 cursor-pointer rounded border-input accent-primary" />
            Enabled (camera will receive speak/broadcast)
          </label>
          <div class="mt-3 flex flex-wrap items-center gap-2">
            <Button onclick={saveCamera} disabled={!camName || !camIP}>Save Camera</Button>
            <Button variant="outline" onclick={() => pingCamera(camName)} disabled={!camName || testCamBusy}>Test Connection</Button>
            {#if camStatus}<span class="text-sm text-primary">{camStatus}</span>{/if}
            {#if testCamStatus}<span class="text-sm text-primary">{testCamStatus}</span>{/if}
          </div>
        </details>
      </section>

    <!-- Vision -->
    {:else if tab === 'vision'}
      <section class="rounded-lg border bg-card p-5">
        <h3 class="mb-1 text-base font-semibold text-primary">Vision Model</h3>
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
        <Button onclick={saveVision} class="mt-3">
          Save Vision Config
        </Button>
        {#if visionStatus}<span class="ml-2 text-sm text-primary">{visionStatus}</span>{/if}
      </section>

    <!-- AirPlay -->
    {:else if tab === 'airplay'}
      <section class="rounded-lg border bg-card p-5">
        <h3 class="mb-1 text-base font-semibold text-primary">AirPlay Receivers</h3>
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
          <Button onclick={saveAirPlay} size="sm">Save</Button>
          {#if airplayStatus}<span class="text-sm text-primary">{airplayStatus}</span>{/if}
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
