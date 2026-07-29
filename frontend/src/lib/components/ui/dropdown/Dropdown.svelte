<script>
  import { cn } from '$lib/utils'
  import { ChevronDown } from 'lucide-svelte'

  let {
    label = '',
    items = [],
    class: className = '',
    trigger: Trigger,
  } = $props()

  let open = $state(false)
  let menuEl = $state(null)

  $effect(() => {
    if (!open) return
    function onClick(e) {
      if (menuEl && !menuEl.contains(e.target)) {
        open = false
      }
    }
    function onKey(e) {
      if (e.key === 'Escape') open = false
    }
    document.addEventListener('click', onClick)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('click', onClick)
      document.removeEventListener('keydown', onKey)
    }
  })

  function select(item) {
    item.onClick?.()
    open = false
  }
</script>

<div class="relative inline-block" bind:this={menuEl}>
  <button
    onclick={() => (open = !open)}
    class="inline-flex items-center gap-1 rounded-md px-3 py-1.5 text-sm font-medium hover:bg-accent"
    aria-expanded={open}
  >
    {#if Trigger}
      {@render Trigger()}
    {:else}
      {label}
      <ChevronDown class="h-4 w-4 opacity-60" />
    {/if}
  </button>

  {#if open}
    <div
      class={cn(
        'absolute right-0 top-full z-50 mt-1 min-w-[160px] overflow-hidden rounded-md border bg-popover p-1 shadow-md',
        'animate-in fade-in zoom-in-95 duration-150',
        className,
      )}
    >
      {#each items as item (item.label)}
        {#if item.separator}
          <div class="my-1 h-px bg-border"></div>
        {:else}
          <button
            onclick={() => select(item)}
            disabled={item.disabled}
            class={cn(
              'flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-sm',
              'hover:bg-accent hover:text-accent-foreground',
              'disabled:pointer-events-none disabled:opacity-50',
              item.destructive && 'text-destructive hover:bg-destructive/10',
            )}
          >
            {#if item.icon}
              <item.icon class="h-4 w-4" />
            {/if}
            {item.label}
          </button>
        {/if}
      {/each}
    </div>
  {/if}
</div>
