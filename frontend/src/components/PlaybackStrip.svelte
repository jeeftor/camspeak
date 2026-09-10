<script lang="ts">
  import { Pause, Play, Square } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { apiClient } from '$lib/api'
  import type { PlaybackState } from '$lib/types'

  let { cameraName, playback, preparing = false, level = 0, showLevel = true, onRefresh }: {
    cameraName: string
    playback?: PlaybackState
    preparing?: boolean
    level?: number
    showLevel?: boolean
    onRefresh?: () => Promise<void>
  } = $props()
  let pending = $state(false)
  let error = $state('')

  async function control(action: 'stop' | 'pause' | 'resume') {
    pending = true
    error = ''
    try {
      await apiClient[action](cameraName)
      await onRefresh?.()
    } catch (cause) {
      error = cause instanceof Error ? cause.message : String(cause)
    } finally { pending = false }
  }
</script>

{#if preparing || (playback && playback.state !== 'idle')}
  <div class="flex flex-col gap-2 rounded-lg border border-primary/20 bg-primary/5 p-3">
    <div class="flex items-center justify-between gap-2">
      <div class="min-w-0">
        <p class="text-xs font-semibold text-primary">{playback?.state === 'paused' ? 'Paused' : playback?.state === 'playing' ? 'Playing' : 'Preparing audio'}</p>
        <p class="truncate text-xs text-muted-foreground" title={playback?.detail}>{playback?.detail || playback?.source || 'Camera audio'}</p>
      </div>
      <div class="flex shrink-0 gap-1">
        {#if playback?.can_pause}
          <Button variant="outline" size="icon" disabled={pending}
            aria-label={playback.state === 'paused' ? `Resume ${cameraName}` : `Pause ${cameraName}`}
            onclick={() => control(playback.state === 'paused' ? 'resume' : 'pause')}>
            {#if playback.state === 'paused'}<Play class="h-4 w-4" />{:else}<Pause class="h-4 w-4" />{/if}
          </Button>
        {/if}
        <Button variant="destructive" size="sm" disabled={pending} onclick={() => control('stop')}>
          <Square class="h-3.5 w-3.5" /> Stop
        </Button>
      </div>
    </div>
    {#if showLevel && playback?.state === 'playing'}
      <meter min="0" max="1" value={Math.max(0, Math.min(1, level))} aria-label={`${cameraName} audio level`} class="h-1.5 w-full accent-primary"></meter>
    {/if}
  </div>
{/if}
{#if error}<p role="alert" class="text-xs text-destructive">{error}</p>{/if}
