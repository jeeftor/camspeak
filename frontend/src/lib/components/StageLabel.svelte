<script lang="ts">
  import { stageInfo } from '$lib/stages'
  let { stage, label }: { stage: string; label?: string } = $props()
  const info = $derived(stageInfo(stage))
</script>

<span class="stage-help">
  <button type="button" class="cursor-help text-left" aria-label={`${label ?? info.label}: ${info.description}`}>{label ?? info.label}</button>
  <span class="explanation" role="tooltip">{info.description}</span>
</span>

<style>
  .stage-help { position: relative; display: inline-block; }
  .explanation { display: none; position: absolute; left: 0; bottom: calc(100% + 6px); z-index: 60; width: 220px; max-width: calc(100vw - 48px); padding: 8px; border: 1px solid hsl(var(--border)); border-radius: 6px; background: hsl(var(--card)); color: hsl(var(--foreground)); font-size: 12px; font-weight: normal; white-space: normal; }
  .stage-help:hover .explanation, .stage-help:focus-within .explanation { display: block; }
</style>
