<script>
  import { X } from 'lucide-svelte'

  let { open = $bindable(false), title = '', children } = $props()

  let dialogEl = $state(null)
  const titleId = $props.id()

  $effect(() => {
    if (!dialogEl) return
    if (open) {
      if (!dialogEl.open) dialogEl.showModal()
    } else {
      if (dialogEl.open) dialogEl.close()
    }
  })

  function onClose() {
    open = false
  }

  function onBackdropClick(e) {
    if (e.target === dialogEl) open = false
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<dialog
  bind:this={dialogEl}
  onclose={onClose}
  onclick={onBackdropClick}
  aria-labelledby={titleId}
  class="m-auto max-h-[90dvh] w-[calc(100%_-_2rem)] max-w-lg rounded-xl border bg-card text-foreground p-0 shadow-2xl backdrop:bg-black/50 open:flex open:flex-col"
>
  <div class="flex items-center justify-between border-b px-5 py-3">
    <h2 id={titleId} class="text-base font-semibold text-primary">{title}</h2>
    <button
      type="button"
      onclick={() => open = false}
      class="rounded p-1 text-muted-foreground hover:bg-accent hover:text-foreground"
      aria-label="Close"
    >
      <X class="h-4 w-4" />
    </button>
  </div>
  <div class="overflow-y-auto p-5">
    {@render children?.()}
  </div>
</dialog>
