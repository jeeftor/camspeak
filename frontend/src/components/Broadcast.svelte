<script lang="ts">
  import AudioComposer from './AudioComposer.svelte'
  import type { Preset } from '$lib/types'
  import type { AudioDraft } from '$lib/audio-draft'
  import type { PlaybackMonitor } from '$lib/playback.svelte'

  let { voices = [], presets = [], draft = $bindable(), monitor }: {
    voices?: string[]
    presets?: Preset[]
    draft: AudioDraft
    monitor: PlaybackMonitor
  } = $props()
</script>

<AudioComposer {voices} {presets} bind:draft {monitor} />
