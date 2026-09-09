<script>
  // Reusable prompt editor with optional preset support.
  // Props: value (bound), disabled, placeholder, presets (optional VisionPrompt[]),
  //        showPresetButtons (bool), onSavePreset (callback), onResetGlobal (callback),
  //        onSetGlobal (callback — shows "Set as Global Default" button),
  //        onDeletePreset (callback(name) — shows delete button on each preset)
  import { Bookmark, Save, Trash2 } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Textarea } from '$lib/components/ui/textarea'
  import { Input } from '$lib/components/ui/input'

  let {
    value = $bindable(),
    disabled = false,
    placeholder = 'Describe what you see...',
    presets = [],
    showPresetButtons = false,
    onSavePreset = null,
    onResetGlobal = null,
    onSetGlobal = null,
    onDeletePreset = null,
    globalPrompt = null,
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
  {#if showPresetButtons && (presets.length > 0 || onSavePreset || onResetGlobal || onSetGlobal)}
    <div class="flex items-center justify-between gap-2 flex-wrap">
      <label class="text-xs font-semibold text-muted-foreground">Prompt</label>
      <div class="flex gap-1.5">
        {#if onResetGlobal && globalPrompt && value !== globalPrompt}
          <Button size="sm" variant="ghost" onclick={onResetGlobal} disabled={disabled}>Reset to default</Button>
        {/if}
        {#if onSavePreset}
          <Button size="sm" variant="ghost" onclick={() => showSavePreset = !showSavePreset} disabled={disabled || !value}>
            <Bookmark class="h-3.5 w-3.5" />
            Save as Preset
          </Button>
        {/if}
        {#if onSetGlobal}
          <Button size="sm" variant="ghost" onclick={() => onSetGlobal(value)} disabled={disabled || !value}>
            <Save class="h-3.5 w-3.5" />
            Set as Global Default
          </Button>
        {/if}
      </div>
    </div>
  {/if}

  {#if showPresetButtons && presets.length > 0}
    <div class="flex flex-wrap items-center gap-2">
      <span class="text-xs font-semibold text-muted-foreground">Presets:</span>
      {#each presets as p (p.name)}
        <div class="flex items-center gap-0.5 rounded-md border bg-card text-xs">
          <button
            onclick={() => value = p.prompt}
            disabled={disabled}
            class="px-2 py-1 hover:bg-accent rounded-l-md disabled:opacity-50"
            title={p.prompt}
          >{p.name}</button>
          {#if onDeletePreset}
            <button
              onclick={() => onDeletePreset(p.name)}
              disabled={disabled}
              class="px-1 py-1 hover:bg-destructive/10 rounded-r-md disabled:opacity-50"
            >
              <Trash2 class="h-3 w-3" />
            </button>
          {:else}
            <span class="pr-1"></span>
          {/if}
        </div>
      {/each}
    </div>
  {/if}

  {#if showSavePreset}
    <div class="flex gap-2 items-center">
      <Input bind:value={presetName} placeholder="Preset name…" class="max-w-[200px] text-sm" />
      <Button size="sm" onclick={savePreset} disabled={!presetName || !value}>Save</Button>
      <Button variant="ghost" size="sm" onclick={() => showSavePreset = false}>Cancel</Button>
    </div>
  {/if}

  <Textarea bind:value rows={3} {placeholder} {disabled} class="text-sm" />
</div>
