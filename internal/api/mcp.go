package api

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// buildMCPServer creates an MCP server exposing camspeak tools.
func buildMCPServer(h *Handlers) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "camspeak", Version: Version}, nil)

	// speak — TTS to a named camera
	mcp.AddTool(s, &mcp.Tool{
		Name:        "speak",
		Description: "Send text-to-speech audio to a named camera speaker",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in SpeakInput) (*mcp.CallToolResult, SpeakOutput, error) {
		if in.Camera == "" || in.Text == "" {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "camera and text required"}}}, SpeakOutput{}, nil
		}
		if _, err := h.speakText(h.log, in.Camera, in.Text, in.Voice, 3.0); err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, SpeakOutput{}, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Spoke to %s: %q", in.Camera, in.Text)}}}, SpeakOutput{}, nil
	})

	// play_preset — play a library preset
	mcp.AddTool(s, &mcp.Tool{
		Name:        "play_preset",
		Description: "Play a saved audio preset on a camera speaker",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in PlayPresetInput) (*mcp.CallToolResult, PlayPresetOutput, error) {
		if in.Camera == "" || in.Preset == "" {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "camera and preset required"}}}, PlayPresetOutput{}, nil
		}
		if _, err := h.playPreset(h.log, in.Camera, in.Category, in.Preset, 3.0, in.Loop); err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, PlayPresetOutput{}, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Played preset %q on %s", in.Preset, in.Camera)}}}, PlayPresetOutput{}, nil
	})

	// broadcast — TTS or preset to all cameras
	mcp.AddTool(s, &mcp.Tool{
		Name:        "broadcast",
		Description: "Send TTS or a preset to all cameras simultaneously",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in BroadcastInput) (*mcp.CallToolResult, BroadcastOutput, error) {
		if in.Text == "" && in.Preset == "" {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "text or preset required"}}}, BroadcastOutput{}, nil
		}
		h.SpeakForMQTT(h.reg.Names(), in.Text, in.Preset, in.Voice, false)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Broadcast sent to all cameras"}}}, BroadcastOutput{}, nil
	})

	// list_cameras
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_cameras",
		Description: "List all configured cameras and their online status",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ListCamerasInput) (*mcp.CallToolResult, ListCamerasOutput, error) {
		status := h.reg.Status()
		lines := make([]string, 0, len(status))
		for name, online := range status {
			s := "offline"
			if online {
				s = "online"
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", name, s))
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: strings.Join(lines, "\n")}}}, ListCamerasOutput{}, nil
	})

	// list_presets
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_presets",
		Description: "List all saved audio presets in the library",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ListPresetsInput) (*mcp.CallToolResult, ListPresetsOutput, error) {
		presets, err := h.store.List()
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, ListPresetsOutput{}, nil
		}
		lines := make([]string, 0, len(presets))
		for _, p := range presets {
			lines = append(lines, fmt.Sprintf("- %s/%s (%.1fs) %q", p.Category, p.Name, p.Duration, p.Text))
		}
		if len(lines) == 0 {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "No presets saved yet"}}}, ListPresetsOutput{}, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: strings.Join(lines, "\n")}}}, ListPresetsOutput{}, nil
	})

	// generate_preset
	mcp.AddTool(s, &mcp.Tool{
		Name:        "generate_preset",
		Description: "Generate a TTS audio clip and save it as a reusable preset",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in GeneratePresetInput) (*mcp.CallToolResult, GeneratePresetOutput, error) {
		if in.Name == "" || in.Text == "" {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "name and text required"}}}, GeneratePresetOutput{}, nil
		}
		category := in.Category
		if category == "" {
			category = "alerts"
		}
		voice := in.Voice
		if voice == "" {
			voice = h.cfg.TTS.DefaultVoice
		}
		wav, err := h.tts.Speak(in.Text, voice)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("TTS failed: %s", err)}}}, GeneratePresetOutput{}, nil
		}
		preset, err := h.store.Save(category, in.Name, in.Text, voice, wav)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, GeneratePresetOutput{}, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Preset saved: %s/%s (%.1fs)", preset.Category, preset.Name, preset.Duration)}}}, GeneratePresetOutput{}, nil
	})

	// beep
	mcp.AddTool(s, &mcp.Tool{
		Name:        "beep",
		Description: "Play an 800Hz test beep on a camera",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in BeepInput) (*mcp.CallToolResult, BeepOutput, error) {
		if in.Camera == "" {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "camera required"}}}, BeepOutput{}, nil
		}
		cam, err := h.reg.Get(in.Camera)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, BeepOutput{}, nil
		}
		raw, err := GenerateBeep(h.tmpDir)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, BeepOutput{}, nil
		}
		if _, err := cam.SendRaw(raw, h.reg.GetGain(in.Camera)); err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, BeepOutput{}, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Beeped " + in.Camera}}}, BeepOutput{}, nil
	})

	// play_stream — start a live audio stream to a camera
	mcp.AddTool(s, &mcp.Tool{
		Name:        "play_stream",
		Description: "Stream a live audio URL (e.g. internet radio, live ATC, .pls/.m3u playlist) to a camera speaker. The stream runs in the background until stopped or paused.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in PlayStreamInput) (*mcp.CallToolResult, PlayStreamOutput, error) {
		if in.Camera == "" || in.URL == "" {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "camera and url required"}}}, PlayStreamOutput{}, nil
		}
		cam, err := h.reg.Get(in.Camera)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, PlayStreamOutput{}, nil
		}
		gain := in.Gain
		if gain == 0 {
			gain = 3.0
		}
		streamURL, err := resolveStreamURL(in.URL)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, PlayStreamOutput{}, nil
		}
		stopStream(in.Camera)
		ctx2, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx2, "ffmpeg",
			"-nostdin", "-loglevel", "error",
			"-re",
			"-user_agent", "Mozilla/5.0 (compatible; camspeak)",
			"-i", streamURL,
			"-af", fmt.Sprintf("volume=%.2f", gain),
			"-acodec", "pcm_mulaw",
			"-ar", "8000",
			"-ac", "1",
			"-f", "mulaw",
			"-",
		)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			cancel()
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, PlayStreamOutput{}, nil
		}
		if err := cmd.Start(); err != nil {
			cancel()
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, PlayStreamOutput{}, nil
		}
		activeStreamsMu.Lock()
		activeStreams[in.Camera] = &streamSession{cmd: cmd, cancel: cancel, url: streamURL, started: now()}
		activeStreamsMu.Unlock()
		setPlayback(in.Camera, "stream", streamURL)
		go func() {
			_ = cam.Stream(stdout)
			stopStream(in.Camera)
		}()
		go func() {
			_ = cmd.Wait()
			stopStream(in.Camera)
		}()
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Streaming %s to %s", in.URL, in.Camera)}}}, PlayStreamOutput{}, nil
	})

	// stop — stop audio on a camera (or all cameras)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "stop",
		Description: "Stop all audio (TTS, streams, AirPlay) on a specific camera, or all cameras if camera is omitted. Tears down ffmpeg and closes the camera speaker connection.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in StopInput) (*mcp.CallToolResult, StopOutput, error) {
		if in.Camera != "" {
			if err := h.reg.Stop(in.Camera); err != nil {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, StopOutput{}, nil
			}
			stopStream(in.Camera)
			clearPlayback(in.Camera)
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Stopped " + in.Camera}}}, StopOutput{}, nil
		}
		h.reg.StopAll()
		stopAllStreams()
		clearAllPlayback()
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Stopped all cameras"}}}, StopOutput{}, nil
	})

	// pause — pause a live stream without tearing down the connection
	mcp.AddTool(s, &mcp.Tool{
		Name:        "pause",
		Description: "Pause a live stream on a camera (or all cameras) by suspending ffmpeg via SIGSTOP. The camera speaker connection stays open so playback can be resumed in place. Only affects streams started via play_stream.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in PauseInput) (*mcp.CallToolResult, PauseOutput, error) {
		if in.Camera != "" {
			url, alreadyPaused, ok := pauseStream(in.Camera)
			if !ok {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("no active stream for camera %s", in.Camera)}}}, PauseOutput{}, nil
			}
			setPlaybackPaused(in.Camera, true)
			status := "paused"
			if alreadyPaused {
				status = "already-paused"
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%s: %s (%s)", in.Camera, status, url)}}}, PauseOutput{}, nil
		}
		paused := pauseAllStreams()
		for _, name := range paused {
			setPlaybackPaused(name, true)
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Paused %d streams: %s", len(paused), strings.Join(paused, ", "))}}}, PauseOutput{}, nil
	})

	// resume — resume a paused live stream
	mcp.AddTool(s, &mcp.Tool{
		Name:        "resume",
		Description: "Resume a paused live stream on a camera (or all cameras) by sending SIGCONT to the suspended ffmpeg process. Only affects streams previously paused with the pause tool.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ResumeInput) (*mcp.CallToolResult, ResumeOutput, error) {
		if in.Camera != "" {
			url, notPaused, ok := resumeStream(in.Camera)
			if !ok {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("no active stream for camera %s", in.Camera)}}}, ResumeOutput{}, nil
			}
			setPlaybackPaused(in.Camera, false)
			status := "resumed"
			if notPaused {
				status = "not-paused"
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%s: %s (%s)", in.Camera, status, url)}}}, ResumeOutput{}, nil
		}
		resumed := resumeAllStreams()
		for _, name := range resumed {
			setPlaybackPaused(name, false)
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("Resumed %d streams: %s", len(resumed), strings.Join(resumed, ", "))}}}, ResumeOutput{}, nil
	})

	// get_playback — query playback status for all cameras
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_playback",
		Description: "Query the current playback state of all cameras. Returns whether each camera is playing, paused, or idle, and what is playing (stream URL, TTS text, preset name, etc.).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in GetPlaybackInput) (*mcp.CallToolResult, GetPlaybackOutput, error) {
		names := make([]string, 0, len(h.cfg.Cameras))
		for name, cfg := range h.cfg.Cameras {
			if cfg.Enabled {
				names = append(names, name)
			}
		}
		states := getAllPlayback(names)
		lines := make([]string, 0, len(states))
		for _, name := range names {
			ps := states[name]
			detail := ""
			if ps.Detail != "" {
				detail = fmt.Sprintf(" — %s", ps.Detail)
			}
			lines = append(lines, fmt.Sprintf("- %s: %s%s", name, ps.State, detail))
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: strings.Join(lines, "\n")}}}, GetPlaybackOutput{}, nil
	})

	// get_events — query historical event log
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_events",
		Description: "Query the historical event log (speak, play, beep, stop, stream, describe actions). Returns recent events in reverse chronological order. Optional camera filter and limit (default 50, max 1000).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in GetEventsInput) (*mcp.CallToolResult, GetEventsOutput, error) {
		limit := in.Limit
		if limit == 0 {
			limit = 50
		}
		events, err := h.events.queryEvents(limit, in.Camera)
		if err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, GetEventsOutput{}, nil
		}
		if len(events) == 0 {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "No events found"}}}, GetEventsOutput{}, nil
		}
		lines := make([]string, 0, len(events))
		for _, ev := range events {
			detail := ""
			if ev.Text != "" {
				detail = fmt.Sprintf(" — %q", ev.Text)
			}
			if ev.Voice != "" {
				detail += fmt.Sprintf(" [voice=%s]", ev.Voice)
			}
			cam := ev.Camera
			if cam == "" {
				cam = "all"
			}
			lines = append(lines, fmt.Sprintf("- %s  %s/%s%s", ev.At.Format("2006-01-02 15:04:05"), cam, ev.Action, detail))
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: strings.Join(lines, "\n")}}}, GetEventsOutput{}, nil
	})

	return s
}

