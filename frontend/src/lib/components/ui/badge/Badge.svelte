<script>
  import { cn } from '$lib/utils'
  let { class: className = '', variant = 'default', ...rest } = $props()
  const variants = {
    default: 'bg-primary text-primary-foreground',
    secondary: 'bg-secondary text-secondary-foreground',
    destructive: 'bg-destructive text-destructive-foreground',
    outline: 'border text-foreground',
  }
</script>

<span
  class={cn(
    'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium transition-colors',
    variants[variant],
    className,
  )}
  {...rest}></span>
