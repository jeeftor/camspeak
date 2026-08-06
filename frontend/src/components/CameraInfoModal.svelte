<script>
  import { Info, X, Loader2, Video, AudioLines, Wifi, Cpu, AlertCircle } from 'lucide-svelte'
  import { Button } from '$lib/components/ui/button'
  import { Badge } from '$lib/components/ui/badge'
  import { apiClient } from '$lib/api'

  let { cameraName, cameraType, show, onClose } = $props()

  let loading = $state(false)
  let info = $state(null)
  let error = $state('')

  async function loadInfo() {
    loading = true
    error = ''
    info = null
    try {
      info = await apiClient.getCameraInfo(cameraName)
    } catch (e) {
      error = e.message
    } finally {
      loading = false
    }
  }

  // Fetch when modal opens
  $effect(() => {
    if (show && cameraName) {
      loadInfo()
    }
  })

  function fmtBitrate(kbps) {
    if (!kbps) return '—'
    if (kbps >= 1000) return `${(kbps / 1000).toFixed(1)} Mbps`
    return `${kbps} kbps`
  }

  function fmtSampleRate(hz) {
    if (!hz) return '—'
    if (hz >= 1000) return `${(hz / 1000).toFixed(1)} kHz`
    return `${hz} Hz`
  }
</script>

