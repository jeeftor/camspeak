<script>
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'

  let { voices = [], presets = [] } = $props()

  let text = $state('')
  let voice = $state('')
  let preset = $state('')
  let gain = $state(3.0)
  let busy = $state(false)
  let status = $state('')

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
      status = res.ok ? '✓ Broadcast sent' : '✗ Failed'
    } catch {
      status = '✗ Error'
    } finally {
      busy = false
      setTimeout(() => (status = ''), 3000)
    }
  }
</script>

<div class="flex flex-wrap items-center gap-2 border-b bg-muted/30 px-6 py-2.5">
  <span class="text-xs uppercase tracking-widest text-muted-foreground">Broadcast</span>
  <select bind:value={preset} class="rounded-md border border-input bg-transparent px-3 py-1 text-sm">
    <option value="">— TTS text —</option>
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
    <span class="text-xs text-muted-foreground">gain</span>
    <input type="range" min="1" max="10" step="0.5" bind:value={gain} disabled={busy} class="accent-primary" />
    <span class="text-xs text-muted-foreground font-mono w-8">{gain}x</span>
  </div>
  <Button onclick={broadcast} disabled={busy || (!text && !preset)} aria-label="Broadcast to all cameras">
    {busy ? '…' : '📢 Broadcast'}
  </Button>
  {#if status}<span class="text-sm text-primary">{status}</span>{/if}
</div>
