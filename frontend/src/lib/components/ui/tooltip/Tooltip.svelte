<script>
  import { cn } from '$lib/utils'

  let {
    content = '',
    side = 'top',
    delay = 200,
    multiline = false,
    children,
    class: className = '',
  } = $props()

  let visible = $state(false)
  let timer

  function show() {
    clearTimeout(timer)
    timer = setTimeout(() => (visible = true), delay)
  }

  function hide() {
    clearTimeout(timer)
    visible = false
  }

  const positions = {
    top: 'bottom-full left-1/2 -translate-x-1/2 mb-2',
    bottom: 'top-full left-1/2 -translate-x-1/2 mt-2',
    left: 'right-full top-1/2 -translate-y-1/2 mr-2',
    right: 'left-full top-1/2 -translate-y-1/2 ml-2',
  }
</script>

<span class="relative inline-flex" onmouseenter={show} onmouseleave={hide} onfocus={show} onblur={hide}>
  {@render children?.()}
  {#if visible}
    <span
      role="tooltip"
      class={cn(
        'absolute z-50 rounded-md border bg-popover px-3 py-1.5 text-xs text-popover-foreground shadow-md',
        'animate-in fade-in zoom-in-95 duration-150',
        multiline ? 'whitespace-pre-line max-w-[340px]' : 'whitespace-nowrap',
        positions[side],
        className,
      )}
    >
      {content}
    </span>
  {/if}
</span>
