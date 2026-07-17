<script>
  import { onMount } from 'svelte'
  import { Play, Loader2, Info } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { Select } from '$lib/components/ui/select'

  let rules = $state([])
  let voices = $state([])
  let loading = $state(true)
  let testStatus = $state({})
  let error = $state('')

  // Rule form
  let ruleTopic = $state('frigate/events')
  let ruleFilter = $state('')
  let ruleCameras = $state('')
  let rulePreset = $state('')
  let ruleText = $state('')
  let ruleVoice = $state('')
  let ruleEnabled = $state(true)
  let ruleStatus = $state('')
  let formOpen = $state(false)

  async function load() {
    loading = true
    try {
      const [rulesRes, voiceRes] = await Promise.all([
        fetch('/api/config/rules'),
        fetch('/api/voices'),
      ])
      rules = await rulesRes.json() ?? []
      voices = await voiceRes.json() ?? []
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  onMount(load)

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
      formOpen = false
      load()
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
</script>

<div class="flex flex-col gap-6 max-w-3xl">
  <div>
    <h2 class="text-lg font-semibold text-primary mb-1">Frigate / MQTT Rules</h2>
    <p class="text-sm text-muted-foreground">
      Rules trigger TTS announcements when MQTT messages arrive matching a topic and optional filter.
    </p>
  </div>

  <!-- Info box -->
  <div class="flex gap-3 rounded-lg border border-primary/20 bg-primary/5 p-4 text-sm">
    <Info class="h-4 w-4 text-primary flex-shrink-0 mt-0.5" />
    <div class="flex flex-col gap-1 text-muted-foreground">
      <p>Configure your Frigate NVR to publish events to MQTT, then add rules here to react to them.</p>
      <p>Example: topic <code class="font-mono text-xs bg-muted px-1 py-0.5 rounded">frigate/events</code> with filter
         <code class="font-mono text-xs bg-muted px-1 py-0.5 rounded">{"{"}"type":"new","after.label":"person"{"}"}</code>
         speaks "Person detected" on matching cameras.
      </p>
    </div>
  </div>

  {#if loading}
    <div class="flex items-center gap-2 text-muted-foreground text-sm">
      <Loader2 class="h-4 w-4 animate-spin" /> Loading rules…
    </div>
  {:else}
    {#if error}<p class="text-sm text-destructive">{error}</p>{/if}

    <!-- Rules list -->
    <div class="flex flex-col gap-2">
      {#if rules.length === 0}
        <p class="italic text-muted-foreground text-sm">No rules configured yet.</p>
      {:else}
        {#each rules as r}
          <div class="flex items-start justify-between gap-3 rounded-lg border bg-card px-4 py-3 {!r.enabled ? 'opacity-50' : ''}">
            <div class="flex min-w-0 flex-1 flex-col gap-1">
              <div class="flex flex-wrap items-center gap-x-3 gap-y-1">
                <code class="text-sm font-mono text-primary">{r.topic}</code>
                {#if !r.enabled}<span class="text-xs text-muted-foreground italic">disabled</span>{/if}
              </div>
              <div class="flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
                {#if r.cameras?.length}<span>cameras: {r.cameras.join(', ')}</span>{/if}
                {#if r.preset}<span>preset: <span class="text-foreground/80">{r.preset}</span></span>{/if}
                {#if r.text}<span>says: <span class="italic text-foreground/80">"{r.text}"</span></span>{/if}
                {#if r.voice}<span>voice: {r.voice}</span>{/if}
                {#if Object.keys(r.filter ?? {}).length}
                  <span>filter: <code class="bg-muted px-1 rounded">{JSON.stringify(r.filter)}</code></span>
                {/if}
              </div>
            </div>
            <div class="flex shrink-0 items-center gap-2">
              {#if testStatus['rule_' + r.id]}
                <span class="text-xs text-primary">{testStatus['rule_' + r.id]}</span>
              {/if}
              <Button variant="outline" size="sm" class="h-7 px-2" onclick={() => testRule(r)} title="Test" aria-label="Test rule">
                <Play class="h-3.5 w-3.5" />
              </Button>
            </div>
          </div>
        {/each}
      {/if}
    </div>

    <!-- Add rule form toggle -->
    <div>
      <Button variant="outline" size="sm" onclick={() => formOpen = !formOpen}>
        {formOpen ? 'Cancel' : '+ Add Rule'}
      </Button>
    </div>

    {#if formOpen}
      <div class="rounded-lg border bg-card p-5 flex flex-col gap-3">
        <h3 class="text-sm font-semibold text-primary">New Rule</h3>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            MQTT Topic
            <Input bind:value={ruleTopic} placeholder="frigate/events" />
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Filter (JSON, optional)
            <Input bind:value={ruleFilter} placeholder={'{"type":"new"}'} />
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Cameras (comma-separated)
            <Input bind:value={ruleCameras} placeholder="backyard,frontyard" />
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Preset (optional)
            <Input bind:value={rulePreset} placeholder="person_detected" />
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Text (if no preset)
            <Input bind:value={ruleText} placeholder="Person detected at the door" />
          </label>
          <label class="flex flex-col gap-1 text-xs text-muted-foreground">
            Voice
            <Select bind:value={ruleVoice}>
              <option value="">default</option>
              {#each voices as v}<option>{v}</option>{/each}
            </Select>
          </label>
        </div>
        <label class="flex items-center gap-2 text-sm text-muted-foreground">
          <input type="checkbox" bind:checked={ruleEnabled} class="h-4 w-4 rounded border-input accent-primary" />
          Enabled
        </label>
        <div class="flex items-center gap-3">
          <Button onclick={saveRule} disabled={!ruleTopic}>Save Rule</Button>
          {#if ruleStatus}<span class="text-sm {ruleStatus.startsWith('✓') ? 'text-primary' : 'text-destructive'}">{ruleStatus}</span>{/if}
        </div>
      </div>
    {/if}
  {/if}
</div>
