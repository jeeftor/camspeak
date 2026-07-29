<script>
  import { CheckCircle2, XCircle, AlertTriangle, Info, X } from 'lucide-svelte'
  import { getToasts, toast } from './store.svelte'
  import { cn } from '$lib/utils'

  let toasts = getToasts()

  const icons = {
    default: Info,
    success: CheckCircle2,
    error: XCircle,
    warning: AlertTriangle,
  }

  const styles = {
    default: 'border-border bg-card text-card-foreground',
    success: 'border-green-500/50 bg-green-500/10 text-green-600 dark:text-green-400',
    error: 'border-red-500/50 bg-red-500/10 text-red-600 dark:text-red-400',
    warning: 'border-yellow-500/50 bg-yellow-500/10 text-yellow-600 dark:text-yellow-400',
  }
</script>

<div class="fixed bottom-4 right-4 z-[100] flex flex-col gap-2">
  {#each toasts as t (t.id)}
    {@const Icon = icons[t.variant]}
    <div
      class={cn(
        'flex items-start gap-3 rounded-lg border px-4 py-3 shadow-lg',
        'animate-in slide-in-from-right fade-in duration-300',
        'min-w-[280px] max-w-md',
        styles[t.variant],
      )}
    >
      <Icon class="mt-0.5 h-5 w-5 shrink-0" />
      <p class="flex-1 text-sm font-medium">{t.message}</p>
      <button
        onclick={() => toast.dismiss(t.id)}
        class="shrink-0 rounded p-0.5 opacity-60 hover:opacity-100"
        aria-label="Dismiss"
      >
        <X class="h-4 w-4" />
      </button>
    </div>
  {/each}
</div>
