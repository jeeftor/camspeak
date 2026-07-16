<script>
  import { cn } from '$lib/utils'
  let { class: className = '', children, ...rest } = $props()
</script>

<div class={cn('flex flex-col space-y-1.5 p-6', className)} {...rest}>{@render children?.()}</div>
