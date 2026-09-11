<script>
  // Reusable camera selector dropdown.
  // Props: value (bound), cameras (Camera[] or string[]), disabled, placeholder, enabledOnly
  let {
    value = $bindable(),
    cameras = [],
    disabled = false,
    placeholder = '— select —',
    enabledOnly = false,
    showDisabledLabel = false,
    class: klass = '',
  } = $props()

  // Normalize: accept either string[] or {name, enabled}[]
  let names = $derived(
    cameras.map(c => (typeof c === 'string' ? c : c.name))
      .filter((n, i, arr) => {
        if (enabledOnly) {
          const cam = cameras[i]
          if (typeof cam === 'object' && cam.enabled === false) return false
        }
        return arr.indexOf(n) === i
      })
  )
</script>

<select bind:value {disabled}
  class="rounded-md border border-input bg-transparent px-3 py-2 text-sm disabled:opacity-50 {klass}">
  <option value="">{placeholder}</option>
  {#each names as name}
    <option value={name}>{name}{showDisabledLabel && cameras.some(c => typeof c === 'object' && c.name === name && c.enabled === false) ? ' (Playback disabled)' : ''}</option>
  {/each}
</select>
