<script>
  // Reusable gain/volume slider — appears in CameraCard and Broadcast.
  // Props: value (bound, 0-10 step 0.5), disabled
  import { Volume2 } from 'lucide-svelte'

  let { value = $bindable(), disabled = false, onchange = (_value) => {}, class: klass = '', 'aria-label': label = 'Volume' } = $props()
</script>

<div class="flex items-center gap-2 {klass}">
  <Volume2 class="h-3.5 w-3.5 text-muted-foreground flex-shrink-0" />
  <input type="range" min="0" max="10" step="0.5" bind:value {disabled} aria-label={label}
    onchange={() => onchange(value)}
    class="min-w-0 flex-1 accent-primary" />
  <span class="text-xs text-muted-foreground font-mono w-8 text-right flex-shrink-0">{value}×</span>
</div>
