<script>
  import { Button } from '$lib/components/ui/button'
  import { Input } from '$lib/components/ui/input'
  import { Select } from '$lib/components/ui/select'

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

<div class="flex flex-wrap items-center gap-2 border-b bg-muted/30 px-6 py-2.5">
  <span class="text-xs uppercase tracking-widest text-muted-foreground">Broadcast</span>
  <Select bind:value={preset} class="w-auto">
    <option value="">— TTS text —</option>
    {#each presets as p}
      <option value={p.name}>{p.category}/{p.name}</option>
    {/each}
  </Select>
  {#if !preset}
    <Input bind:value={text} placeholder="Text to broadcast..." class="min-w-[200px] flex-1" onkeydown={e => e.key === 'Enter' && broadcast()} />
    <Select bind:value={voice} class="w-auto">
      <option value="">default voice</option>
      {#each voices as v}
        <option>{v}</option>
      {/each}
    </Select>
  {/if}
  <Button onclick={broadcast} disabled={busy || (!text && !preset)}>
    {busy ? '…' : '📢 Broadcast'}
  </Button>
  {#if status}<span class="text-sm text-primary">{status}</span>{/if}
</div>
