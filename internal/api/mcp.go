package api

import (
	"context"
	"fmt"
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
		if _, err := h.playPreset(h.log, in.Camera, in.Category, in.Preset, 3.0); err != nil {
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
		h.SpeakForMQTT(h.reg.Names(), in.Text, in.Preset, in.Voice)
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
		if _, err := cam.SendRaw(raw); err != nil {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}}}, BeepOutput{}, nil
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "Beeped " + in.Camera}}}, BeepOutput{}, nil
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
