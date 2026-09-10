<script>
  import { onMount } from 'svelte'
  import { Bell, Pencil, X, Loader2, ScanLine } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { Select } from '$lib/components/ui/select'
  import { Badge } from '$lib/components/ui/badge'
  import JsonCode from '$lib/components/JsonCode.svelte'
  import Modal from '$lib/components/Modal.svelte'
  import CaptureTest from '$lib/components/CaptureTest.svelte'
  import PromptEditor from '$lib/components/PromptEditor.svelte'
  import TTSSettings from './TTSSettings.svelte'
  import VisionSettings from './VisionSettings.svelte'
  import { toast } from '$lib/components/ui/toast'
  import { apiClient } from '$lib/api'

  let { onRefresh } = $props()

  let tab = $state('settings')
  let config = $state(null)
  let cameras = $state([])
  let loading = $state(true)

  // Camera modal
  let camFormOpen = $state(false)
  let testCamStatus = $state('')
  let testCamBusy = $state(false)
  let detectCamBusy = $state(false)
  let detectCamStatus = $state('')

  // go2rtc streams for dropdown population
  let go2rtcStreams = $state([])
  let go2rtcStreamsLoading = $state(false)
  let go2rtcStreamsError = $state('')
  let camStreamCustom = $state(false) // toggle for free-text stream entry

  let camName = $state('')
  let camType = $state('hikvision')
  let camIP = $state('')
  let camUser = $state('')
  let camPass = $state('')
  let camChannel = $state(1)
  let camStream = $state('')
  let camEnabled = $state(false)
  let camAirPlayName = $state('')
  let camAirPlayModel = $state('')
  let camAirPlayEnabled = $state(true)
  let camVisionPrompt = $state('')
  let camVisionStream = $state('')
  let camVisionWidth = $state(0)
  let camSnapMethod = $state('')
  let camStatus = $state('')
  let availableStreams = $state([])

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
  let airplayModel = $state('RealityDevice14,1')
  let airplayGain = $state(1.0)
  let airplayToggling = $state({})  // camera name → true while toggling

  // Test status
  let testStatus = $state({})
  let configError = $state('')

  async function loadConfig() {
    loading = true
    configError = ''
    try {
      const [cfg, cams, ap, st] = await Promise.all([
        apiClient.getConfig(),
        apiClient.listCamerasConfig(),
        apiClient.getAirPlayConfig(),
        apiClient.getSettings(),
      ])
      config = cfg
      cameras = cams ?? []
      airplayEnabled = ap.enabled ?? false
      airplayBasePort = ap.base_port ?? 5000
      airplayPrimeSilenceMs = ap.prime_silence_ms ?? 500
      airplayModel = ap.model ?? 'RealityDevice14,1'
      airplayGain = ap.gain ?? 1.0
      frigateURL = st.frigate_url ?? ''
      go2rtcURL = st.go2rtc_url ?? ''
      advertiseIP = st.advertise_ip ?? ''
    } catch (e) {
      console.error('loadConfig error:', e)
      configError = '✗ Failed to load config: ' + (e?.message ?? String(e))
    } finally {
      loading = false
    }
  }

  onMount(loadConfig)

  async function refreshRuntime() {
    try {
      config = await apiClient.getConfig()
      onRefresh?.()
    } catch (cause) {
      configError = 'Your runtime configuration could not be refreshed: ' + cause.message
    }
  }

  // --- Cameras ---
  async function saveCamera() {
    if (!camName || !camIP) return
    camStatus = ''
    try {
      await apiClient.saveCamera({
        name: camName, type: camType, ip: camIP,
        user: camUser, pass: camPass, channel: parseInt(camChannel) || 1,
        stream: camStream, enabled: camEnabled,
        airplay_enabled: camAirPlayEnabled,
        vision_prompt: camVisionPrompt,
        vision_stream: camVisionStream,
        vision_width: camVisionWidth || 0,
        snap_method: camSnapMethod,
        airplay_name: camAirPlayName,
        airplay_model: camAirPlayModel,
      })
      camStatus = '✓ Saved'
      toast.success(`Camera "${camName}" saved`)
      camFormOpen = false
      camName = ''; camIP = ''; camUser = ''; camPass = ''; camChannel = 1; camStream = ''; camVisionPrompt = ''; camVisionStream = ''; camVisionWidth = 0; camSnapMethod = ''; camAirPlayName = ''; camAirPlayModel = ''
      loadConfig()
      onRefresh?.()
    } catch (e) {
      camStatus = '✗ ' + e.message
      toast.error(`Failed to save camera: ${e.message}`)
    } finally {
      setTimeout(() => camStatus = '', 4000)
    }
  }

  async function deleteCamera(name) {
    if (!confirm(`Delete camera "${name}"?`)) return
    try {
      await apiClient.deleteCamera(name)
      toast.success(`Camera "${name}" deleted`)
      loadConfig()
      onRefresh?.()
    } catch (e) {
      configError = '✗ ' + e.message
      toast.error(`Failed to delete camera: ${e.message}`)
    }
  }

  async function testCamera(name) {
    testStatus = { ...testStatus, [name]: 'testing...' }
    try {
      await apiClient.beep({ camera: name })
      testStatus = { ...testStatus, [name]: '✓ Beep sent' }
    } catch (e) {
      let msg = e.message
      try { msg = JSON.parse(e.message).message || e.message } catch {}
      if (msg.length > 120) msg = msg.slice(0, 117) + '...'
      testStatus = { ...testStatus, [name]: '✗ ' + msg }
    }
    setTimeout(() => {
      const s = { ...testStatus }
      delete s[name]
      testStatus = s
    }, 8000)
  }

  function openAddCamera() {
    camName = ''; camType = 'hikvision'; camIP = ''; camUser = ''; camPass = ''
    camChannel = 1; camStream = ''; camEnabled = false; camVisionPrompt = ''
    camVisionStream = ''; camVisionWidth = 0; camSnapMethod = ''
    camAirPlayName = ''; camAirPlayModel = ''; camAirPlayEnabled = true
    camStatus = ''; testCamStatus = ''; detectCamStatus = ''
    camStreamCustom = false
    loadGo2rtcStreams()
    loadAvailableStreams()
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
    camAirPlayEnabled = cam.airplay_enabled ?? true
    camVisionPrompt = cam.vision_prompt ?? ''
    camVisionStream = cam.vision_stream ?? ''
    camVisionWidth = cam.vision_width ?? 0
    camSnapMethod = cam.snap_method ?? ''
    camAirPlayName = cam.airplay_name ?? ''
    camAirPlayModel = cam.airplay_model ?? airplayModel ?? 'RealityDevice14,1'
    camStatus = ''; testCamStatus = ''; detectCamStatus = ''
    camStreamCustom = false
    loadGo2rtcStreams()
    loadAvailableStreams()
    camFormOpen = true
  }

  async function loadAvailableStreams() {
    try {
      const res = await apiClient.streams()
      availableStreams = res.streams ?? []
    } catch {
      availableStreams = []
    }
  }

  async function loadGo2rtcStreams() {
    go2rtcStreamsLoading = true
    go2rtcStreamsError = ''
    try {
      const data = await apiClient.getGo2rtcStreams()
      go2rtcStreams = data.streams ?? []
      if (data.error) go2rtcStreamsError = data.error
    } catch (e) {
      go2rtcStreamsError = e.message
      go2rtcStreams = []
    } finally {
      go2rtcStreamsLoading = false
    }
  }

  async function pingCamera(name) {
    testCamBusy = true
    testCamStatus = ''
    try {
      const data = await apiClient.pingCamera(name)
      testCamStatus = data.ok ? '✓ Reachable' : '✗ Unreachable'
    } catch (e) {
      testCamStatus = '✗ ' + e.message
    } finally {
      testCamBusy = false
      setTimeout(() => testCamStatus = '', 5000)
    }
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
        await apiClient.saveCamera({
          name, type: cam.type, ip: cam.ip, user: cam.user,
          pass: cam.pass, channel: cam.channel, stream: cam.stream || '',
          enabled, airplay_name: cam.airplay_name || '', airplay_model: cam.airplay_model || '',
          vision_prompt: cam.vision_prompt || '',
        })
      }
      pendingEnabled = {}
      camerasStatus = '✓ Saved'
      toast.success('Camera settings saved')
      loadConfig()
      onRefresh?.()
    } catch (e) {
      camerasStatus = '✗ ' + e.message
      toast.error(`Failed to save: ${e.message}`)
    } finally {
      camerasSaving = false
      setTimeout(() => camerasStatus = '', 4000)
    }
  }

  // --- AirPlay ---
  async function toggleCameraAirPlay(cam) {
    airplayToggling = { ...airplayToggling, [cam.name]: true }
    try {
      await apiClient.toggleAirPlay(cam.name)
      loadConfig()
      onRefresh?.()
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
      const data = await apiClient.discoverCameras()
      discoverStatus = `✓ Found ${data.discovered} camera${data.discovered === 1 ? '' : 's'}`
      if (data.discovered > 0) loadConfig()
    } catch (e) {
      discoverStatus = '✗ ' + e.message
    } finally {
      discoverBusy = false
      setTimeout(() => discoverStatus = '', 8000)
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
      const data = await apiClient.testSettingsURL(url, 'frigate')
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
      const data = await apiClient.testSettingsURL(url, 'go2rtc')
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

  async function detectCameraType() {
    if (!camIP) { detectCamStatus = '✗ Enter an IP first'; return }
    detectCamBusy = true
    detectCamStatus = ''
    try {
      const data = await apiClient.detectCamera(camIP, camUser, camPass)
      if (data.type) {
        camType = data.type
        if (data.type === 'reolink' && !camStream) {
          const match = go2rtcStreams.find(s => s.source && s.source.includes(camIP))
          if (match) {
            camStream = match.name
          } else {
            camStream = camName || 'doorbell'
          }
        }
        detectCamStatus = `✓ Detected ${data.type}`
        if (data.note) {
          detectCamStatus += ` — ${data.note}`
        } else if (data.type === 'reolink' && data.go2rtc_url) {
          detectCamStatus += ' (needs go2rtc stream)'
        }
      } else {
        detectCamStatus = '✗ Could not detect type'
      }
    } catch (e) {
      detectCamStatus = '✗ ' + (e.message ?? 'detection failed')
    } finally {
      detectCamBusy = false
      setTimeout(() => detectCamStatus = '', 6000)
    }
  }

  async function saveSettings() {
    settingsStatus = ''
    try {
      // Save settings and AirPlay config sequentially to avoid SQLITE_BUSY
      await apiClient.saveSettings({
        frigate_url: normalizeURL(frigateURL),
        go2rtc_url: normalizeURL(go2rtcURL),
        advertise_ip: advertiseIP,
      })
      await apiClient.saveAirPlayConfig({
        enabled: airplayEnabled,
        base_port: parseInt(airplayBasePort) || 5000,
        prime_silence_ms: parseInt(airplayPrimeSilenceMs) || 500,
        model: airplayModel || 'RealityDevice14,1',
        gain: parseFloat(airplayGain) || 1.0,
      })
      settingsStatus = '✓ Saved'
      loadConfig()
      onRefresh?.()
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
    { id: 'overview', label: 'Overview' },
  ]
</script>

{#if loading}
  <p class="flex items-center gap-2 text-muted-foreground"><Loader2 class="h-4 w-4 animate-spin" /> Loading config…</p>
{:else}
  <div class="flex flex-col gap-4">
    {#if configError}<p role="alert" class="text-sm text-destructive">{configError} <button class="underline" onclick={loadConfig}>Retry</button></p>{/if}
    <div class="flex gap-1 overflow-x-auto" style="scrollbar-width:none;">
      {#each configTabs as t}
        <Button
          variant={tab === t.id ? 'default' : 'ghost'}
          size="sm"
          onclick={() => tab = t.id}
          class="flex-shrink-0"
        >
          {t.label}
        </Button>
      {/each}
    </div>

    <!-- Keep independent settings mounted so switching tabs preserves your drafts. -->
    <div hidden={tab !== 'tts'}><TTSSettings onChanged={refreshRuntime} /></div>
    <div hidden={tab !== 'vision'}><VisionSettings onChanged={refreshRuntime} /></div>

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

        <div class="mt-6 border-t pt-4">
          <h4 class="mb-3 text-sm font-semibold text-primary">AirPlay Receivers</h4>
          <p class="mb-3 text-xs text-muted-foreground">
            When enabled, each camera can appear as a separate AirPlay target in the iOS AirPlay picker.
            Audio is decoded and sent to the camera speaker.
          </p>
          <div class="flex flex-wrap items-end gap-4">
            <label class="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                bind:checked={airplayEnabled}
                class="h-4 w-4 cursor-pointer rounded border-input accent-primary"
              />
              Enable AirPlay globally
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Base port
              <Input bind:value={airplayBasePort} type="number" min="1024" max="65535" class="w-24" />
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Prime silence
              <div class="flex items-center gap-1">
                <Input bind:value={airplayPrimeSilenceMs} type="number" min="0" max="5000" class="w-24" />
                <span class="text-xs text-muted-foreground/70">ms</span>
              </div>
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Default icon model
              <Input bind:value={airplayModel} list="airplay-models" placeholder="RealityDevice14,1" class="w-56" />
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              AirPlay gain
              <div class="flex items-center gap-1">
                <Input bind:value={airplayGain} type="number" step="0.1" min="0" max="10" class="w-24" />
                <span class="text-xs text-muted-foreground/70">×</span>
              </div>
            </label>
          </div>
          <p class="mt-2 text-[11px] text-muted-foreground/80">
            The model string is advertised over mDNS and tells iOS which icon to show. Cameras can override the default icon individually.
          </p>
        </div>
      </section>

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
                {#if cam.note}
                  <span class="text-xs text-amber-500" title={cam.note}>⚠ Limited</span>
                {/if}
                <span class="text-sm text-muted-foreground">{cam.ip}</span>
                <span class="text-sm text-muted-foreground" title="Hikvision ISAPI two-way audio channel number; usually 1">ch{cam.channel}</span>
                {#if !cam.enabled}<span class="text-xs text-muted-foreground italic">disabled</span>{/if}
              </div>
              <div class="flex shrink-0 items-center gap-1">
                {#if testStatus[cam.name]}<span class="mr-1 max-w-md text-sm {testStatus[cam.name].startsWith('✓') ? 'text-primary' : 'text-destructive'}" title={testStatus[cam.name]}>{testStatus[cam.name]}</span>{/if}
                <label class="flex items-center gap-1 text-xs text-muted-foreground" title="AirPlay receiver for this camera">
                  <input
                    type="checkbox"
                    checked={cam.airplay_enabled}
                    disabled={airplayToggling[cam.name] || !airplayEnabled}
                    onchange={() => toggleCameraAirPlay(cam)}
                    class="h-4 w-4 cursor-pointer rounded border-input accent-primary disabled:cursor-wait"
                  />
                  AirPlay
                </label>
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
            <span class="flex items-center justify-between">
              Type
              <Button variant="ghost" size="sm" class="h-5 px-1.5 py-0 text-[11px]" onclick={detectCameraType} disabled={detectCamBusy} title="Auto-detect camera type">
                {#if detectCamBusy}<Loader2 class="h-3 w-3 animate-spin mr-1" />{/if}
                <ScanLine class="h-3 w-3 mr-1" />
                Detect
              </Button>
            </span>
            <Select bind:value={camType}>
              <option value="hikvision">hikvision</option>
              <option value="reolink">reolink</option>
              <option value="go2rtc">go2rtc</option>
              <option value="onvif">onvif</option>
            </Select>
            {#if detectCamStatus}<span class="text-[11px] text-primary">{detectCamStatus}</span>{/if}
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
            <span class="flex items-center gap-1">Channel <span class="text-[10px] opacity-60">(ISAPI audio channel, usually 1)</span></span>
            <Input bind:value={camChannel} type="number" min="1" />
          </label>
          {#if camType === 'go2rtc' || camType === 'onvif' || camType === 'reolink'}
          <label class="flex flex-col gap-1 text-xs text-muted-foreground sm:col-span-2">
            {#if camType === 'go2rtc'}
              go2rtc Stream Name
            {:else if camType === 'reolink'}
              go2rtc Stream Name (defaults to camera name)
            {:else}
              RTSP URL
            {/if}
            {#if camType === 'reolink' || camType === 'go2rtc'}
              {#if camStreamCustom}
                <Input bind:value={camStream} placeholder="garage_2way" />
              {:else if go2rtcStreamsLoading}
                <div class="flex items-center gap-2 text-[11px] text-muted-foreground">
                  <Loader2 class="h-3 w-3 animate-spin" /> Loading go2rtc streams…
                </div>
              {:else if go2rtcStreams.length > 0}
                <Select bind:value={camStream} onchange={(e) => { if (e.currentTarget.value === '__custom__') { camStream = ''; camStreamCustom = true } }}>
                  <option value="">— select a stream —</option>
                  {#each go2rtcStreams as s}
                    <option value={s.name}>{s.name}{s.has_backchannel ? ' ✓ backchannel' : ''}</option>
                  {/each}
                  <option value="__custom__">✎ Custom (type manually)…</option>
                </Select>
                {#if camStream && go2rtcStreams.find(s => s.name === camStream) && !go2rtcStreams.find(s => s.name === camStream).has_backchannel}
                  <span class="text-[11px] text-amber-500">
                    ⚠ This stream does not have #backchannel=1 — audio may not work.
                  </span>
                {/if}
              {:else}
                <Input bind:value={camStream} placeholder="garage_2way" />
                {#if go2rtcStreamsError}
                  <span class="text-[11px] text-amber-500">
                    Could not fetch go2rtc streams: {go2rtcStreamsError}. Set the go2rtc URL in Settings.
                  </span>
                {:else}
                  <span class="text-[11px] text-amber-500">
                    No go2rtc streams found. Set the go2rtc URL in Settings or type manually.
                  </span>
                {/if}
              {/if}
              {#if !camStreamCustom && go2rtcStreams.length > 0}
                <button
                  type="button"
                  class="text-[11px] text-primary/70 hover:text-primary text-left"
                  onclick={() => { camStreamCustom = true }}
                >
                  ✎ Type stream name manually instead
                </button>
              {:else if camStreamCustom}
                <button
                  type="button"
                  class="text-[11px] text-primary/70 hover:text-primary text-left"
                  onclick={() => { camStreamCustom = false }}
                >
                  ← Back to dropdown
                </button>
              {/if}
            {:else}
              <Input bind:value={camStream} placeholder="rtsp://user:pass@ip:554/stream0" />
            {/if}
            {#if camType === 'reolink'}
              <span class="text-[11px] text-amber-500">
                Reolink requires a go2rtc stream with #backchannel=1. Add one in your go2rtc config (e.g. reolink_doorbell: rtsp://user:pass@doorbell-ip:554/stream_1#backchannel=1).
              </span>
            {/if}
          </label>
          {/if}
        </div>
        <label class="mt-3 flex flex-col gap-1 text-xs text-muted-foreground">
          Vision Prompt (optional)
          <PromptEditor bind:value={camVisionPrompt}
            placeholder="Describe what you see. Focus on people, vehicles, and animals." />
          <span class="text-[11px] opacity-60">Used when clicking Describe on this camera. Can be overridden per-session.</span>
        </label>
        <div class="mt-3 border-t pt-3">
          <h4 class="mb-2 text-sm font-semibold text-primary">Vision Stream</h4>
          <div class="grid grid-cols-2 gap-2.5 max-sm:grid-cols-1">
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Stream for vision snapshots
              <select
                bind:value={camVisionStream}
                class="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm
                       focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
              >
                <option value="">Frigate detect (default)</option>
                {#each availableStreams as s}
                  <option value={s.name}>{s.name} ({s.video || 'no video'}){!s.active ? ' — inactive' : ''}</option>
                {/each}
              </select>
              <span class="text-[11px] opacity-60">Which go2rtc stream to grab frames from for vision/describe. Substreams are faster (smaller images).</span>
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Max width (px)
              <Input type="number" bind:value={camVisionWidth} placeholder="1280" />
              <span class="text-[11px] opacity-60">0 = no resize. 1280 recommended for vision models.</span>
            </label>
          </div>
          <label class="mt-3 flex flex-col gap-1 text-xs text-muted-foreground">
            Snapshot method
            <select
              bind:value={camSnapMethod}
              class="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm
                     focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            >
              <option value="">auto (ISAPI/Reolink → go2rtc → Frigate)</option>
              <option value="isapi">direct camera API (ISAPI/Reolink)</option>
              <option value="go2rtc">go2rtc</option>
              <option value="frigate">frigate</option>
            </select>
            <span class="text-[11px] opacity-60">Which snapshot method to prefer for this camera. Test it below or benchmark all methods.</span>
          </label>

          <!-- Capture test widget -->
          <div class="mt-3 rounded-lg border border-dashed p-3">
            <span class="text-xs font-semibold text-muted-foreground block mb-2">Capture test</span>
            <CaptureTest camera={camName} stream={camVisionStream} width={camVisionWidth} snapMethod={camSnapMethod} disabled={!camName} />
          </div>
        </div>
        <label class="mt-3 flex items-center gap-2 text-sm text-muted-foreground">
          <input type="checkbox" bind:checked={camEnabled} class="h-4 w-4 cursor-pointer rounded border-input accent-primary" />
          Enabled (camera will receive speak/broadcast)
        </label>
        <div class="mt-4 border-t pt-4">
          <h4 class="mb-2 text-sm font-semibold text-primary">AirPlay for this camera</h4>
          <div class="grid grid-cols-2 gap-2.5 max-sm:grid-cols-1">
            <label class="flex items-center gap-2 text-xs text-muted-foreground col-span-2">
              <input type="checkbox" bind:checked={camAirPlayEnabled} class="h-4 w-4 rounded border-input accent-primary" />
              Enable AirPlay receiver for this camera
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              AirPlay display name
              <Input bind:value={camAirPlayName} placeholder={camName.replace(/[-_]/g, ' ').replace(/\b\w/g, c => c.toUpperCase()) + ' Camera'} />
              <span class="text-[11px] opacity-60">Name shown in the iOS AirPlay picker. Auto-derived from the camera name if left empty.</span>
            </label>
            <label class="flex flex-col gap-1 text-xs text-muted-foreground">
              Device icon model
              <Input bind:value={camAirPlayModel} list="airplay-models" placeholder={airplayModel || 'RealityDevice14,1'} />
              <datalist id="airplay-models">
                <option value="RealityDevice14,1">Vision Pro</option>
                <option value="AppleTV6,2">Apple TV 4K</option>
                <option value="AppleTV3,2">Apple TV HD</option>
                <option value="AudioAccessory5,1">HomePod mini</option>
                <option value="AudioAccessory1,1">HomePod</option>
                <option value="AirPort4,115">AirPort Express</option>
                <option value="MacBookPro18,1">MacBook Pro</option>
              </datalist>
              <span class="text-[11px] opacity-60">Advertised over mDNS; tells iOS which icon to show. Leave empty to use the default above.</span>
            </label>
          </div>
        </div>
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

    <!-- Overview -->
    {:else if tab === 'overview'}
      <section class="rounded-lg border bg-card p-5">
        <h3 class="mb-3 text-base font-semibold text-primary">Runtime Configuration</h3>
        <JsonCode code={JSON.stringify(config, null, 2)} class="max-h-[600px] overflow-auto text-sm" />
      </section>
    {/if}
  </div>
{/if}
