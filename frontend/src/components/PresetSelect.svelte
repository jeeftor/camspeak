<script lang="ts">
  import type { Preset } from '$lib/types'

  let { value = $bindable(''), presets = [], disabled = false, label = 'Preset', placeholder = 'Select a preset' }: {
    value?: string
    presets?: Preset[]
    disabled?: boolean
    label?: string
    placeholder?: string
  } = $props()

  const grouped = $derived(presets.reduce<Record<string, Preset[]>>((groups, preset) => {
    ;(groups[preset.category] ??= []).push(preset)
    return groups
  }, {}))
</script>

<label class="flex flex-col gap-1.5 text-sm">
  {label}
  <select bind:value {disabled} class="min-w-0 rounded-md border border-input bg-background px-3 py-2 disabled:opacity-50">
    <option value="">{placeholder}</option>
    {#each Object.entries(grouped) as [category, items]}
      <optgroup label={category}>
        {#each items as preset (preset.name)}
          <option value={JSON.stringify([category, preset.name])}>{preset.name}</option>
        {/each}
      </optgroup>
    {/each}
  </select>
</label>
