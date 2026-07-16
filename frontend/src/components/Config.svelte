<script>
  import { onMount } from 'svelte'

  let { onRefresh } = $props()

  let tab = $state('tts')
  let config = $state(null)
  let ttsPresets = $state([])
  let activeTTS = $state('')
  let cameras = $state([])
  let rules = $state([])
  let voices = $state([])
  let loading = $state(true)

  // TTS form
  let ttsName = $state('')
  let ttsEndpoint = $state('')
  let ttsModel = $state('')
  let ttsVoice = $state('')
  let ttsKey = $state('')
  let ttsDesc = $state('')
  let ttsStatus = $state('')

  // Camera form
  let camName = $state('')
  let camType = $state('hikvision')
  let camIP = $state('')
  let camUser = $state('')
  let camPass = $state('')
  let camChannel = $state(1)
  let camStatus = $state('')

  // Rule form
  let ruleTopic = $state('frigate/events')
  let ruleFilter = $state('')  // JSON string
  let ruleCameras = $state('')
  let rulePreset = $state('')
  let ruleText = $state('')
  let ruleVoice = $state('')
  let ruleEnabled = $state(true)
  let ruleStatus = $state('')

  // Test status
  let testStatus = $state({})

  async function loadConfig() {
    loading = true
    try {
      const [cfgRes, ttsRes, camRes, rulesRes, voiceRes] = await Promise.all([
        fetch('/api/config'),
        fetch('/api/config/tts'),
        fetch('/api/config/cameras'),
        fetch('/api/config/rules'),
        fetch('/api/voices'),
      ])
      config = await cfgRes.json()
      const ttsData = await ttsRes.json()
      ttsPresets = ttsData.presets ?? []
      activeTTS = ttsData.active?.url ?? ''
      cameras = await camRes.json() ?? []
      rules = await rulesRes.json() ?? []
      voices = await voiceRes.json() ?? []
    } catch (e) {
      console.error('loadConfig error:', e)
    } finally {
      loading = false
    }
  }

  onMount(loadConfig)

  // --- TTS Presets ---
  async function saveTTS() {
    if (!ttsName || !ttsEndpoint) return
    ttsStatus = ''
    try {
      const res = await fetch('/api/config/tts', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: ttsName, endpoint: ttsEndpoint, model: ttsModel,
          default_voice: ttsVoice, api_key: ttsKey, description: ttsDesc,
        }),
      })
      if (!res.ok) throw new Error(await res.text())
      ttsStatus = '✓ Saved'
      ttsName = ''; ttsEndpoint = ''; ttsModel = ''; ttsVoice = ''; ttsKey = ''; ttsDesc = ''
      loadConfig()
    } catch (e) {
      ttsStatus = '✗ ' + e.message
    } finally {
      setTimeout(() => ttsStatus = '', 4000)
    }
  }

  async function activateTTS(name) {
    await fetch(`/api/config/tts/${name}/activate`, { method: 'POST' })
    loadConfig()
  }

  async function deleteTTS(name) {
    if (!confirm(`Delete TTS preset "${name}"?`)) return
    await fetch(`/api/config/tts/${name}`, { method: 'DELETE' })
    loadConfig()
  }

  async function testTTS() {
    testStatus = { ...testStatus, tts: 'testing...' }
    try {
      const res = await fetch('/api/voices')
      if (res.ok) {
        const v = await res.json()
        testStatus = { ...testStatus, tts: `✓ Connected (${v?.length ?? 0} voices)` }
      } else {
        testStatus = { ...testStatus, tts: '✗ HTTP ' + res.status }
      }
    } catch (e) {
      testStatus = { ...testStatus, tts: '✗ ' + e.message }
    }
  }

  // --- Cameras ---
  async function saveCamera() {
    if (!camName || !camIP) return
    camStatus = ''
    try {
      const res = await fetch('/api/config/cameras', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: camName, type: camType, ip: camIP,
          user: camUser, pass: camPass, channel: parseInt(camChannel) || 1,
        }),
      })
      if (!res.ok) throw new Error(await res.text())
      camStatus = '✓ Saved'
      camName = ''; camIP = ''; camUser = ''; camPass = ''; camChannel = 1
      loadConfig()
      onRefresh?.()
    } catch (e) {
      camStatus = '✗ ' + e.message
    } finally {
      setTimeout(() => camStatus = '', 4000)
    }
  }

  async function deleteCamera(name) {
    if (!confirm(`Delete camera "${name}"?`)) return
    await fetch(`/api/config/cameras/${name}`, { method: 'DELETE' })
    loadConfig()
    onRefresh?.()
  }

  async function testCamera(name) {
    testStatus = { ...testStatus, [name]: 'testing...' }
    try {
      const res = await fetch('/api/beep', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ camera: name }),
      })
      testStatus = { ...testStatus, [name]: res.ok ? '✓ Beep sent' : '✗ HTTP ' + res.status }
    } catch (e) {
      testStatus = { ...testStatus, [name]: '✗ ' + e.message }
    }
    setTimeout(() => {
      const s = { ...testStatus }
      delete s[name]
      testStatus = s
    }, 5000)
  }

  // --- Rules ---
  async function saveRule() {
    if (!ruleTopic) return
    ruleStatus = ''
    let filter = {}
    if (ruleFilter) {
      try { filter = JSON.parse(ruleFilter) }
      catch { ruleStatus = '✗ Invalid filter JSON'; return }
    }
    try {
      const res = await fetch('/api/config/rules', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          topic: ruleTopic,
          filter,
          cameras: ruleCameras.split(',').map(s => s.trim()).filter(Boolean),
          preset: rulePreset,
          text: ruleText,
          voice: ruleVoice,
          enabled: ruleEnabled,
        }),
      })
      if (!res.ok) throw new Error(await res.text())
      ruleStatus = '✓ Saved'
      ruleTopic = 'frigate/events'; ruleFilter = ''; ruleCameras = ''
      rulePreset = ''; ruleText = ''; ruleVoice = ''; ruleEnabled = true
      loadConfig()
    } catch (e) {
      ruleStatus = '✗ ' + e.message
    } finally {
      setTimeout(() => ruleStatus = '', 4000)
    }
  }

  async function testRule(rule) {
    testStatus = { ...testStatus, ['rule_' + rule.id]: 'speaking...' }
    try {
      const body = rule.preset
        ? { preset: rule.preset, camera: rule.cameras?.[0] }
        : { text: rule.text || 'Test announcement', voice: rule.voice, camera: rule.cameras?.[0] }
      const res = await fetch('/api/speak', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      testStatus = { ...testStatus, ['rule_' + rule.id]: res.ok ? '✓ Sent' : '✗ HTTP ' + res.status }
    } catch (e) {
      testStatus = { ...testStatus, ['rule_' + rule.id]: '✗ ' + e.message }
    }
    setTimeout(() => {
      const s = { ...testStatus }
      delete s['rule_' + rule.id]
      testStatus = s
    }, 5000)
  }

  function editCamera(cam) {
    camName = cam.name
    camType = cam.type
    camIP = cam.ip
    camChannel = cam.channel || 1
  }

  function editTTS(p) {
    ttsName = p.name
    ttsEndpoint = p.endpoint
    ttsModel = p.model
    ttsVoice = p.default_voice
    ttsDesc = p.description
  }
</script>

{#if loading}
  <p class="empty">Loading config…</p>
{:else}
  <div class="config">
    <div class="config-tabs">
      <button class:active={tab === 'tts'} onclick={() => tab = 'tts'}>TTS Presets</button>
      <button class:active={tab === 'cameras'} onclick={() => tab = 'cameras'}>Cameras</button>
      <button class:active={tab === 'rules'} onclick={() => tab = 'rules'}>MQTT Rules</button>
      <button class:active={tab === 'overview'} onclick={() => tab = 'overview'}>Overview</button>
    </div>

    <!-- TTS Presets -->
    {#if tab === 'tts'}
      <section class="panel">
        <div class="panel-header">
          <h3>TTS Presets</h3>
          <button class="test-btn" onclick={testTTS}>Test Connection</button>
        </div>
        {#if testStatus.tts}<span class="test-result">{testStatus.tts}</span>{/if}

        <div class="preset-list">
          {#each ttsPresets as p}
            <div class="preset-row" class:active={p.is_active}>
              <div class="preset-info">
                <span class="pname">{p.name}</span>
                {#if p.is_active}<span class="badge">ACTIVE</span>{/if}
                <span class="pmodel">{p.model}</span>
                <span class="pvoice">{p.default_voice}</span>
              </div>
              <div class="preset-actions">
                <button class="icon-btn" onclick={() => editTTS(p)} title="Edit">✎</button>
                {#if !p.is_active}
                  <button class="icon-btn" onclick={() => activateTTS(p.name)} title="Activate">●</button>
                {/if}
                <button class="icon-btn del" onclick={() => deleteTTS(p.name)} title="Delete">✕</button>
              </div>
            </div>
          {/each}
          {#if ttsPresets.length === 0}
            <p class="empty">No TTS presets configured.</p>
          {/if}
        </div>

        <details class="add-form">
          <summary>{ttsName ? 'Edit' : 'Add'} TTS Preset</summary>
          <div class="form-grid">
            <label>Name <input bind:value={ttsName} placeholder="lemonade-local" /></label>
            <label>Endpoint <input bind:value={ttsEndpoint} placeholder="http://192.168.1.91:13305/v1/audio/speech" /></label>
            <label>Model <input bind:value={ttsModel} placeholder="kokoro" /></label>
            <label>Default Voice
              <select bind:value={ttsVoice}>
                <option value="">default</option>
                {#each voices as v}<option>{v}</option>{/each}
              </select>
            </label>
            <label>API Key (optional) <input bind:value={ttsKey} type="password" placeholder="sk-..." /></label>
            <label>Description <input bind:value={ttsDesc} placeholder="Local Lemonade instance" /></label>
          </div>
          <button onclick={saveTTS} disabled={!ttsName || !ttsEndpoint}>Save Preset</button>
          {#if ttsStatus}<span class="status">{ttsStatus}</span>{/if}
        </details>
      </section>

    <!-- Cameras -->
    {:else if tab === 'cameras'}
      <section class="panel">
        <h3>Cameras</h3>
        <div class="cam-list">
          {#each cameras as cam}
            <div class="cam-row">
              <div class="cam-info">
                <span class="cname">{cam.name}</span>
                <span class="ctype">{cam.type}</span>
                <span class="cip">{cam.ip}</span>
                <span class="cch">ch{cam.channel}</span>
              </div>
              <div class="cam-actions">
                {#if testStatus[cam.name]}<span class="test-result">{testStatus[cam.name]}</span>{/if}
                <button class="icon-btn" onclick={() => testCamera(cam.name)} title="Test beep">🔔</button>
                <button class="icon-btn" onclick={() => editCamera(cam)} title="Edit">✎</button>
                <button class="icon-btn del" onclick={() => deleteCamera(cam.name)} title="Delete">✕</button>
              </div>
            </div>
          {/each}
          {#if cameras.length === 0}
            <p class="empty">No cameras configured. Run <code>camspeak discover --frigate &lt;url&gt;</code> or add one below.</p>
          {/if}
        </div>

        <details class="add-form">
          <summary>{camName ? 'Edit' : 'Add'} Camera</summary>
          <div class="form-grid">
            <label>Name <input bind:value={camName} placeholder="backyard" /></label>
            <label>Type
              <select bind:value={camType}>
                <option value="hikvision">hikvision</option>
                <option value="reolink">reolink</option>
              </select>
            </label>
            <label>IP <input bind:value={camIP} placeholder="192.168.1.181" /></label>
            <label>Username <input bind:value={camUser} placeholder="Operator" /></label>
            <label>Password <input bind:value={camPass} type="password" placeholder="password" /></label>
            <label>Channel <input bind:value={camChannel} type="number" min="1" /></label>
          </div>
          <button onclick={saveCamera} disabled={!camName || !camIP}>Save Camera</button>
          {#if camStatus}<span class="status">{camStatus}</span>{/if}
        </details>
      </section>

    <!-- MQTT Rules -->
    {:else if tab === 'rules'}
      <section class="panel">
        <h3>MQTT Rules</h3>
        <p class="hint">Rules trigger TTS announcements when MQTT messages match the topic + filter.</p>
        <div class="rule-list">
          {#each rules as r}
            <div class="rule-row" class:disabled={!r.enabled}>
              <div class="rule-info">
                <span class="rtopic">{r.topic}</span>
                {#if r.preset}<span class="rpreset">preset: {r.preset}</span>{/if}
                {#if r.text}<span class="rtext">"{r.text}"</span>{/if}
                {#if r.cameras?.length}<span class="rcams">→ {r.cameras.join(', ')}</span>{/if}
                {#if r.voice}<span class="rvoice">voice: {r.voice}</span>{/if}
                {#if Object.keys(r.filter ?? {}).length}
                  <span class="rfilter">filter: {JSON.stringify(r.filter)}</span>
                {/if}
              </div>
              <div class="rule-actions">
                {#if testStatus['rule_' + r.id]}<span class="test-result">{testStatus['rule_' + r.id]}</span>{/if}
                <button class="icon-btn" onclick={() => testRule(r)} title="Test speak">▶</button>
              </div>
            </div>
          {/each}
          {#if rules.length === 0}
            <p class="empty">No rules configured.</p>
          {/if}
        </div>

        <details class="add-form">
          <summary>Add Rule</summary>
          <div class="form-grid">
            <label>MQTT Topic <input bind:value={ruleTopic} placeholder="frigate/events" /></label>
            <label>Filter (JSON) <input bind:value={ruleFilter} placeholder={'{"type":"person"}'} /></label>
            <label>Cameras (comma-sep) <input bind:value={ruleCameras} placeholder="backyard,frontyard" /></label>
            <label>Preset (optional) <input bind:value={rulePreset} placeholder="person_detected" /></label>
            <label>Text (if no preset) <input bind:value={ruleText} placeholder="Person detected" /></label>
            <label>Voice
              <select bind:value={ruleVoice}>
                <option value="">default</option>
                {#each voices as v}<option>{v}</option>{/each}
              </select>
            </label>
            <label class="checkbox">
              <input type="checkbox" bind:checked={ruleEnabled} /> Enabled
            </label>
          </div>
          <button onclick={saveRule} disabled={!ruleTopic}>Save Rule</button>
          {#if ruleStatus}<span class="status">{ruleStatus}</span>{/if}
        </details>
      </section>

    <!-- Overview -->
    {:else if tab === 'overview'}
      <section class="panel">
        <h3>Runtime Configuration</h3>
        <pre class="json">{JSON.stringify(config, null, 2)}</pre>
      </section>
    {/if}
  </div>
{/if}

<style>
  .config { display: flex; flex-direction: column; gap: 1rem; }

  .config-tabs { display: flex; gap: 0.25rem; }
  .config-tabs button {
    padding: 0.4rem 1rem;
    border: 1px solid #3a3a50;
    border-radius: 6px;
    background: transparent;
    color: #888;
    font-size: 0.9rem;
  }
  .config-tabs button:hover { border-color: #7c3aed; color: #ccc; }
  .config-tabs button.active { background: #4c1d95; border-color: #7c3aed; color: #e8e8f0; }

  .panel {
    background: #1a1a24;
    border: 1px solid #2a2a3a;
    border-radius: 10px;
    padding: 1.25rem;
  }
  .panel-header { display: flex; align-items: center; justify-content: space-between; }
  h3 { font-size: 1rem; color: #a78bfa; margin-bottom: 0.75rem; }
  .hint { font-size: 0.8rem; color: #666; margin-bottom: 0.75rem; }

  .preset-list, .cam-list, .rule-list { display: flex; flex-direction: column; gap: 0.4rem; margin-bottom: 1rem; }

  .preset-row, .cam-row, .rule-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.5rem 0.75rem;
    background: #12121a;
    border: 1px solid #2a2a3a;
    border-radius: 8px;
  }
  .preset-row.active { border-color: #7c3aed; background: #1a1525; }
  .rule-row.disabled { opacity: 0.5; }

  .preset-info, .cam-info, .rule-info { display: flex; align-items: center; gap: 0.5rem; flex: 1; min-width: 0; flex-wrap: wrap; }
  .pname, .cname { font-weight: 600; }
  .badge { font-size: 0.65rem; background: #7c3aed; color: #fff; padding: 0.1rem 0.4rem; border-radius: 4px; }
  .pmodel, .pvoice, .ctype, .cip, .cch, .rtopic { font-size: 0.8rem; color: #888; }
  .rtopic { font-family: monospace; color: #a78bfa; }
  .rfilter, .rtext, .rcams, .rvoice, .rpreset { font-size: 0.75rem; color: #666; }

  .preset-actions, .cam-actions, .rule-actions { display: flex; align-items: center; gap: 0.3rem; flex-shrink: 0; }

  .icon-btn {
    padding: 0.2rem 0.5rem;
    background: transparent;
    border: 1px solid #3a3a50;
    border-radius: 5px;
    color: #888;
    font-size: 0.85rem;
  }
  .icon-btn:hover { border-color: #7c3aed; color: #ccc; }
  .icon-btn.del:hover { border-color: #ef4444; color: #ef4444; }

  .test-btn {
    padding: 0.3rem 0.8rem;
    background: transparent;
    border: 1px solid #3a3a50;
    border-radius: 6px;
    color: #888;
    font-size: 0.85rem;
  }
  .test-btn:hover { border-color: #7c3aed; color: #ccc; }

  .test-result { font-size: 0.8rem; color: #a78bfa; margin-right: 0.5rem; }

  .add-form { margin-top: 0.75rem; border-top: 1px solid #2a2a3a; padding-top: 0.75rem; }
  .add-form summary {
    cursor: pointer;
    font-size: 0.9rem;
    color: #a78bfa;
    padding: 0.3rem 0;
  }
  .add-form summary:hover { color: #c4b5fd; }

  .form-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.6rem;
    margin: 0.75rem 0;
  }
  .form-grid label { display: flex; flex-direction: column; gap: 0.25rem; font-size: 0.8rem; color: #aaa; }
  .form-grid label.checkbox { flex-direction: row; align-items: center; gap: 0.4rem; }
  .form-grid input, .form-grid select {
    padding: 0.35rem 0.6rem;
    background: #12121a;
    border: 1px solid #3a3a50;
    border-radius: 6px;
    color: #e8e8f0;
    font-size: 0.85rem;
  }
  .form-grid input:focus, .form-grid select:focus { outline: none; border-color: #7c3aed; }

  .add-form button {
    padding: 0.4rem 1rem;
    background: #7c3aed;
    border: none;
    border-radius: 6px;
    color: #fff;
    font-size: 0.85rem;
  }
  .add-form button:disabled { opacity: 0.4; }

  .status { font-size: 0.85rem; color: #a78bfa; margin-left: 0.5rem; }
  .empty { color: #555; font-style: italic; }
  .empty code { color: #888; font-style: normal; }

  .json {
    background: #12121a;
    border: 1px solid #2a2a3a;
    border-radius: 8px;
    padding: 1rem;
    font-size: 0.8rem;
    color: #ccc;
    overflow: auto;
    max-height: 600px;
  }

  @media (max-width: 600px) {
    .form-grid { grid-template-columns: 1fr; }
  }
</style>
