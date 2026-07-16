<script>
  import { cn } from '$lib/utils'
  let { class: className = '', ...rest } = $props()
</script>

<div
  class={cn('inline-flex items-center gap-1 rounded-lg bg-muted p-1', className)}
  {...rest}></div>
