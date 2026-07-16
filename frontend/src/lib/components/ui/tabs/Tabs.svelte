<script>
  import { cn } from '$lib/utils'
  let { value = $bindable(), class: className = '', ...rest } = $props()
</script>

<div class={cn('w-full', className)} {...rest}></div>
