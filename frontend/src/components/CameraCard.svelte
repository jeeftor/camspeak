<script>
  let { camera, voices = [], presets = [] } = $props()

  let text = $state('')
  let voice = $state('')
  let preset = $state('')
  let busy = $state(false)
  let status = $state('')

  async function post(path, body) {
    const res = await fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    if (!res.ok) throw new Error(await res.text())
  }

  async function speak() {
    if (!text) return
    busy = true; status = ''
    try {
      await post('/api/speak', { camera: camera.name, text, voice })
      status = '✓'
      text = ''
    } catch (e) {
      status = '✗ ' + e.message
    } finally {
      busy = false
      setTimeout(() => (status = ''), 3000)
    }
  }

  async function play() {
    if (!preset) return
    busy = true; status = ''
    try {
      await post('/api/play', { camera: camera.name, preset })
      status = '✓'
    } catch (e) {
      status = '✗ ' + e.message
    } finally {
      busy = false
      setTimeout(() => (status = ''), 3000)
    }
  }

  async function beep() {
    busy = true; status = ''
    try {
      await post('/api/beep', { camera: camera.name })
      status = '✓ beep'
    } catch (e) {
      status = '✗ ' + e.message
    } finally {
      busy = false
      setTimeout(() => (status = ''), 3000)
    }
  }
</script>

<div class="card" class:offline={!camera.online}>
  <div class="card-header">
    <div class="cam-info">
      <span class="dot" class:online={camera.online}></span>
      <span class="name">{camera.name}</span>
      <span class="type">{camera.type}</span>
    </div>
    <button class="beep-btn" onclick={beep} disabled={busy} title="Test beep">🔔</button>
  </div>

  <div class="speak-row">
    <input
      bind:value={text}
      placeholder="Say something..."
      onkeydown={e => e.key === 'Enter' && speak()}
      disabled={busy}
    />
    <select bind:value={voice} disabled={busy}>
      <option value="">default</option>
      {#each voices as v}
        <option>{v}</option>
      {/each}
    </select>
    <button onclick={speak} disabled={busy || !text}>▶</button>
  </div>

  {#if presets.length > 0}
    <div class="preset-row">
      <select bind:value={preset} disabled={busy}>
        <option value="">— play preset —</option>
        {#each presets as p}
          <option value={p.name}>{p.category}/{p.name}</option>
        {/each}
      </select>
      <button onclick={play} disabled={busy || !preset}>▶</button>
    </div>
  {/if}

  {#if status}<div class="status">{status}</div>{/if}
</div>

<style>
  .card {
    background: #1a1a24;
    border: 1px solid #2a2a3a;
    border-radius: 10px;
    padding: 1rem;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    transition: border-color 0.2s;
  }
  .card:hover { border-color: #4c1d95; }
  .card.offline { opacity: 0.6; }

  .card-header { display: flex; align-items: center; justify-content: space-between; }
  .cam-info { display: flex; align-items: center; gap: 0.5rem; }

  .dot {
    width: 8px; height: 8px;
    border-radius: 50%;
    background: #444;
  }
  .dot.online { background: #22c55e; box-shadow: 0 0 6px #22c55e88; }

  .name { font-weight: 600; }
  .type { font-size: 0.75rem; color: #666; background: #2a2a3a; padding: 0.1rem 0.4rem; border-radius: 4px; }

  .beep-btn {
    background: transparent;
    border: 1px solid #3a3a50;
    border-radius: 6px;
    padding: 0.2rem 0.5rem;
    color: #888;
    font-size: 0.85rem;
  }
  .beep-btn:hover:not(:disabled) { border-color: #7c3aed; color: #ccc; }

  .speak-row, .preset-row { display: flex; gap: 0.4rem; }

  input {
    flex: 1;
    padding: 0.35rem 0.6rem;
    background: #12121a;
    border: 1px solid #3a3a50;
    border-radius: 6px;
    color: #e8e8f0;
    font-size: 0.85rem;
  }
  input:focus { outline: none; border-color: #7c3aed; }

  select {
    padding: 0.35rem 0.4rem;
    background: #12121a;
    border: 1px solid #3a3a50;
    border-radius: 6px;
    color: #e8e8f0;
    font-size: 0.85rem;
    max-width: 120px;
  }

  button {
    padding: 0.35rem 0.7rem;
    background: #7c3aed;
    border: none;
    border-radius: 6px;
    color: #fff;
    font-size: 0.9rem;
  }
  button:disabled { opacity: 0.4; }

  .status { font-size: 0.8rem; color: #a78bfa; }
</style>
