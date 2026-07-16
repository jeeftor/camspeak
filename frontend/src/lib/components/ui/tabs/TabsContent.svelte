<script>
  import { cn } from '$lib/utils'
  let { class: className = '', children, ...rest } = $props()
</script>

<div class={cn('mt-2 focus-visible:outline-none', className)} {...rest}>{@render children?.()}</div>
