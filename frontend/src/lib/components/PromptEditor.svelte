<script>
  // Reusable prompt editor with optional preset support.
  // Props: value (bound), disabled, placeholder, presets (optional VisionPrompt[]),
  //        showPresetButtons (bool), onSavePreset (callback), onResetGlobal (callback)
  import { Button } from '$lib/components/ui/button'
  import { Textarea } from '$lib/components/ui/textarea'

  let {
    value = $bindable(),
    disabled = false,
    placeholder = 'Describe what you see...',
    presets = [],
    showPresetButtons = false,
    onSavePreset = null,
    onResetGlobal = null,
    class: klass = '',
  } = $props()

  let showSavePreset = $state(false)
  let presetName = $state('')

  function savePreset() {
    if (!presetName.trim() || !onSavePreset) return
    onSavePreset(presetName.trim(), value)
    presetName = ''
    showSavePreset = false
  }
</script>

<div class="flex flex-col gap-2 {klass}">
  {#if showPresetButtons && (presets.length > 0 || onSavePreset)}
    <div class="flex items-center justify-between gap-2 flex-wrap">
      <label class="text-xs font-semibold text-muted-foreground">Prompt</label>
      <div class="flex gap-1.5">
        {#if onResetGlobal}
          <Button size="sm" variant="ghost" onclick={onResetGlobal} disabled={disabled}>Reset to default</Button>
        {/if}
        {#if onSavePreset}
          <Button size="sm" variant="ghost" onclick={() => showSavePreset = !showSavePreset} disabled={disabled}>Save as Preset</Button>
        {/if}
      </div>
    </div>
  {/if}

  {#if showPresetButtons && presets.length > 0}
    <div class="flex flex-wrap gap-1">
      {#each presets as p}
        <button
          class="rounded-full border px-2.5 py-0.5 text-xs hover:border-primary hover:text-primary transition-colors"
          onclick={() => value = p.prompt}
          disabled={disabled}
          title={p.description}
        >{p.name}</button>
      {/each}
    </div>
  {/if}

  {#if showSavePreset}
    <div class="flex gap-2">
      <input
        bind:value={presetName}
        placeholder="Preset name..."
        class="flex h-8 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm"
      />
      <Button size="sm" onclick={savePreset} disabled={!presetName.trim()}>Save</Button>
    </div>
  {/if}

  <Textarea bind:value rows={3} {placeholder} {disabled} class="text-sm" />
</div>
