<script>
  import { cn } from '$lib/utils'
  let { class: className = '', children, ...rest } = $props()
</script>

<div
  class={cn('rounded-lg border bg-card text-card-foreground shadow-sm', className)}
  {...rest}>
  {@render children?.()}
</div>
