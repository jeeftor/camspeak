<script>
  import { cn } from '$lib/utils'
  let { class: className = '', ...rest } = $props()
</script>

<div class={cn('mt-2 focus-visible:outline-none', className)} {...rest}></div>
