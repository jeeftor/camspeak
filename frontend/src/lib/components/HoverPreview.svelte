<script>
  // Reusable floating hover preview for captured images.
  // Used in Benchmark and CaptureBenchmark tables.
  // Callers set hoveredImage, hoverX, hoverY via bindable props.
  let {
    hoveredImage = $bindable(),
    hoverX = $bindable(),
    hoverY = $bindable(),
  } = $props()
</script>

{#if hoveredImage}
  <div
    class="fixed z-50 pointer-events-none max-w-[480px] max-h-[360px] rounded-lg border-2 border-primary/50 shadow-xl bg-black/90 p-1"
    style="left: {hoverX + 16}px; top: {hoverY + 16}px;">
    <img src={hoveredImage} alt="captured frame" class="max-w-[464px] max-h-[348px] rounded object-contain" />
  </div>
{/if}
