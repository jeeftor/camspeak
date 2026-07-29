<script>
  let { multiple = false, children } = $props()

  let openItems = $state(new Set())

  export function toggle(value) {
    const next = new Set(multiple ? openItems : [])
    if (openItems.has(value)) {
      next.delete(value)
    } else {
      next.add(value)
    }
    openItems = next
  }

  export function isOpen(value) {
    return openItems.has(value)
  }
</script>

<div class="divide-y border-y">
  {@render children?.()}
</div>
