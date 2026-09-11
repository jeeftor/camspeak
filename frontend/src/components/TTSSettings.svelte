<script>
  import ErrorNotice from '$lib/components/ErrorNotice.svelte'
  import { onMount, onDestroy } from 'svelte'
  import { Pencil, X, Check } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { Badge } from '$lib/components/ui/badge'
  import Modal from '$lib/components/Modal.svelte'
  import VoiceSelect from '$lib/components/VoiceSelect.svelte'
  import { toast } from '$lib/components/ui/toast'
  import { apiClient } from '$lib/api'
  import TTSBenchmark from './TTSBenchmark.svelte'

  let { onChanged } = $props()
  let ttsPresets = $state([])
  let voices = $state([])
  let loading = $state(true)
  let saving = $state(false)
  let error = $state('')
  let connectionStatus = $state('')
  let ttsFormOpen = $state(false)
  let saveTimer
  let testTimer
  // TTS form
  let ttsName = $state('')
  let ttsEndpoint = $state('')
  let ttsModel = $state('')
  let ttsVoice = $state('')
  let ttsKey = $state('')
  let ttsHasKey = $state(false)
  let ttsClearKey = $state(false)
  let ttsDesc = $state('')
  let ttsStreaming = $state(false)
  let ttsRate = $state(24000)
  let ttsChannels = $state(1)
  let ttsStatus = $state('')
  let ttsTestBusy = $state(false)
  let ttsTestStatus = $state('')
  let models = $state([])
  let runtimeModel = $state('')
  $effect(() => { ttsEndpoint; ttsKey; models = []; ttsTestStatus = '' })
  const savedTestPreset = $derived(ttsPresets.find(p => p.name === ttsName && p.endpoint === ttsEndpoint && p.model === ttsModel && p.default_voice === ttsVoice && !ttsKey && !ttsClearKey))


  async function loadTTS() {
    error = ''
    try {
      const data = await apiClient.listTTSPresets()
      ttsPresets = data.presets ?? []
      apiClient.getConfig().then(config => runtimeModel = config.tts.model).catch(() => {})
      apiClient.getVoices().then(value => voices = value ?? []).catch(() => {})
    } catch (cause) {
      error = 'Your TTS presets could not be loaded: ' + cause.message
    } finally { loading = false }
  }

  onMount(loadTTS)
  onDestroy(() => { clearTimeout(saveTimer); clearTimeout(testTimer) })

  // --- TTS Presets ---
  async function saveTTS() {
    if (!ttsName || !ttsEndpoint || saving) return
    saving = true
    clearTimeout(saveTimer)
    ttsStatus = ''
    try {
      await apiClient.saveTTSPreset({
        name: ttsName, endpoint: ttsEndpoint, model: ttsModel,
        streaming: ttsStreaming, pcm_sample_rate: Number(ttsRate), pcm_channels: Number(ttsChannels),
        default_voice: ttsVoice, api_key: ttsKey || undefined, clear_api_key: ttsClearKey, description: ttsDesc,
      })
      ttsStatus = '✓ Saved'
      toast.success(`TTS preset "${ttsName}" saved`)
      ttsKey = ''; ttsClearKey = false
      await loadTTS()
      onChanged?.()
    } catch (e) {
      ttsStatus = '✗ ' + e.message
    } finally {
      saving = false
      saveTimer = setTimeout(() => ttsStatus = '', 4000)
    }
  }

  async function activateTTS(name) {
    try {
      await apiClient.activateTTSPreset(name)
      await loadTTS()
      onChanged?.()
    } catch (e) {
      error = '✗ ' + e.message
    }
  }

  async function deleteTTS(name) {
    if (!confirm(`Delete TTS preset "${name}"?`)) return
    try {
      await apiClient.deleteTTSPreset(name)
      await loadTTS()
      onChanged?.()
    } catch (e) {
      error = '✗ ' + e.message
    }
  }

  async function testTTS() {
    connectionStatus = 'Testing…'
    try {
      const activePreset = ttsPresets.find(p => p.is_active) ?? ttsPresets[0]
      const data = await apiClient.testTTSConfig(activePreset?.endpoint ?? ttsEndpoint, activePreset?.api_key ?? ttsKey, activePreset?.model ?? ttsModel)
      connectionStatus = data.ok ? data.message : ''
      if (!data.ok) toast.error(data.message)
    } catch (e) {
      connectionStatus = '✗ ' + e.message
    }
  }

  function openAddTTS() {
    models = []; ttsTestStatus = ''
    ttsStreaming = false; ttsRate = 24000; ttsChannels = 1
    ttsHasKey = false; ttsClearKey = false
    ttsName = ''; ttsEndpoint = ''; ttsModel = ''; ttsVoice = ''; ttsKey = ''; ttsDesc = ''
    ttsStatus = ''
    ttsFormOpen = true
  }

  function editTTS(p) {
    models = []; ttsTestStatus = ''
    ttsStreaming = p.streaming ?? false
    ttsRate = p.pcm_sample_rate || 24000
    ttsChannels = p.pcm_channels || 1
    ttsHasKey = p.has_api_key ?? false
    ttsClearKey = false
    ttsName = p.name
    ttsEndpoint = p.endpoint
    ttsModel = p.model
    ttsVoice = p.default_voice
    ttsKey = ''
    ttsDesc = p.description
    ttsStatus = ''
    ttsFormOpen = true
  }

  async function testTTSModal() {
    ttsTestBusy = true
    clearTimeout(testTimer)
    ttsTestStatus = ''
    const endpoint = ttsEndpoint
    const model = ttsModel
    const key = ttsKey
    try {
      const data = await apiClient.testTTSConfig(endpoint, key, model)
      if (endpoint !== ttsEndpoint || key !== ttsKey || model !== ttsModel || !ttsFormOpen) return
      models = data.models ?? []
      ttsTestStatus = data.ok ? data.message : ''
      if (!data.ok) toast.error(data.message)
    } catch (e) {
      toast.error(e.message)
    } finally {
      ttsTestBusy = false
      testTimer = setTimeout(() => ttsTestStatus = '', 6000)
    }
  }

