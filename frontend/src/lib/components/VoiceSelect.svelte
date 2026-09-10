<script>
  // Reusable voice dropdown — appears in CameraCard, Broadcast, Library, BroadcastBar.
  // Props: value (bound), voices (string[]), disabled
  let { value = $bindable(), voices = [], disabled = false, class: klass = '', 'aria-label': label = 'Voice' } = $props()
</script>

<select bind:value {disabled} aria-label={label}
  class="rounded-md border border-input bg-transparent px-2 py-1 text-sm disabled:opacity-50 {klass}">
  <option value="">default</option>
  {#each voices as v}
    <option>{v}</option>
  {/each}
</select>
