<script>
  import { Home, BookOpen } from 'lucide-svelte'
  import CopyButton from '$lib/components/CopyButton.svelte'
  import YamlCode from '$lib/components/YamlCode.svelte'

  // YAML snippets — each has an id for copy-button state
  const restCommandYaml = `rest_command:
  camspeak_speak:
    url: http://CAMSPEAK_IP:8585/api/speak
    method: POST
    content_type: application/json
    payload: '{"camera":"{{ camera }}","text":"{{ text }}","voice":"{{ voice }}"}'

  camspeak_broadcast:
    url: http://CAMSPEAK_IP:8585/api/broadcast
    method: POST
    content_type: application/json
    payload: '{"text":"{{ text }}","voice":"{{ voice }}"}'

  camspeak_play_preset:
    url: http://CAMSPEAK_IP:8585/api/play
    method: POST
    content_type: application/json
    payload: '{"camera":"{{ camera }}","preset":"{{ preset }}"}'

  camspeak_beep:
    url: http://CAMSPEAK_IP:8585/api/beep
    method: POST
    content_type: application/json
    payload: '{"camera":"{{ camera }}"}'`

  const automationYaml = `# Person detected on the backyard camera → speak announcement
automation:
  - alias: "Backyard person detected"
    trigger:
      - platform: state
        entity_id: binary_sensor.backyard_person
        to: "on"
    condition:
      - condition: time
        after: "06:00:00"
        before: "22:00:00"
    action:
      - service: rest_command.camspeak_speak
        data:
          camera: backyard
          text: "Person detected in the backyard"
          voice: af_sky`

  const frigateAutomationYaml = `# Frigate review alert → speak on the camera that triggered it
automation:
  - alias: "Frigate alert announcement"
    trigger:
      - platform: mqtt
        topic: frigate/reviews
        payload: '{"type":"new","after":{"severity":"alert"}}'
        value_template: "{{ value_json.type }}"
    condition:
      - condition: template
        value_template: "{{ trigger.payload_json.after.severity == 'alert' }}"
    action:
      - service: rest_command.camspeak_speak
        data:
          camera: "{{ trigger.payload_json.after.camera }}"
          text: "Security alert on {{ trigger.payload_json.after.camera }}"
          voice: af_sky`

  const webhookYaml = `# Trigger camspeak from any HA webhook (e.g. Node-RED, external scripts)
automation:
  - alias: "Webhook → camspeak"
    trigger:
      - platform: webhook
        webhook_id: camspeak_speak
        allowed_methods: ["POST"]
        local_only: true
    action:
      - service: rest_command.camspeak_speak
        data:
          camera: "{{ trigger.json.camera }}"
          text: "{{ trigger.json.text }}"
          voice: "{{ trigger.json.voice | default('af_sky') }}"`

  const dashboardYaml = `# Dashboard button — one-tap speak from the HA UI
type: button
name: "Announce: Person at door"
tap_action:
  action: call-service
  service: rest_command.camspeak_speak
  service_data:
    camera: frontdoor
    text: "Someone is at the front door"
    voice: af_sky`
</script>

