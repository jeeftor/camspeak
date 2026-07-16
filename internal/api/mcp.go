package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// buildMCPServer creates an MCP server exposing camspeak tools.
func buildMCPServer(h *Handlers) *server.MCPServer {
	s := server.NewMCPServer("camspeak", Version,
		server.WithToolCapabilities(true),
	)

	// speak — TTS to a named camera
	s.AddTool(
		mcp.NewTool(
			"speak",
			mcp.WithDescription("Send text-to-speech audio to a named camera speaker"),
			mcp.WithString(
				"camera",
				mcp.Required(),
				mcp.Description("Camera name (e.g. backyard, frontyard)"),
			),
			mcp.WithString("text", mcp.Required(), mcp.Description("Text to speak")),
			mcp.WithString("voice", mcp.Description("TTS voice (e.g. af_sky, af_bella)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			camera := req.GetString("camera", "")
			text := req.GetString("text", "")
			voice := req.GetString("voice", "")

			if camera == "" || text == "" {
				return mcp.NewToolResultError("camera and text required"), nil
			}

			err := h.speakText(camera, text, voice, 3.0)

			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			return mcp.NewToolResultText(fmt.Sprintf("Spoke to %s: %q", camera, text)), nil
		},
	)

	// play_preset — play a library preset
	s.AddTool(
		mcp.NewTool("play_preset",
			mcp.WithDescription("Play a saved audio preset on a camera speaker"),
			mcp.WithString("camera", mcp.Required(), mcp.Description("Camera name")),
			mcp.WithString("preset", mcp.Required(), mcp.Description("Preset name")),
			mcp.WithString("category", mcp.Description("Preset category (optional)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			camera := req.GetString("camera", "")
			preset := req.GetString("preset", "")
			category := req.GetString("category", "")

			if camera == "" || preset == "" {
				return mcp.NewToolResultError("camera and preset required"), nil
			}

			err := h.playPreset(camera, category, preset, 3.0)

			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			return mcp.NewToolResultText(fmt.Sprintf("Played preset %q on %s", preset, camera)), nil
		},
	)

	// broadcast — TTS or preset to all cameras
	s.AddTool(
		mcp.NewTool("broadcast",
			mcp.WithDescription("Send TTS or a preset to all cameras simultaneously"),
			mcp.WithString("text", mcp.Description("Text to speak")),
			mcp.WithString("preset", mcp.Description("Preset name to play")),
			mcp.WithString("voice", mcp.Description("TTS voice")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			text := req.GetString("text", "")
			preset := req.GetString("preset", "")
			voice := req.GetString("voice", "")

			if text == "" && preset == "" {
				return mcp.NewToolResultError("text or preset required"), nil
			}

			h.SpeakForMQTT(h.reg.Names(), text, preset, voice)

			return mcp.NewToolResultText("Broadcast sent to all cameras"), nil
		},
	)

	// list_cameras
	s.AddTool(
		mcp.NewTool("list_cameras",
			mcp.WithDescription("List all configured cameras and their online status"),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			status := h.reg.Status()
			lines := make([]string, 0)

			for name, online := range status {
				s := "offline"
				if online {
					s = "online"
				}

				lines = append(lines, fmt.Sprintf("- %s: %s", name, s))
			}

			return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
		},
	)

	// list_presets
	s.AddTool(
		mcp.NewTool("list_presets",
			mcp.WithDescription("List all saved audio presets in the library"),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			presets, err := h.store.List()
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			lines := make([]string, 0, len(presets))
			for _, p := range presets {
				lines = append(
					lines,
					fmt.Sprintf("- %s/%s (%.1fs) %q", p.Category, p.Name, p.Duration, p.Text),
				)
			}

			if len(lines) == 0 {
				return mcp.NewToolResultText("No presets saved yet"), nil
			}

			return mcp.NewToolResultText(strings.Join(lines, "\n")), nil
		},
	)

	// generate_preset
	s.AddTool(
		mcp.NewTool("generate_preset",
			mcp.WithDescription("Generate a TTS audio clip and save it as a reusable preset"),
			mcp.WithString("name", mcp.Required(), mcp.Description("Preset name")),
			mcp.WithString("text", mcp.Required(), mcp.Description("Text to synthesize")),
			mcp.WithString("category", mcp.Description("Category (default: alerts)")),
			mcp.WithString("voice", mcp.Description("TTS voice")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name := req.GetString("name", "")
			text := req.GetString("text", "")
			category := req.GetString("category", "alerts")
			voice := req.GetString("voice", h.cfg.TTS.DefaultVoice)

			if name == "" || text == "" {
				return mcp.NewToolResultError("name and text required"), nil
			}

			wav, err := h.tts.Speak(text, voice)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("TTS failed: %s", err)), nil
			}

			preset, err := h.store.Save(category, name, text, voice, wav)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			return mcp.NewToolResultText(
				fmt.Sprintf("Preset saved: %s/%s (%.1fs)", preset.Category, preset.Name, preset.Duration),
			), nil
		},
	)

	// beep
	s.AddTool(
		mcp.NewTool("beep",
			mcp.WithDescription("Play an 800Hz test beep on a camera"),
			mcp.WithString("camera", mcp.Required(), mcp.Description("Camera name")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			camera := req.GetString("camera", "")
			if camera == "" {
				return mcp.NewToolResultError("camera required"), nil
			}

			cam, err := h.reg.Get(camera)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			raw, err := GenerateBeep(h.tmpDir)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			if err := cam.SendRaw(raw); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			return mcp.NewToolResultText("Beeped " + camera), nil
		},
	)

	return s
}
