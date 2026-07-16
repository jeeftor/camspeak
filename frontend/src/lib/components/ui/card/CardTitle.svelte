<script>
  import { cn } from '$lib/utils'
  let { class: className = '', ...rest } = $props()
</script>

<h3 class={cn('text-lg font-semibold leading-none tracking-tight', className)} {...rest}></h3>
