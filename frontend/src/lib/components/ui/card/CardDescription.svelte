<script>
  import { cn } from '$lib/utils'
  let { class: className = '', ...rest } = $props()
</script>

<p class={cn('text-sm text-muted-foreground', className)} {...rest}></p>
