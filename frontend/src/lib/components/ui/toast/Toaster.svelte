<script>
  import { CheckCircle2, XCircle, AlertTriangle, Info, X } from 'lucide-svelte'
  import { getToasts, toast } from './store.svelte'
  import { cn } from '$lib/utils'

  let toasts = $derived(getToasts())

  // Native modal dialogs occupy the browser's top layer; z-index alone cannot
  // place a body-level notification above them or make its Dismiss interactive.
  function dialogPortal(node) {
    const originalParent = node.parentNode
    function place() {
      const dialogs = document.querySelectorAll('dialog[open]')
      const target = dialogs[dialogs.length - 1] || originalParent
      if (target && node.parentNode !== target) target.appendChild(node)
    }
    const observer = new MutationObserver(place)
    observer.observe(document.body, { subtree: true, childList: true, attributes: true, attributeFilter: ['open'] })
    place()
    return { destroy() { observer.disconnect(); node.remove() } }
  }

  const icons = {
    default: Info,
    success: CheckCircle2,
    error: XCircle,
    warning: AlertTriangle,
  }

  const styles = {
    default: 'border-border bg-card text-card-foreground',
    success: 'border-green-500/50 bg-green-500/10 text-green-600 dark:text-green-400',
    error: 'border-red-500/50 bg-card text-red-600 dark:text-red-400',
    warning: 'border-yellow-500/50 bg-yellow-500/10 text-yellow-600 dark:text-yellow-400',
  }
</script>

<div use:dialogPortal class="fixed bottom-4 right-4 z-[100] flex max-h-[80dvh] w-[calc(100%_-_2rem)] max-w-md flex-col gap-2 overflow-y-auto">
  {#each toasts as t (t.id)}
    {@const Icon = icons[t.variant]}
    <div
      role={t.variant === 'error' ? 'alert' : 'status'}
      aria-atomic="true"
      class={cn(
        'flex items-start gap-3 rounded-lg border px-4 py-3 shadow-lg',
        'animate-in slide-in-from-right fade-in duration-300',
        'w-full min-w-0',
        styles[t.variant],
      )}
    >
      <Icon class="mt-0.5 h-5 w-5 shrink-0" />
      <p class="min-w-0 flex-1 break-words text-sm font-medium">{t.message}</p>
      <button
        type="button"
        onclick={() => toast.dismiss(t.id)}
        class="shrink-0 rounded p-0.5 opacity-60 hover:opacity-100"
        aria-label="Dismiss"
      >
        <X class="h-4 w-4" />
      </button>
    </div>
  {/each}
</div>
