<script>
  import CameraCard from './CameraCard.svelte'

  let { cameras = [], voices = [], presets = [], onRefresh } = $props()
</script>

<div class="grid-header">
  <h2>Cameras</h2>
  <button class="refresh" onclick={onRefresh}>↻ Refresh</button>
</div>

{#if cameras.length === 0}
  <p class="empty">No cameras configured.</p>
{:else}
  <div class="grid">
    {#each cameras as cam (cam.name)}
      <CameraCard camera={cam} {voices} {presets} />
    {/each}
  </div>
{/if}

<style>
  .grid-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 1rem; }
  h2 { font-size: 1.1rem; color: #a78bfa; }
  .refresh {
    padding: 0.3rem 0.7rem;
    background: transparent;
    border: 1px solid #3a3a50;
    border-radius: 6px;
    color: #888;
    font-size: 0.85rem;
  }
  .refresh:hover { border-color: #7c3aed; color: #ccc; }
  .grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(300px, 1fr)); gap: 1rem; }
  .empty { color: #555; font-style: italic; }
</style>
