<script>
  let { presets = [], voices = [], onRefresh } = $props()

  let tab = $state('browse')
  let genName = $state('')
  let genText = $state('')
  let genCategory = $state('alerts')
  let genVoice = $state('')
  let genBusy = $state(false)
  let genStatus = $state('')

  let uploadName = $state('')
  let uploadCategory = $state('uploads')
  let uploadFile = $state(null)
  let uploadBusy = $state(false)
  let uploadStatus = $state('')

  // Group presets by category
  let grouped = $derived(
    presets.reduce((acc, p) => {
      ;(acc[p.category] ??= []).push(p)
      return acc
    }, {})
  )

  async function generate() {
    if (!genName || !genText) return
    genBusy = true; genStatus = ''
    try {
      const res = await fetch('/api/library', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: genName, text: genText, category: genCategory, voice: genVoice }),
      })
      if (!res.ok) throw new Error(await res.text())
      genStatus = '✓ Saved'
      genName = ''; genText = ''
      onRefresh()
    } catch (e) {
      genStatus = '✗ ' + e.message
    } finally {
      genBusy = false
      setTimeout(() => (genStatus = ''), 4000)
    }
  }

  async function upload() {
    if (!uploadName || !uploadFile) return
    uploadBusy = true; uploadStatus = ''
    try {
      const fd = new FormData()
      fd.append('name', uploadName)
      fd.append('category', uploadCategory)
      fd.append('file', uploadFile)
      const res = await fetch('/api/library/upload', { method: 'POST', body: fd })
      if (!res.ok) throw new Error(await res.text())
      uploadStatus = '✓ Uploaded'
      uploadName = ''; uploadFile = null
      onRefresh()
    } catch (e) {
      uploadStatus = '✗ ' + e.message
    } finally {
      uploadBusy = false
      setTimeout(() => (uploadStatus = ''), 4000)
    }
  }

  async function deletePreset(category, name) {
    if (!confirm(`Delete ${category}/${name}?`)) return
    await fetch(`/api/library/${category}/${name}`, { method: 'DELETE' })
    onRefresh()
  }

  function preview(category, name) {
    const a = document.createElement('a')
    a.href = `/api/library/${category}/${name}/preview`
    a.target = '_blank'
    a.click()
  }
</script>

