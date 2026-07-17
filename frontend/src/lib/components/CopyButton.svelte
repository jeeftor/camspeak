<script>
  // Reusable copy-to-clipboard button with check-mark feedback.
  // Props:
  //   text         — string to copy
  //   label        — optional aria-label / title (default "Copy")
  //   size         — "sm" | "icon" (default "icon")
  //   class        — extra classes
  //   preview      — if true, show syntax-highlighted hover tooltip (desktop only)
  //   previewType  — "curl" | "text" (default "text") — controls highlighting
  import { Copy, Check } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { copyToClipboard } from '$lib/curl.svelte'
  import CurlCode from '$lib/components/CurlCode.svelte'

  let {
    text,
    label = 'Copy',
    size = 'icon',
    class: klass = '',
    disabled = false,
    preview = false,
    previewType = 'text',
  } = $props()

  let copied = $state(false)
  let showTooltip = $state(false)
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

<div
  class="copy-wrapper"
  class:preview-enabled={preview && !disabled}
  onmouseover={() => { if (preview && text && !disabled) showTooltip = true }}
  onmouseout={() => { showTooltip = false }}
>
  {#if size === 'sm'}
    <Button
      variant="outline" size="sm"
      onclick={doCopy} {disabled}
      title={preview ? undefined : label} aria-label={label}
      class={klass}
    >
      {#if copied}<Check class="h-4 w-4 text-green-500" />{:else}<Copy class="h-4 w-4" />{/if}
    </Button>
  {:else}
    <Button
      variant="outline" size="icon"
      onclick={doCopy} {disabled}
      title={preview ? undefined : label} aria-label={label}
      class={klass}
    >
      {#if copied}<Check class="h-4 w-4 text-green-500" />{:else}<Copy class="h-4 w-4" />{/if}
    </Button>
  {/if}

  {#if preview && text}
    <div class="curl-tooltip" class:show={showTooltip}>
      {#if previewType === 'curl'}
        <CurlCode code={text} />
      {:else}
        <pre class="text-xs whitespace-pre-wrap break-all">{text}</pre>
      {/if}
    </div>
  {/if}
</div>

<style>
  .copy-wrapper { position: relative; display: inline-flex; }

  .curl-tooltip {
    display: none;
    position: absolute;
    bottom: calc(100% + 8px);
    right: 0;
    z-index: 50;
    min-width: 320px;
    max-width: 520px;
    padding: 10px 12px;
    background: hsl(var(--card));
    border: 1px solid hsl(var(--border));
    border-radius: 6px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 11px;
    line-height: 1.5;
    white-space: pre-wrap;
    word-break: break-all;
    pointer-events: none;
  }

  .curl-tooltip.show {
    display: block;
    animation: tooltip-fade 120ms ease-out;
  }

  @keyframes tooltip-fade {
    from { opacity: 0; transform: translateY(4px); }
    to { opacity: 1; transform: translateY(0); }
  }
</style>
