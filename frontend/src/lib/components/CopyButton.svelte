<script>
  // Reusable copy-to-clipboard button with check-mark feedback.
  // Props:
  //   text  — string to copy
  //   label — optional aria-label / title (default "Copy")
  //   size  — "sm" | "icon" (default "icon")
  //   class — extra classes
  import { Copy, Check } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { copyToClipboard } from '$lib/curl'

  let { text, label = 'Copy', size = 'icon', class: klass = '', disabled = false } = $props()

  let copied = $state(false)
  let timeout

  async function doCopy() {
    const ok = await copyToClipboard(text)
    if (ok) {
      copied = true
      clearTimeout(timeout)
      timeout = setTimeout(() => (copied = false), 2000)
    }
  }
</script>

{#if size === 'sm'}
  <Button
    variant="outline" size="sm"
    onclick={doCopy} {disabled}
    title={label} aria-label={label}
    class={klass}
  >
    {#if copied}<Check class="h-4 w-4 text-green-500" />{:else}<Copy class="h-4 w-4" />{/if}
  </Button>
{:else}
  <Button
    variant="outline" size="icon"
    onclick={doCopy} {disabled}
    title={label} aria-label={label}
    class={klass}
  >
    {#if copied}<Check class="h-4 w-4 text-green-500" />{:else}<Copy class="h-4 w-4" />{/if}
  </Button>
{/if}