<div class="library">
  <div class="lib-tabs">
    <button class:active={tab === 'browse'} onclick={() => tab = 'browse'}>Browse</button>
    <button class:active={tab === 'generate'} onclick={() => tab = 'generate'}>Generate TTS</button>
    <button class:active={tab === 'upload'} onclick={() => tab = 'upload'}>Upload</button>
  </div>

  {#if tab === 'browse'}
    {#if presets.length === 0}
      <p class="empty">No presets yet. Generate or upload one.</p>
    {:else}
      {#each Object.entries(grouped) as [cat, items]}
        <div class="category">
          <h3>{cat}</h3>
          <div class="preset-list">
            {#each items as p}
              <div class="preset">
                <div class="preset-info">
                  <span class="pname">{p.name}</span>
                  <span class="dur">{p.duration?.toFixed(1)}s</span>
                  {#if p.text}<span class="ptext">"{p.text}"</span>{/if}
                </div>
                <div class="preset-actions">
                  <button class="icon-btn" onclick={() => preview(p.category, p.name)} title="Preview">▶</button>
                  <button class="icon-btn del" onclick={() => deletePreset(p.category, p.name)} title="Delete">✕</button>
                </div>
              </div>
            {/each}
          </div>
        </div>
      {/each}
    {/if}

  {:else if tab === 'generate'}
    <div class="form">
      <h3>Generate TTS Preset</h3>
      <label>
        Name
        <input bind:value={genName} placeholder="e.g. person_detected" />
      </label>
      <label>
        Category
        <input bind:value={genCategory} placeholder="alerts" />
      </label>
      <label>
        Text
        <textarea bind:value={genText} rows="3" placeholder="Text to synthesize..."></textarea>
      </label>
      <label>
        Voice
        <select bind:value={genVoice}>
          <option value="">default</option>
          {#each voices as v}
            <option>{v}</option>
          {/each}
        </select>
      </label>
      <button onclick={generate} disabled={genBusy || !genName || !genText}>
        {genBusy ? 'Generating…' : '✨ Generate & Save'}
      </button>
      {#if genStatus}<p class="status">{genStatus}</p>{/if}
    </div>

  {:else}
    <div class="form">
      <h3>Upload Audio File</h3>
      <p class="hint">Any format — ffmpeg will convert to G.711ulaw 8kHz</p>
      <label>
        Name
        <input bind:value={uploadName} placeholder="preset name" />
      </label>
      <label>
        Category
        <input bind:value={uploadCategory} placeholder="uploads" />
      </label>
      <label>
        File
        <input type="file" accept="audio/*" onchange={e => uploadFile = e.target.files[0]} />
      </label>
      <button onclick={upload} disabled={uploadBusy || !uploadName || !uploadFile}>
        {uploadBusy ? 'Uploading…' : '⬆ Upload'}
      </button>
      {#if uploadStatus}<p class="status">{uploadStatus}</p>{/if}
    </div>
  {/if}
</div>

<style>
  .library { display: flex; flex-direction: column; gap: 1rem; }

  .lib-tabs { display: flex; gap: 0.25rem; }
  .lib-tabs button {
    padding: 0.4rem 1rem;
    border: 1px solid #3a3a50;
    border-radius: 6px;
    background: transparent;
    color: #888;
    font-size: 0.9rem;
  }
  .lib-tabs button:hover { border-color: #7c3aed; color: #ccc; }
  .lib-tabs button.active { background: #4c1d95; border-color: #7c3aed; color: #e8e8f0; }

  .category { margin-bottom: 1rem; }
  h3 { font-size: 0.85rem; color: #888; text-transform: uppercase; letter-spacing: 0.08em; margin-bottom: 0.5rem; }

  .preset-list { display: flex; flex-direction: column; gap: 0.4rem; }
  .preset {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.5rem 0.75rem;
    background: #1a1a24;
    border: 1px solid #2a2a3a;
    border-radius: 8px;
  }
  .preset-info { display: flex; align-items: center; gap: 0.6rem; flex: 1; min-width: 0; }
  .pname { font-weight: 600; white-space: nowrap; }
  .dur { font-size: 0.75rem; color: #888; white-space: nowrap; }
  .ptext { font-size: 0.8rem; color: #666; font-style: italic; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .preset-actions { display: flex; gap: 0.3rem; flex-shrink: 0; }

  .icon-btn {
    padding: 0.2rem 0.5rem;
    background: transparent;
    border: 1px solid #3a3a50;
    border-radius: 5px;
    color: #888;
    font-size: 0.8rem;
  }
  .icon-btn:hover { border-color: #7c3aed; color: #ccc; }
  .icon-btn.del:hover { border-color: #ef4444; color: #ef4444; }

  .form {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    max-width: 480px;
  }
  .hint { font-size: 0.8rem; color: #666; }
  label { display: flex; flex-direction: column; gap: 0.3rem; font-size: 0.85rem; color: #aaa; }
  input, textarea, select {
    padding: 0.4rem 0.6rem;
    background: #1a1a24;
    border: 1px solid #3a3a50;
    border-radius: 6px;
    color: #e8e8f0;
    font-size: 0.9rem;
    font-family: inherit;
  }
  input:focus, textarea:focus, select:focus { outline: none; border-color: #7c3aed; }
  input[type="file"] { background: transparent; border: 1px dashed #3a3a50; padding: 0.5rem; }
  textarea { resize: vertical; }
  .form button {
    align-self: flex-start;
    padding: 0.45rem 1.1rem;
    background: #7c3aed;
    border: none;
    border-radius: 7px;
    color: #fff;
    font-size: 0.9rem;
  }
  .form button:disabled { opacity: 0.4; }
  .status { font-size: 0.85rem; color: #a78bfa; }
  .empty { color: #555; font-style: italic; }
</style>
