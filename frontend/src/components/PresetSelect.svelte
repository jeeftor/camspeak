<script lang="ts">
  import type { Preset } from '$lib/types'

  let { value = $bindable(''), presets = [], disabled = false }: {
    value?: string
    presets?: Preset[]
    disabled?: boolean
  } = $props()

  const grouped = $derived(presets.reduce<Record<string, Preset[]>>((groups, preset) => {
    ;(groups[preset.category] ??= []).push(preset)
    return groups
  }, {}))
</script>

<label class="flex flex-col gap-1.5 text-sm">
  Preset
  <select bind:value {disabled} class="min-w-0 rounded-md border border-input bg-background px-3 py-2 disabled:opacity-50">
    <option value="">Select a preset</option>
    {#each Object.entries(grouped) as [category, items]}
      <optgroup label={category}>
        {#each items as preset (preset.name)}
          <option value={JSON.stringify([category, preset.name])}>{preset.name}</option>
        {/each}
      </optgroup>
    {/each}
  </select>
</label>
