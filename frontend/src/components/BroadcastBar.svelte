<script>
  import { onDestroy } from 'svelte'
  import { Radio, Volume2, Loader2 } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'

  let { voices = [], presets = [] } = $props()

  let text = $state('')
  let voice = $state('')
  let preset = $state('')
  let gain = $state(3.0)
  let busy = $state(false)
  let status = $state('')
  let statusTimeout

  onDestroy(() => clearTimeout(statusTimeout))

  async function broadcast() {
    if (!text && !preset) return
    busy = true
    status = ''
    try {
      const body = preset ? { preset, gain } : { text, voice, gain }
      const res = await fetch('/api/broadcast', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      status = res.ok ? '✓ sent' : '✗ failed'
    } catch {
      status = '✗ error'
    } finally {
      busy = false
      clearTimeout(statusTimeout)
      statusTimeout = setTimeout(() => (status = ''), 4000)
    }
  }
</script>

<div class="flex flex-wrap items-center gap-2 border-b bg-muted/30 px-6 py-2.5">
  <div class="flex items-center gap-1.5 text-sm font-medium text-muted-foreground">
    <Radio class="h-4 w-4" />
    Broadcast
  </div>
  <select bind:value={preset} class="rounded-md border border-input bg-transparent px-3 py-1 text-sm">
    <option value="">TTS text</option>
    {#each presets as p}
      <option value={p.name}>{p.category}/{p.name}</option>
    {/each}
  </select>
  {#if !preset}
    <Input bind:value={text} placeholder="Text to broadcast..." class="min-w-[200px] flex-1" onkeydown={e => e.key === 'Enter' && broadcast()} />
    <select bind:value={voice} class="rounded-md border border-input bg-transparent px-3 py-1 text-sm">
      <option value="">default voice</option>
      {#each voices as v}
        <option>{v}</option>
      {/each}
    </select>
  {/if}
  <div class="flex items-center gap-1.5">
    <Volume2 class="h-3.5 w-3.5 text-muted-foreground" />
    <input type="range" min="1" max="10" step="0.5" bind:value={gain} disabled={busy} class="accent-primary" />
    <span class="text-xs text-muted-foreground font-mono w-10 text-right">{gain}x</span>
  </div>
  <Button onclick={broadcast} disabled={busy || (!text && !preset)} aria-label="Broadcast to all cameras">
    {#if busy}<Loader2 class="h-4 w-4 animate-spin" />{:else}<Radio class="h-4 w-4" />{/if}
    Broadcast
  </Button>
  {#if status}<span class="text-sm text-primary">{status}</span>{/if}
</div>