// Tool input/output types. The jsonschema tags generate the MCP tool schema.

type SpeakInput struct {
	Camera string `json:"camera" jsonschema:"the camera name (e.g. backyard, frontyard),required"`
	Text   string `json:"text" jsonschema:"the text to speak,required"`
	Voice  string `json:"voice,omitempty" jsonschema:"optional TTS voice (e.g. af_sky, af_bella)"`
}

type SpeakOutput struct{}

type PlayPresetInput struct {
	Camera   string `json:"camera" jsonschema:"the camera name,required"`
	Preset   string `json:"preset" jsonschema:"the preset name,required"`
	Category string `json:"category,omitempty" jsonschema:"optional preset category"`
	Loop     bool   `json:"loop,omitempty" jsonschema:"if true, loop the preset infinitely (pausable via pause tool)"`
}

type PlayPresetOutput struct{}

type BroadcastInput struct {
	Text   string `json:"text,omitempty" jsonschema:"text to speak (if using TTS)"`
	Preset string `json:"preset,omitempty" jsonschema:"preset name to play (if using a preset)"`
	Voice  string `json:"voice,omitempty" jsonschema:"optional TTS voice"`
}

type BroadcastOutput struct{}

type ListCamerasInput struct{}

type ListCamerasOutput struct{}

