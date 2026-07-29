// Toast store — call toast.success("Saved!") or toast.error("Failed") from anywhere.
// Render <Toaster /> once in App.svelte to display them.

interface Toast {
  id: number
  message: string
  variant: 'default' | 'success' | 'error' | 'warning'
  duration: number
}

let toasts = $state<Toast[]>([])
let nextId = 0

function add(message: string, variant: Toast['variant'] = 'default', duration = 3000) {
  const id = nextId++
  toasts = [...toasts, { id, message, variant, duration }]
  if (duration > 0) {
    setTimeout(() => dismiss(id), duration)
  }
}

function dismiss(id: number) {
  toasts = toasts.filter((t) => t.id !== id)
}

export const toast = {
  show: (message: string, duration?: number) => add(message, 'default', duration),
  success: (message: string, duration?: number) => add(message, 'success', duration),
  error: (message: string, duration?: number) => add(message, 'error', duration ?? 5000),
  warning: (message: string, duration?: number) => add(message, 'warning', duration ?? 4000),
  dismiss,
}

export function getToasts() {
  return toasts
}