{#if show}
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <!-- Backdrop -->
  <div class="fixed inset-0 z-[100] bg-black/60" onclick={onClose}></div>

  <!-- Modal -->
  <div class="fixed z-[101] top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2
    rounded-lg border border-border bg-card shadow-2xl flex flex-col
    w-[640px] max-w-[calc(100vw-2rem)] max-h-[calc(100vh-4rem)]">
    <!-- Header -->
    <div class="flex items-center justify-between p-4 border-b border-border">
      <div class="flex items-center gap-2">
        <Info class="h-5 w-5 text-primary" />
        <h3 class="text-sm font-semibold">{cameraName} — Device Info</h3>
        <Badge variant="secondary" class="text-xs">{cameraType}</Badge>
      </div>
      <div class="flex items-center gap-1">
        <Button variant="ghost" size="icon" class="h-7 w-7" onclick={loadInfo} disabled={loading} title="Refresh">
          {#if loading}
            <Loader2 class="h-4 w-4 animate-spin" />
          {:else}
            <Info class="h-4 w-4" />
          {/if}
        </Button>
        <Button variant="ghost" size="icon" class="h-7 w-7" onclick={onClose} title="Close">
          <X class="h-4 w-4" />
        </Button>
      </div>
    </div>

    <!-- Body (scrollable) -->
    <div class="overflow-y-auto p-4 flex flex-col gap-4">
      {#if loading}
        <div class="flex items-center justify-center py-12 text-muted-foreground">
          <Loader2 class="h-6 w-6 animate-spin mr-2" />
          Querying camera…
        </div>
      {:else if error}
        <div class="flex items-start gap-2 p-3 rounded-lg border border-destructive/30 bg-destructive/5">
          <AlertCircle class="h-5 w-5 text-destructive flex-shrink-0 mt-0.5" />
          <div class="flex flex-col gap-1">
            <p class="text-sm font-medium text-destructive">Failed to query camera</p>
            <p class="text-xs text-muted-foreground">{error}</p>
            <p class="text-xs text-muted-foreground mt-1">
              Make sure the camera is online and credentials are correct.
              {cameraType === 'hikvision' ? 'ISAPI must be enabled on the camera.' : ''}
              {cameraType === 'onvif' ? 'ONVIF service must be enabled on the camera.' : ''}
            </p>
          </div>
        </div>
      {:else if info}
        <!-- Online status -->
        <div class="flex items-center gap-2">
          <span class="h-2.5 w-2.5 rounded-full {info.online ? 'bg-green-500' : 'bg-muted-foreground/40'}"></span>
          <span class="text-xs {info.online ? 'text-green-600' : 'text-muted-foreground'}">
            {info.online ? 'Online' : 'Offline'}
          </span>
        </div>

        <!-- Device info -->
        {#if info.device.manufacturer || info.device.model || info.device.firmware || info.device.serial}
          <div class="flex flex-col gap-2">
            <div class="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground uppercase tracking-wide">
              <Cpu class="h-3.5 w-3.5" /> Device
            </div>
            <div class="grid grid-cols-2 gap-x-4 gap-y-1.5 text-sm">
              {#if info.device.manufacturer}
                <div class="flex justify-between border-b border-border/50 pb-1">
                  <span class="text-muted-foreground">Manufacturer</span>
                  <span class="font-medium">{info.device.manufacturer}</span>
                </div>
              {/if}
              {#if info.device.model}
                <div class="flex justify-between border-b border-border/50 pb-1">
                  <span class="text-muted-foreground">Model</span>
                  <span class="font-medium">{info.device.model}</span>
                </div>
              {/if}
              {#if info.device.firmware}
                <div class="flex justify-between border-b border-border/50 pb-1">
                  <span class="text-muted-foreground">Firmware</span>
                  <span class="font-medium">{info.device.firmware}</span>
                </div>
              {/if}
              {#if info.device.serial}
                <div class="flex justify-between border-b border-border/50 pb-1">
                  <span class="text-muted-foreground">Serial</span>
                  <span class="font-medium font-mono text-xs">{info.device.serial}</span>
                </div>
              {/if}
              {#if info.device.device_type}
                <div class="flex justify-between border-b border-border/50 pb-1">
                  <span class="text-muted-foreground">Type</span>
                  <span class="font-medium">{info.device.device_type}</span>
                </div>
              {/if}
              {#if info.device.hardware}
                <div class="flex justify-between border-b border-border/50 pb-1">
                  <span class="text-muted-foreground">Hardware</span>
                  <span class="font-medium">{info.device.hardware}</span>
                </div>
              {/if}
            </div>
          </div>
        {/if}

        <!-- Network info -->
        {#if info.network && (info.network.ip || info.network.mac || info.network.gateway)}
          <div class="flex flex-col gap-2">
            <div class="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground uppercase tracking-wide">
              <Wifi class="h-3.5 w-3.5" /> Network
            </div>
            <div class="grid grid-cols-2 gap-x-4 gap-y-1.5 text-sm">
              {#if info.network.ip}
                <div class="flex justify-between border-b border-border/50 pb-1">
                  <span class="text-muted-foreground">IP</span>
                  <span class="font-medium font-mono text-xs">{info.network.ip}</span>
                </div>
              {/if}
              {#if info.network.mac}
                <div class="flex justify-between border-b border-border/50 pb-1">
                  <span class="text-muted-foreground">MAC</span>
                  <span class="font-medium font-mono text-xs">{info.network.mac}</span>
                </div>
              {/if}
              {#if info.network.gateway}
                <div class="flex justify-between border-b border-border/50 pb-1">
                  <span class="text-muted-foreground">Gateway</span>
                  <span class="font-medium font-mono text-xs">{info.network.gateway}</span>
                </div>
              {/if}
              {#if info.network.subnet}
                <div class="flex justify-between border-b border-border/50 pb-1">
                  <span class="text-muted-foreground">Subnet</span>
                  <span class="font-medium font-mono text-xs">{info.network.subnet}</span>
                </div>
              {/if}
              {#if info.network.dns}
                <div class="flex justify-between border-b border-border/50 pb-1">
                  <span class="text-muted-foreground">DNS</span>
                  <span class="font-medium font-mono text-xs">{info.network.dns}</span>
                </div>
              {/if}
            </div>
          </div>
        {/if}

        <!-- Streams -->
        {#if info.streams && info.streams.length > 0}
          <div class="flex flex-col gap-2">
            <div class="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground uppercase tracking-wide">
              <Video class="h-3.5 w-3.5" /> Streams ({info.streams.length})
            </div>
            {#each info.streams as stream (stream.channel)}
              <div class="rounded-lg border border-border/60 bg-muted/20 p-3 flex flex-col gap-2">
                <div class="flex items-center justify-between">
                  <span class="text-sm font-medium">
                    {stream.name || `Channel ${stream.channel}`}
                  </span>
                  <Badge variant="outline" class="text-xs">#{stream.channel}</Badge>
                </div>

                <!-- Video config -->
                {#if stream.video}
                  <div class="flex items-start gap-2">
                    <Video class="h-3.5 w-3.5 text-muted-foreground mt-0.5 flex-shrink-0" />
                    <div class="grid grid-cols-2 gap-x-3 gap-y-0.5 text-xs flex-1">
                      <div class="flex justify-between">
                        <span class="text-muted-foreground">Codec</span>
                        <span class="font-medium">{stream.video.codec || '—'}</span>
                      </div>
                      <div class="flex justify-between">
                        <span class="text-muted-foreground">Resolution</span>
                        <span class="font-medium">{stream.video.resolution || '—'}</span>
                      </div>
                      <div class="flex justify-between">
                        <span class="text-muted-foreground">Framerate</span>
                        <span class="font-medium">{stream.video.framerate ? stream.video.framerate + ' fps' : '—'}</span>
                      </div>
                      <div class="flex justify-between">
                        <span class="text-muted-foreground">Bitrate</span>
                        <span class="font-medium">{fmtBitrate(stream.video.bitrate)}</span>
                      </div>
                      {#if stream.video.bitrate_type}
                        <div class="flex justify-between">
                          <span class="text-muted-foreground">Mode</span>
                          <span class="font-medium">{stream.video.bitrate_type}</span>
                        </div>
                      {/if}
                      {#if stream.video.gop}
                        <div class="flex justify-between">
                          <span class="text-muted-foreground">GOP</span>
                          <span class="font-medium">{stream.video.gop}</span>
                        </div>
                      {/if}
                      {#if stream.video.profile}
                        <div class="flex justify-between">
                          <span class="text-muted-foreground">Profile</span>
                          <span class="font-medium">{stream.video.profile}</span>
                        </div>
                      {/if}
                    </div>
                  </div>
                {/if}

                <!-- Audio config -->
                {#if stream.audio}
                  <div class="flex items-start gap-2">
                    <AudioLines class="h-3.5 w-3.5 text-muted-foreground mt-0.5 flex-shrink-0" />
                    <div class="grid grid-cols-2 gap-x-3 gap-y-0.5 text-xs flex-1">
                      <div class="flex justify-between">
                        <span class="text-muted-foreground">Codec</span>
                        <span class="font-medium">{stream.audio.codec || '—'}</span>
                      </div>
                      <div class="flex justify-between">
                        <span class="text-muted-foreground">Sample Rate</span>
                        <span class="font-medium">{fmtSampleRate(stream.audio.sample_rate)}</span>
                      </div>
                      <div class="flex justify-between">
                        <span class="text-muted-foreground">Bitrate</span>
                        <span class="font-medium">{fmtBitrate(stream.audio.bitrate)}</span>
                      </div>
                      <div class="flex justify-between">
                        <span class="text-muted-foreground">Channels</span>
                        <span class="font-medium">{stream.audio.channels || '—'}</span>
                      </div>
                    </div>
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {/if}

        <!-- Partial errors -->
        {#if info.errors && info.errors.length > 0}
          <div class="flex items-start gap-2 p-2.5 rounded-lg border border-amber-500/30 bg-amber-500/5">
            <AlertCircle class="h-4 w-4 text-amber-500 flex-shrink-0 mt-0.5" />
            <div class="flex flex-col gap-0.5">
              <p class="text-xs font-medium text-amber-600">Some queries failed</p>
              {#each info.errors as err}
                <p class="text-xs text-muted-foreground">{err}</p>
              {/each}
            </div>
          </div>
        {/if}
      {:else}
        <div class="text-center py-8 text-muted-foreground text-sm">No data</div>
      {/if}
    </div>
  </div>
{/if}
