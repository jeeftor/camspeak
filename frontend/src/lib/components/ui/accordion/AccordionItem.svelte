<script>
  import { ChevronDown } from 'lucide-svelte'
  import { cn } from '$lib/utils'

  let {
    value,
    title = '',
    defaultOpen = false,
    children,
  } = $props()

  // Access parent accordion context via props or a simple store.
  // For simplicity, we manage open state locally if no parent context.
  let open = $state(defaultOpen)

  function toggle() {
    open = !open
  }
</script>

<div class="py-1">
  <button
    onclick={toggle}
    class="flex w-full items-center justify-between py-3 text-left text-sm font-medium hover:bg-accent/50 rounded-md px-2"
    aria-expanded={open}
  >
    {title}
    <ChevronDown
      class={cn(
        'h-4 w-4 shrink-0 text-muted-foreground transition-transform duration-200',
        open && 'rotate-180',
      )}
    />
  </button>
  {#if open}
    <div class="px-2 pb-3 pt-1 animate-in fade-in slide-in-from-top-1 duration-200">
      {@render children?.()}
    </div>
  {/if}
</div>
