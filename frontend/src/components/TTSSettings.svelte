<script>
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
  let ttsStatus = $state('')
  let ttsTestBusy = $state(false)
  let ttsTestStatus = $state('')


  async function loadTTS() {
    error = ''
    try {
      const data = await apiClient.listTTSPresets()
      ttsPresets = data.presets ?? []
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
        default_voice: ttsVoice, api_key: ttsKey || undefined, clear_api_key: ttsClearKey, description: ttsDesc,
      })
      ttsStatus = '✓ Saved'
      toast.success(`TTS preset "${ttsName}" saved`)
      ttsFormOpen = false
      ttsName = ''; ttsEndpoint = ''; ttsModel = ''; ttsVoice = ''; ttsKey = ''; ttsDesc = ''
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
      const data = await apiClient.testTTSConfig(activePreset?.endpoint ?? ttsEndpoint, activePreset?.api_key ?? ttsKey)
      connectionStatus = data.ok ? '✓ Connected' : '✗ ' + (data.message ?? 'failed')
    } catch (e) {
      connectionStatus = '✗ ' + e.message
    }
  }

  function openAddTTS() {
    ttsHasKey = false; ttsClearKey = false
    ttsName = ''; ttsEndpoint = ''; ttsModel = ''; ttsVoice = ''; ttsKey = ''; ttsDesc = ''
    ttsStatus = ''
    ttsFormOpen = true
  }

  function editTTS(p) {
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
    try {
      const data = await apiClient.testTTSConfig(ttsEndpoint, ttsKey)
      ttsTestStatus = data.ok ? '✓ Connected' : '✗ ' + data.message
    } catch (e) {
      ttsTestStatus = '✗ ' + e.message
    } finally {
      ttsTestBusy = false
      testTimer = setTimeout(() => ttsTestStatus = '', 6000)
    }
  }

</script>

{#if error}<p role="alert" class="mb-3 text-sm text-destructive">{error} <button class="underline" onclick={loadTTS}>Retry</button></p>{/if}
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
      <TTSBenchmark presets={ttsPresets} />

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
        </div>
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
