<script>
  import { onMount, onDestroy } from 'svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import PromptEditor from '$lib/components/PromptEditor.svelte'
  import { toast } from '$lib/components/ui/toast'
  import { apiClient } from '$lib/api'
  import { isVisionCapableModel } from '$lib/models'

  let { onChanged } = $props()
  let loading = $state(true)
  let saving = $state(false)
  let error = $state('')
  let saveTimer
  let testTimer
  // Vision form
  let visionURL = $state('')
  let visionModel = $state('')
  let visionAPIKey = $state('')
  let visionHasKey = $state(false)
  let visionClearKey = $state(false)
  let visionPrompt = $state('')
  let visionStatus = $state('')
  let visionTestStatus = $state('')
  let visionTestBusy = $state(false)
  let visionModelList = $state([]) // models fetched from endpoint
  let visionModelPickerOpen = $state(false)


  async function loadVision() {
    error = ''
    try {
      const config = await apiClient.getVisionConfig()
      visionURL = config.url ?? ''
      visionModel = config.model ?? ''
      visionAPIKey = ''
      visionHasKey = config.has_api_key ?? false
      visionClearKey = false
      visionPrompt = config.prompt ?? ''
    } catch (cause) {
      error = 'Your vision settings could not be loaded: ' + cause.message
    } finally { loading = false }
  }

  onMount(loadVision)
  onDestroy(() => { clearTimeout(saveTimer); clearTimeout(testTimer) })

  // --- Vision ---
  async function saveVision() {
    if (saving) return
    saving = true
    clearTimeout(saveTimer)
    visionStatus = ''
    try {
      await apiClient.saveVisionConfig({
        url: visionURL,
        model: visionModel,
        api_key: visionAPIKey || undefined,
        clear_api_key: visionClearKey,
        prompt: visionPrompt,
      })
      visionStatus = '✓ Saved'
      toast.success('Vision config saved')
      await loadVision()
      onChanged?.()
    } catch (e) {
      visionStatus = '✗ ' + e.message
      toast.error(`Failed to save vision config: ${e.message}`)
    } finally {
      saving = false
      saveTimer = setTimeout(() => visionStatus = '', 4000)
    }
  }

  async function testVision() {
    visionTestBusy = true
    clearTimeout(testTimer)
    visionTestStatus = ''
    visionModelList = []
    visionModelPickerOpen = false
    try {
      const data = await apiClient.testVisionConfig(visionURL, visionAPIKey)
      if (data.ok) {
        visionTestStatus = `✓ Connected (${data.models} model${data.models === 1 ? '' : 's'})`
        const ids = (data.data?.data?.map(m => m.id).filter(Boolean) ?? []).filter(isVisionCapableModel)
        if (ids.length > 0) {
          visionModelList = ids
          visionModelPickerOpen = true
        }
      } else {
        visionTestStatus = '✗ ' + data.message
      }
    } catch (e) {
      visionTestStatus = '✗ ' + e.message
    } finally {
      visionTestBusy = false
      testTimer = setTimeout(() => visionTestStatus = '', 8000)
    }
  }

</script>

{#if error}<p role="alert" class="mb-3 text-sm text-destructive">{error} <button class="underline" onclick={loadVision}>Retry</button></p>{/if}
{#if loading}
  <p class="text-sm text-muted-foreground">Loading your vision settings…</p>
{:else}
      <section class="rounded-lg border bg-card p-5">
        <div class="mb-4 flex flex-wrap items-center justify-between gap-4">
          <h3 class="text-base font-semibold text-primary">Vision Model</h3>
          <div class="flex items-center gap-2">
            {#if visionTestStatus}<span class="text-sm text-primary">{visionTestStatus}</span>{/if}
            {#if visionStatus}<span class="text-sm text-primary">{visionStatus}</span>{/if}
            <Button variant="outline" size="sm" onclick={testVision} disabled={visionTestBusy}>
              {visionTestBusy ? 'Testing…' : 'Test'}
            </Button>
            <Button onclick={saveVision} size="sm" disabled={saving}>{saving ? 'Saving…' : 'Save'}</Button>
          </div>
        </div>
        <p class="mb-4 text-sm text-muted-foreground">
          OpenAI-compatible vision endpoint for the Describe and Vision endpoints.
          The default prompt is used when neither the request nor the camera specifies one.
        </p>
        <div class="grid grid-cols-2 gap-2.5 max-sm:grid-cols-1">
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Endpoint URL <span class="text-muted-foreground/60 font-normal">(base URL or full /v1/chat/completions path)</span>
            <Input bind:value={visionURL} placeholder="http://10.0.0.x:8080" />
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Model
            {#if visionModelPickerOpen && visionModelList.length > 0}
              <select
                class="rounded-md border border-input bg-transparent px-3 py-2 text-sm"
                onchange={(e) => { visionModel = e.target.value; visionModelPickerOpen = false }}
              >
                <option value="">— pick a model —</option>
                {#each visionModelList as m}
                  <option value={m} selected={m === visionModel}>{m}</option>
                {/each}
              </select>
            {:else}
              <div class="flex gap-1.5">
                <Input bind:value={visionModel} placeholder="llama3.2-vision" class="flex-1" />
                {#if visionModelList.length > 0}
                  <button
                    onclick={() => visionModelPickerOpen = true}
                    class="rounded-md border border-input px-2 text-xs text-muted-foreground hover:bg-accent"
                    title="Pick from scanned models"
                  >↓</button>
                {/if}
              </div>
            {/if}
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground sm:col-span-2">
            API Key (optional)
            <Input bind:value={visionAPIKey} type="password" disabled={visionClearKey} placeholder={visionHasKey ? 'Stored key — leave empty to keep' : 'sk-...'} />
          </label>
          {#if visionHasKey}
            <label class="flex items-center gap-2 text-xs sm:col-span-2"><input type="checkbox" bind:checked={visionClearKey} />Remove the stored API key on save</label>
          {/if}
        </div>
        <label class="flex flex-col gap-1 text-xs text-muted-foreground mt-3">
          Default Vision Prompt
          <PromptEditor bind:value={visionPrompt}
            placeholder="Describe what you see in one or two sentences. Be concise and factual." />
          <span class="text-[11px] opacity-60">
            Fallback chain: request prompt → camera's vision_prompt → this global default → hardcoded default.
            Leave empty to use the hardcoded default.
          </span>
        </label>
      </section>

{/if}
