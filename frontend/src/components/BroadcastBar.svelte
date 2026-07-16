<script>
  let { voices = [], presets = [] } = $props()

  let text = $state('')
  let voice = $state('')
  let preset = $state('')
  let busy = $state(false)
  let status = $state('')

  async function broadcast() {
    if (!text && !preset) return
    busy = true
    status = ''
    try {
      const body = preset ? { preset } : { text, voice }
      const res = await fetch('/api/broadcast', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      status = res.ok ? '✓ Broadcast sent' : '✗ Failed'
    } catch {
      status = '✗ Error'
    } finally {
      busy = false
      setTimeout(() => (status = ''), 3000)
    }
  }
</script>

<div class="bar">
  <span class="label">Broadcast</span>
  <select bind:value={preset}>
    <option value="">— TTS text —</option>
    {#each presets as p}
      <option value={p.name}>{p.category}/{p.name}</option>
    {/each}
  </select>
  {#if !preset}
    <input bind:value={text} placeholder="Text to broadcast..." onkeydown={e => e.key === 'Enter' && broadcast()} />
    <select bind:value={voice}>
      <option value="">default voice</option>
      {#each voices as v}
        <option>{v}</option>
      {/each}
    </select>
  {/if}
  <button onclick={broadcast} disabled={busy || (!text && !preset)}>
    {busy ? '…' : '📢 Broadcast'}
  </button>
  {#if status}<span class="status">{status}</span>{/if}
</div>

<style>
  .bar {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.6rem 1.5rem;
    background: #12121a;
    border-bottom: 1px solid #2a2a3a;
    flex-wrap: wrap;
  }
  .label { font-size: 0.8rem; color: #888; text-transform: uppercase; letter-spacing: 0.08em; }
  input {
    flex: 1;
    min-width: 200px;
    padding: 0.35rem 0.6rem;
    background: #1a1a24;
    border: 1px solid #3a3a50;
    border-radius: 6px;
    color: #e8e8f0;
    font-size: 0.9rem;
  }
  input:focus { outline: none; border-color: #7c3aed; }
  select {
    padding: 0.35rem 0.5rem;
    background: #1a1a24;
    border: 1px solid #3a3a50;
    border-radius: 6px;
    color: #e8e8f0;
    font-size: 0.9rem;
  }
  button {
    padding: 0.35rem 0.9rem;
    background: #7c3aed;
    border: none;
    border-radius: 6px;
    color: #fff;
    font-size: 0.9rem;
    white-space: nowrap;
  }
  button:disabled { opacity: 0.5; }
  .status { font-size: 0.85rem; color: #a78bfa; }
</style>
