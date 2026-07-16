<script>
  import { cn } from '$lib/utils'
  let { class: className = '', ...rest } = $props()
</script>

<div class={cn('p-6 pt-0', className)} {...rest}></div>
