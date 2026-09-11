<script>
  import VisionTest from './VisionTest.svelte'
  import CaptureBenchmark from './CaptureBenchmark.svelte'
  import Benchmark from './Benchmark.svelte'
  import { Button } from '$lib/components/ui/button'
  import { onMount } from 'svelte'
  import { apiClient } from '$lib/api'
  import { toast } from '$lib/components/ui/toast'

  let { cameras = [] } = $props()
  let tab = $state('vision')
  let matrixVision = $state(true)
  let globalPrompt = $state('')
  onMount(() => {
    apiClient.getVisionConfig().then(config => globalPrompt = config.prompt ?? '')
      .catch(error => toast.error('Could not load the vision prompt: ' + error.message))
  })
  async function savePrompt(prompt) {
    try {
      const config = await apiClient.getVisionConfig()
      await apiClient.saveVisionConfig({ url: config.url, model: config.model, prompt })
      globalPrompt = prompt
      toast.success('Default vision prompt saved')
    } catch (error) {
      toast.error('Could not save the vision prompt: ' + error.message)
    }
  }
  const tabs = [
    { id: 'vision', label: 'Vision models' },
    { id: 'capture', label: 'Capture methods' },
    { id: 'matrix', label: 'Advanced matrix' },
  ]
</script>

<h1 class="mb-2 text-xl font-semibold">Benchmark</h1>
<p class="mb-4 text-sm text-muted-foreground">Test snapshots and compare vision models before changing your camera settings.</p>
<div class="mb-4 flex flex-wrap gap-2">
  {#each tabs as item}
    <Button variant={tab === item.id ? 'default' : 'outline'} onclick={() => tab = item.id}>{item.label}</Button>
  {/each}
</div>
{#if tab === 'vision'}
  <VisionTest {cameras} {globalPrompt} onSavePrompt={savePrompt} onCompareCameras={() => { matrixVision = false; tab = 'matrix' }} onCompareEverything={() => { matrixVision = true; tab = 'matrix' }} />
{:else if tab === 'capture'}
  <CaptureBenchmark {cameras} />
{:else}
  <Benchmark {cameras} initialWithVision={matrixVision} />
{/if}