</script>

{#if error}<ErrorNotice message={error} /><button class="underline" onclick={loadTTS}>Retry loading TTS presets</button>{/if}
{#if loading}
  <p class="text-sm text-muted-foreground">Loading your TTS presets…</p>
{:else}
      <section class="rounded-lg border bg-card p-5">
        <div class="mb-3 flex flex-wrap items-center justify-between gap-3">
          <h3 class="text-base font-semibold text-primary">TTS Presets</h3>
          <div class="flex items-center gap-2">
            {#if connectionStatus}<span class="text-sm text-primary">{connectionStatus}</span>{/if}
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
      <p class="mt-3 text-sm text-muted-foreground">Edit a preset to compare buffered/streaming audio and choose its playback mode.</p>
      {#if runtimeModel}<p class="mt-2 text-sm text-muted-foreground">Effective runtime model: <code>{runtimeModel}</code>. Environment variables override the active preset. If this differs from your saved model, check CAMSPEAK_TTS_MODEL and restart the server after changing it.</p>{/if}

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
            <Input bind:value={ttsModel} placeholder="Exact server model ID (Lemonade: kokoro-v1)" list="tts-server-models" />
            <datalist id="tts-server-models">{#each models as model}<option value={model}></option>{/each}</datalist>
            <span>Test connection to fetch model IDs. Select explicitly; listed models may not all support speech.</span>
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Default Voice
            <VoiceSelect bind:value={ttsVoice} {voices} />
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            API Key (optional)
            <Input bind:value={ttsKey} type="password" disabled={ttsClearKey} placeholder={ttsHasKey ? 'Stored key — leave empty to keep' : 'sk-...'} />
          </label>
          {#if ttsHasKey}
            <label class="flex items-center gap-2 text-xs"><input type="checkbox" bind:checked={ttsClearKey} />Remove the stored API key on save</label>
          {/if}
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Description
            <Input bind:value={ttsDesc} placeholder="Local Lemonade instance" />
          </label>
          <label class="flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={ttsStreaming} />Experimental streaming playback</label>
          <p class="text-xs text-muted-foreground sm:col-span-2">Opt in for Hikvision Speak and Describe. Other paths remain buffered. Compare audio below; failed streams are not automatically replayed.</p>
          {#if ttsStreaming}
            <p class="text-xs text-warning">Match your server's raw signed 16-bit little-endian PCM output. This is not the camera rate; CamSpeak converts to 8 kHz μ-law. Streaming timings measure readiness of each stage; playback includes remaining generation.</p>
          {/if}
        </div>
        {#if savedTestPreset}
          <TTSBenchmark presets={[savedTestPreset]} initialRate={Number(ttsRate)} initialChannels={Number(ttsChannels)} onFormatChange={(rate, channels) => { ttsRate = rate; ttsChannels = channels }} />
          <p class="text-xs text-muted-foreground">The comparison's PCM format is also used for streaming playback after Save Preset.</p>
        {:else}
          <p class="mt-3 text-xs text-warning">Save the endpoint, model and voice before testing. The comparison uses your saved preset and credentials.</p>
        {/if}
        <div class="mt-4 flex flex-wrap items-center gap-2 border-t pt-4">
          <Button onclick={saveTTS} disabled={saving || !ttsName || !ttsEndpoint}>{saving ? 'Saving…' : 'Save Preset'}</Button>
          <Button variant="outline" onclick={testTTSModal} disabled={ttsTestBusy || !ttsEndpoint}>
            {ttsTestBusy ? 'Testing…' : 'Test Connection'}
          </Button>
          <Button variant="ghost" onclick={() => ttsFormOpen = false}>Cancel</Button>
          {#if ttsTestStatus}<span class="text-sm {ttsTestStatus.startsWith('✓') ? 'text-primary' : 'text-destructive'}">{ttsTestStatus}</span>{/if}
          {#if ttsStatus}<span class="text-sm text-primary">{ttsStatus}</span>{/if}
        </div>
      </Modal>

{/if}
