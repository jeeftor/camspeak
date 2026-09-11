<script lang="ts">
  import { untrack } from 'svelte'
  import { toast } from '$lib/components/ui/toast'
  let { message }: { message: string } = $props()
  $effect(() => {
    const current = message
    if (current) untrack(() => toast.error(current))
  })
</script>
