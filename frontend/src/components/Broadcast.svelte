<script>
  import { onDestroy } from 'svelte'
  import { Radio, Loader2, Square } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import VoiceSelect from '$lib/components/VoiceSelect.svelte'
  import GainSlider from '$lib/components/GainSlider.svelte'

  let { voices = [], presets = [] } = $props()

  let mode = $state('tts')
  let text = $state('')
  let voice = $state('')
  let preset = $state('')
  let gain = $state(3.0)
  let busy = $state(false)
  let status = $state('')
  let statusType = $state('ok')
  let statusTimeout

  onDestroy(() => clearTimeout(statusTimeout))

  async function broadcast() {
    if (mode === 'tts' && !text) return
    if (mode === 'preset' && !preset) return
    busy = true
    status = ''
    try {
      const body = mode === 'preset' ? { preset, gain } : { text, voice, gain }
      const res = await fetch('/api/broadcast', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      status = res.ok ? '✓ Broadcast sent to all cameras' : '✗ Failed'
      statusType = res.ok ? 'ok' : 'err'
    } catch {
      status = '✗ Error connecting to server'
      statusType = 'err'
    } finally {
      busy = false
      clearTimeout(statusTimeout)
      statusTimeout = setTimeout(() => (status = ''), 5000)
    }
  }

  async function stopAll() {
    try {
      await fetch('/api/stop', { method: 'POST' })
      status = '⏹ Stopped all cameras'
      statusType = 'ok'
    } catch {
      status = '✗ Error stopping'
      statusType = 'err'
    }
    clearTimeout(statusTimeout)
    statusTimeout = setTimeout(() => (status = ''), 5000)
  }

  const grouped = $derived(
    presets.reduce((acc, p) => {
      ;(acc[p.category] ??= []).push(p)
      return acc
    }, {})
  )
</script>

<div class="flex flex-col gap-6 max-w-3xl">
  <div>
    <h2 class="text-lg font-semibold text-primary mb-1">Broadcast</h2>
    <p class="text-sm text-muted-foreground">Send audio to all cameras simultaneously.</p>
  </div>

  <!-- Mode toggle -->
  <div class="flex gap-2">
    <Button
      variant={mode === 'tts' ? 'default' : 'outline'}
      size="sm"
      onclick={() => mode = 'tts'}
    >TTS Text</Button>
    <Button
      variant={mode === 'preset' ? 'default' : 'outline'}
      size="sm"
      onclick={() => mode = 'preset'}
      disabled={presets.length === 0}
    >Audio Preset</Button>
  </div>

  {#if mode === 'tts'}
    <div class="flex flex-col gap-3">
      <label class="flex flex-col gap-1.5 text-sm text-muted-foreground">
        Message
        <textarea
          bind:value={text}
          rows="5"
          placeholder="Type your announcement..."
          disabled={busy}
          onkeydown={e => e.ctrlKey && e.key === 'Enter' && broadcast()}
          class="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm
                 placeholder:text-muted-foreground disabled:opacity-50 resize-none
                 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
        ></textarea>
      </label>
      <label class="flex flex-col gap-1.5 text-sm text-muted-foreground">
        Voice
        <VoiceSelect bind:value={voice} {voices} {busy} class="rounded-md border border-input bg-transparent px-3 py-2" />
      </label>
    </div>
  {:else}
    <label class="flex flex-col gap-1.5 text-sm text-muted-foreground">
      Preset
      <select bind:value={preset} disabled={busy} class="rounded-md border border-input bg-transparent px-3 py-2 text-sm disabled:opacity-50">
        <option value="">— select a preset —</option>
        {#each Object.entries(grouped) as [cat, items]}
          <optgroup label={cat}>
            {#each items as p}<option value={p.name}>{p.name}</option>{/each}
          </optgroup>
        {/each}
      </select>
    </label>
  {/if}

  <!-- Volume -->
  <div class="flex flex-col gap-2">
    <div class="flex items-center justify-between text-sm text-muted-foreground">
      <span>Volume</span>
      <span class="font-mono text-foreground">{gain}×</span>
    </div>
    <GainSlider bind:value={gain} {busy} />
  </div>

  <!-- Action -->
  <div class="flex items-center gap-3 flex-wrap">
    <Button
      onclick={broadcast}
      disabled={busy || (mode === 'tts' ? !text : !preset)}
      size="lg"
    >
      {#if busy}
        <Loader2 class="h-4 w-4 animate-spin" />
        Broadcasting…
      {:else}
        <Radio class="h-4 w-4" />
        Broadcast to All Cameras
      {/if}
    </Button>
    <Button
      variant="destructive"
      onclick={stopAll}
      size="lg"
      title="Immediately stop all audio on all cameras"
    >
      <Square class="h-4 w-4 fill-current" />
      Stop All
    </Button>
    {#if status}
      <span class="text-sm {statusType === 'err' ? 'text-destructive' : 'text-primary'}">{status}</span>
    {/if}
  </div>

  <p class="text-xs text-muted-foreground">Tip: Ctrl+Enter sends when typing a message.</p>
</div>