<div class="flex flex-col gap-5 max-w-3xl">
  <!-- Header -->
  <div>
    <h2 class="text-lg font-semibold text-primary mb-1">Home Assistant</h2>
    <p class="text-sm text-muted-foreground">
      Trigger camspeak from Home Assistant automations using the REST API.
      No custom integration needed — works with the built-in
      <code class="bg-muted px-1 rounded text-xs">rest_command</code> platform.
    </p>
  </div>

  <!-- Why HA -->
  <div class="rounded-lg border bg-card px-4 py-3 text-sm flex gap-3">
    <BookOpen class="h-5 w-5 text-primary flex-shrink-0 mt-0.5" />
    <div class="text-muted-foreground">
      <p class="text-foreground font-medium mb-1">Why use Home Assistant as the trigger?</p>
      <p>
        HA gives you rich conditions (time windows, presence, multi-sensor AND/OR), Jinja
        templates, scene management, and a dashboard — all things that would be complex to
        build into camspeak's built-in MQTT rule engine. The MQTT rules (Frigate tab) still
        work alongside this for standalone setups without HA.
      </p>
    </div>
  </div>

  <!-- Step 1: rest_command setup -->
  <div class="flex flex-col gap-2">
    <h3 class="text-sm font-semibold text-foreground flex items-center gap-2">
      <span class="flex h-5 w-5 items-center justify-center rounded-full bg-primary text-primary-foreground text-xs font-bold">1</span>
      Define REST commands
    </h3>
    <p class="text-sm text-muted-foreground">
      Add these to your <code class="bg-muted px-1 rounded text-xs">configuration.yaml</code>.
      Replace <code class="bg-muted px-1 rounded text-xs">CAMSPEAK_IP</code> with your camspeak
      host (e.g. <code class="bg-muted px-1 rounded text-xs">192.168.1.50</code> or
      <code class="bg-muted px-1 rounded text-xs">camspeak</code> if on the same Docker network).
    </p>
    <div class="relative">
      <YamlCode code={restCommandYaml} />
      <div class="absolute top-2 right-2">
        <CopyButton text={restCommandYaml} label="Copy YAML" size="sm" />
      </div>
    </div>
    <p class="text-xs text-muted-foreground">
      After editing, reload via <em>Developer Tools → YAML → Restart</em>.
      The four commands map to camspeak's <code class="bg-muted px-1 rounded">/api/speak</code>,
      <code class="bg-muted px-1 rounded">/api/broadcast</code>,
      <code class="bg-muted px-1 rounded">/api/play</code>, and
      <code class="bg-muted px-1 rounded">/api/beep</code> endpoints.
    </p>
  </div>

  <!-- Step 2: Example automations -->
  <div class="flex flex-col gap-3">
    <h3 class="text-sm font-semibold text-foreground flex items-center gap-2">
      <span class="flex h-5 w-5 items-center justify-center rounded-full bg-primary text-primary-foreground text-xs font-bold">2</span>
      Example automations
    </h3>

    <!-- Simple Frigate binary sensor -->
    <div class="flex flex-col gap-2">
      <p class="text-sm font-medium text-foreground">Frigate binary sensor → speak</p>
      <p class="text-xs text-muted-foreground">
        Uses the official Frigate HA integration, which creates
        <code class="bg-muted px-1 rounded">binary_sensor.&lt;camera&gt;_&lt;label&gt;</code>
        entities. Fires only during daytime hours.
      </p>
      <div class="relative">
        <YamlCode code={automationYaml} />
        <div class="absolute top-2 right-2">
          <CopyButton text={automationYaml} label="Copy YAML" size="sm" />
        </div>
      </div>
    </div>

    <!-- MQTT-based (Frigate review alerts) -->
    <div class="flex flex-col gap-2">
      <p class="text-sm font-medium text-foreground">Frigate MQTT review alert → speak on triggering camera</p>
      <p class="text-xs text-muted-foreground">
        Subscribes to <code class="bg-muted px-1 rounded">frigate/reviews</code> via HA's
        built-in MQTT trigger and uses Jinja to extract the camera name from the payload.
        This gives you the same event source as camspeak's MQTT rules, but with HA's
        condition/template engine.
      </p>
      <div class="relative">
        <YamlCode code={frigateAutomationYaml} />
        <div class="absolute top-2 right-2">
          <CopyButton text={frigateAutomationYaml} label="Copy YAML" size="sm" />
        </div>
      </div>
    </div>

    <!-- Webhook trigger -->
    <div class="flex flex-col gap-2">
      <p class="text-sm font-medium text-foreground">Webhook trigger (Node-RED, scripts, etc.)</p>
      <p class="text-xs text-muted-foreground">
        Expose a HA webhook that forwards to camspeak. POST to
        <code class="bg-muted px-1 rounded">/api/webhook/camspeak_speak</code> with a JSON body
        like <code class="bg-muted px-1 rounded">{`{"camera":"backyard","text":"Hello"}`}</code>.
      </p>
      <div class="relative">
        <YamlCode code={webhookYaml} />
        <div class="absolute top-2 right-2">
          <CopyButton text={webhookYaml} label="Copy YAML" size="sm" />
        </div>
      </div>
    </div>
  </div>

  <!-- Step 3: Dashboard button -->
  <div class="flex flex-col gap-2">
    <h3 class="text-sm font-semibold text-foreground flex items-center gap-2">
      <span class="flex h-5 w-5 items-center justify-center rounded-full bg-primary text-primary-foreground text-xs font-bold">3</span>
      Dashboard button (optional)
    </h3>
    <p class="text-xs text-muted-foreground">
      Add a one-tap speak button to your HA dashboard. Paste this into a
      <em>Manual</em> card.
    </p>
    <div class="relative">
      <YamlCode code={dashboardYaml} />
      <div class="absolute top-2 right-2">
        <CopyButton text={dashboardYaml} label="Copy YAML" size="sm" />
      </div>
    </div>
  </div>

  <!-- Note about MQTT -->
  <div class="rounded-lg border bg-muted/30 px-4 py-3 text-sm text-muted-foreground flex gap-3">
    <Home class="h-5 w-5 text-primary flex-shrink-0 mt-0.5" />
    <div>
      <p class="text-foreground font-medium mb-0.5">MQTT rules still work</p>
      <p>
        The Frigate tab's built-in MQTT rule engine is independent of this HA setup.
        If you run camspeak without Home Assistant, use the MQTT rules. If you run HA,
        the REST approach above gives you more flexibility (conditions, templates, time
        windows) without any extra camspeak configuration.
      </p>
    </div>
  </div>

  <!-- API reference pointer -->
  <div class="rounded-lg border bg-card px-4 py-3 text-sm text-muted-foreground">
    <p>
      See the <strong class="text-foreground">REST</strong> tab for the full endpoint reference,
      including <code class="bg-muted px-1 rounded text-xs">/api/describe</code> (snapshot → vision → TTS)
      and <code class="bg-muted px-1 rounded text-xs">/api/vision</code> (snapshot → description).
    </p>
  </div>
</div>