type ListPresetsInput struct{}

type ListPresetsOutput struct{}

type GeneratePresetInput struct {
	Name     string `json:"name" jsonschema:"the preset name,required"`
	Text     string `json:"text" jsonschema:"the text to synthesize,required"`
	Category string `json:"category,omitempty" jsonschema:"optional category (default: alerts)"`
	Voice    string `json:"voice,omitempty" jsonschema:"optional TTS voice"`
}

type GeneratePresetOutput struct{}

type BeepInput struct {
	Camera string `json:"camera" jsonschema:"the camera name,required"`
}

type BeepOutput struct{}

type PlayStreamInput struct {
	Camera string  `json:"camera" jsonschema:"the camera name,required"`
	URL    string  `json:"url" jsonschema:"the audio stream URL (http/https, can be .pls or .m3u playlist),required"`
	Gain   float64 `json:"gain,omitempty" jsonschema:"optional volume gain (default 3.0)"`
}

type PlayStreamOutput struct{}

type StopInput struct {
	Camera string `json:"camera,omitempty" jsonschema:"camera name to stop (omit for all cameras)"`
}

type StopOutput struct{}

type PauseInput struct {
	Camera string `json:"camera,omitempty" jsonschema:"camera name to pause (omit for all cameras)"`
}

type PauseOutput struct{}

type ResumeInput struct {
	Camera string `json:"camera,omitempty" jsonschema:"camera name to resume (omit for all cameras)"`
}

type ResumeOutput struct{}

type GetPlaybackInput struct{}

type GetPlaybackOutput struct{}

type GetEventsInput struct {
	Camera string `json:"camera,omitempty" jsonschema:"optional camera name to filter events"`
	Limit  int    `json:"limit,omitempty" jsonschema:"max events to return (default 50, max 1000)"`
}

type GetEventsOutput struct{}
