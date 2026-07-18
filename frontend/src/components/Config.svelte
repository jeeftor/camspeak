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

  // Test status
  let testStatus = $state({})
  let configError = $state('')

  async function loadConfig() {
    loading = true
    try {
      const [cfgRes, ttsRes, camRes, voiceRes, visionRes] = await Promise.all([
        fetch('/api/config'),
        fetch('/api/config/tts'),
        fetch('/api/config/cameras'),
        fetch('/api/voices'),
        fetch('/api/config/vision'),
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
    camChannel = cam.channel || 1
    camStream = cam.stream || ''
    camEnabled = cam.enabled ?? false
    camVisionPrompt = cam.vision_prompt ?? ''
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

  const configTabs = [
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
                <Button variant="outline" size="sm" class="h-7 px-2" onclick={() => editCamera(cam)} title="Edit" aria-label="Edit camera"><Pencil class="h-4 w-4" /></Button>
                <Button variant="outline" size="sm" class="h-7 px-2 hover:border-destructive hover:text-destructive" onclick={() => deleteCamera(cam.name)} title="Delete" aria-label="Delete camera"><X class="h-4 w-4" /></Button>
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
                <option value="go2rtc">go2rtc</option>
                <option value="onvif">onvif</option>
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
          <Button onclick={saveCamera} disabled={!camName || !camIP} class="mt-3">
            Save Camera
          </Button>
          {#if camStatus}<span class="ml-2 text-sm text-primary">{camStatus}</span>{/if}
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
            <Input bind:value={visionURL} placeholder="http://192.168.1.91:8080/v1/chat/completions" />
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

    <!-- Overview -->
    {:else if tab === 'overview'}
      <section class="rounded-lg border bg-card p-5">
        <h3 class="mb-3 text-base font-semibold text-primary">Runtime Configuration</h3>
        <JsonCode code={JSON.stringify(config, null, 2)} class="max-h-[600px] overflow-auto text-sm" />
      </section>
    {/if}
  </div>
{/if}
