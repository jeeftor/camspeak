<script>
  import { cn } from '$lib/utils'
  let { value, class: className = '', ...rest } = $props()
</script>

<button
  class={cn(
    'inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50',
    className,
  )}
  {...rest}></button>
