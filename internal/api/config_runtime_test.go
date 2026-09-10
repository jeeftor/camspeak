package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jeeftor/camspeak/internal/config"
)

func TestTTSActivationRebuildsAuthenticatedClient(t *testing.T) {
	h, e, database := setupTestHandlers(t)
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-secret" {
			t.Error("missing configured TTS authorization")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "new-model" {
			t.Errorf("model = %v", body["model"])
		}
		_, _ = io.WriteString(w, "audio")
	}))
	defer endpoint.Close()
	if err := config.SaveTTSPreset(database, config.TTSPreset{
		Name: "alternate", Endpoint: endpoint.URL, Model: "new-model", APIKey: "fixture-secret",
	}); err != nil {
		t.Fatal(err)
	}
	e.POST("/api/config/tts/:name/activate", h.ActivateTTSPreset)
	if rec := doJSON(e, http.MethodPost, "/api/config/tts/alternate/activate", "{}"); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	if _, err := h.ttsClient().Speak("test", "voice"); err != nil {
		t.Fatal(err)
	}
	rec := doJSON(e, http.MethodGet, "/api/config/tts", "")
	if strings.Contains(rec.Body.String(), "fixture-secret") ||
		!strings.Contains(rec.Body.String(), `"has_api_key":true`) {
		t.Fatalf("unsafe or incomplete config response: %s", rec.Body.String())
	}
}

func TestTTSActivationHonorsEnvironment(t *testing.T) {
	h, e, database := setupTestHandlers(t)
	t.Setenv("CAMSPEAK_TTS_URL", "http://override.invalid/speech")
	t.Setenv("CAMSPEAK_TTS_API_KEY", "environment-key")
	if err := config.SaveTTSPreset(database, config.TTSPreset{Name: "alternate", Endpoint: "http://stored.invalid", Model: "model"}); err != nil {
		t.Fatal(err)
	}
	e.POST("/api/config/tts/:name/activate", h.ActivateTTSPreset)
	rec := doJSON(e, http.MethodPost, "/api/config/tts/alternate/activate", "{}")
	if rec.Code != 200 || h.ttsClient().URL != "http://override.invalid/speech" ||
		h.ttsClient().APIKey != "environment-key" {
		t.Fatal("environment override was lost during activation")
	}
}

func TestVisionSavePreservesAndExplicitlyClearsSecret(t *testing.T) {
	h, e, database := setupTestHandlers(t)
	if err := config.SetPreference(database, "vision_api_key", "vision-secret"); err != nil {
		t.Fatal(err)
	}
	e.PUT("/api/config/vision", h.UpdateVisionConfig)
	for _, tc := range []struct {
		body, want string
	}{
		{`{"url":"http://vision.invalid","model":"v","prompt":"updated","api_key":""}`, "vision-secret"},
		{`{"url":"http://vision.invalid","model":"v","clear_api_key":true}`, ""},
	} {
		rec := doJSON(e, http.MethodPut, "/api/config/vision", tc.body)
		if rec.Code != 200 || h.configSnapshot().Vision.APIKey != tc.want {
			t.Fatalf(
				"save: %s; key preserved=%t",
				rec.Body.String(),
				h.configSnapshot().Vision.APIKey == tc.want,
			)
		}
		if strings.Contains(rec.Body.String(), "vision-secret") {
			t.Fatal("vision response disclosed the secret")
		}
	}
}

func TestCameraEditPreservesOrderAndAcceptsClearAndMute(t *testing.T) {
	h, e, database := setupTestHandlers(t)
	cam := config.CameraConfig{
		Type: "hikvision", IP: "127.0.0.1", SortOrder: 7, Gain: 3,
		Pass: "password", VisionPrompt: "old", AirPlayName: "old", Channel: 1,
	}
	if err := config.SaveCamera(database, "front", cam); err != nil {
		t.Fatal(err)
	}
	h.cfg.Cameras["front"] = cam
	rec := doJSON(e, http.MethodPost, "/api/config/cameras",
		`{"name":"front","ip":"127.0.0.1","gain":0,"vision_prompt":"","airplay_name":""}`)
	if rec.Code != http.StatusCreated {
		t.Fatal(rec.Body.String())
	}
	got := h.configSnapshot().Cameras["front"]
	if got.SortOrder != 7 || got.Gain != 0 || got.VisionPrompt != "" || got.AirPlayName != "" ||
		got.Pass != "password" {
		t.Fatalf("camera edit did not preserve unrelated state or apply explicit zero/empty values")
	}
	if gain := h.reg.GetGain("front").Get(); gain != 0 {
		t.Fatalf("runtime gain = %v, want mute", gain)
	}
}

func TestActiveTTSEditPreservesAndClearsAuthentication(t *testing.T) {
	h, e, database := setupTestHandlers(t)
	if err := config.SaveTTSPreset(database, config.TTSPreset{
		Name: "active", Endpoint: "http://old.invalid", APIKey: "stored-key", IsActive: true,
	}); err != nil {
		t.Fatal(err)
	}
	e.PUT("/api/config/tts/:name", h.UpdateTTSPreset)
	for _, tc := range []struct{ body, key string }{
		{`{"endpoint":"http://new.invalid","model":"new-model"}`, "stored-key"},
		{`{"endpoint":"http://new.invalid","model":"new-model","clear_api_key":true}`, ""},
	} {
		rec := doJSON(e, http.MethodPut, "/api/config/tts/active", tc.body)
		client := h.ttsClient()
		if rec.Code != 200 || client.URL != "http://new.invalid" || client.Model != "new-model" ||
			client.APIKey != tc.key {
			t.Fatalf("active edit not applied: %s", rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "stored-key") {
			t.Fatal("preset update exposed a key")
		}
	}
}

func TestVisionProbeReusesKeyOnlyForConfiguredEndpoint(t *testing.T) {
	h, e, _ := setupTestHandlers(t)
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	defer server.Close()
	h.cfg.Vision = config.VisionConfig{URL: server.URL + "/v1/chat/completions", APIKey: "secret"}
	e.POST("/api/config/vision/test", h.TestVisionConfig)
	for _, tc := range []struct{ path, want string }{
		{"/v1/chat/completions", "Bearer secret"},
		{"/different", ""},
	} {
		body, err := json.Marshal(map[string]string{"url": server.URL + tc.path})
		if err != nil {
			t.Fatal(err)
		}
		rec := doJSON(e, http.MethodPost, "/api/config/vision/test", string(body))
		if rec.Code != 200 || authorization != tc.want {
			t.Fatalf("probe authorization mismatch for %s", tc.path)
		}
	}
}

func TestRoutingChangeRetiresPreparationAndLiveStream(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged", true: "changed"}[changed], func(t *testing.T) {
			h, e, _ := setupTestHandlers(t)
			cam := config.CameraConfig{Type: "hikvision", IP: "127.0.0.1", Enabled: true}
			h.cfg.Cameras["routing-test"] = cam
			if err := h.reg.EnableCamera("routing-test", cam); err != nil {
				t.Fatal(err)
			}
			oldSpeaker := &operationSpeaker{}
			op := beginOperation(context.Background(), "routing-test", oldSpeaker, "speak", "preparing")
			defer op.finish()
			streamCtx, cancelStream := context.WithCancel(context.Background())
			session := &streamSession{cancel: cancelStream}
			activeStreamsMu.Lock()
			activeStreams["routing-test"] = session
			activeStreamsMu.Unlock()
			defer finishStream("routing-test", session)
			e.PUT("/api/config/settings", h.UpdateSettings)
			url := "http://127.0.0.1:1"
			if changed {
				url = "http://127.0.0.1:2"
			}
			body, err := json.Marshal(map[string]string{"go2rtc_url": url})
			if err != nil {
				t.Fatal(err)
			}
			rec := doJSON(e, http.MethodPut, "/api/config/settings", string(body))
			if rec.Code != 200 {
				t.Fatal(rec.Body.String())
			}
			if !changed {
				if op.ctx.Err() != nil || streamCtx.Err() != nil {
					t.Fatal("unchanged routing interrupted playback")
				}
				return
			}
			if streamCtx.Err() == nil {
				t.Fatal("routing change left live supervisor active")
			}
			if _, err := op.send("unused", nil); !errors.Is(err, context.Canceled) {
				t.Fatalf("retired preparation result = %v", err)
			}
			if oldSpeaker.sends != 0 {
				t.Fatal("retired preparation reached old speaker")
			}
			activeStreamsMu.Lock()
			remaining := activeStreams["routing-test"]
			activeStreamsMu.Unlock()
			if remaining != nil {
				t.Fatal("routing change retained old live stream")
			}
		})
	}
}

func TestSharedVolumeUpdateRejectsMissingAndInvalidCameraGain(t *testing.T) {
	h, _, database := setupTestHandlers(t)
	cam := config.CameraConfig{Type: "hikvision", Gain: 3, SortOrder: 7}
	h.cfg.Cameras["volume-test"] = cam
	h.reg.UpdateConfig("volume-test", cam)
	if err := h.setCameraGain("missing", 1); err == nil {
		t.Fatal("missing camera accepted")
	}
	if err := h.setCameraGain("volume-test", 11); err == nil {
		t.Fatal("out-of-range gain accepted")
	}
	if err := h.setCameraGain("volume-test", 0); err != nil {
		t.Fatal(err)
	}
	var gain float64
	var order int
	if err := database.QueryRow(`SELECT gain, sort_order FROM cameras WHERE name='volume-test'`).Scan(&gain, &order); err != nil {
		t.Fatal(err)
	}
	if gain != 0 || order != 7 || h.reg.GetGain("volume-test").Get() != 0 {
		t.Fatal("shared volume update lost settings or failed to persist mute")
	}
}

func TestSettingsPersistenceFailureKeepsRuntimeRouting(t *testing.T) {
	h, e, database := setupTestHandlers(t)
	cam := config.CameraConfig{Type: "hikvision", IP: "127.0.0.1", Enabled: true}
	h.cfg.Cameras["persist-routing-test"] = cam
	if err := h.reg.EnableCamera("persist-routing-test", cam); err != nil {
		t.Fatal(err)
	}
	previousSpeaker, err := h.reg.GetForPlayback("persist-routing-test")
	if err != nil {
		t.Fatal(err)
	}
	previousURL, previousIP := h.reg.Routing()
	op := beginOperation(
		context.Background(),
		"persist-routing-test",
		&operationSpeaker{},
		"speak",
		"preparing",
	)
	defer op.finish()
	streamCtx, cancelStream := context.WithCancel(context.Background())
	session := &streamSession{cancel: cancelStream}
	activeStreamsMu.Lock()
	activeStreams["persist-routing-test"] = session
	activeStreamsMu.Unlock()
	defer finishStream("persist-routing-test", session)
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	e.PUT("/api/config/settings", h.UpdateSettings)
	rec := doJSON(e, http.MethodPut, "/api/config/settings", `{"go2rtc_url":"http://127.0.0.1:2"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want persistence failure", rec.Code)
	}
	currentURL, currentIP := h.reg.Routing()
	currentSpeaker, err := h.reg.GetForPlayback("persist-routing-test")
	if err != nil {
		t.Fatal(err)
	}
	if currentURL != previousURL || currentIP != previousIP || currentSpeaker != previousSpeaker {
		t.Fatal("failed persistence changed registry routing or retired its speaker")
	}
	if op.ctx.Err() != nil || streamCtx.Err() != nil {
		t.Fatal("failed persistence interrupted existing playback")
	}
	if h.configSnapshot().Go2rtcURL != previousURL {
		t.Fatal("failed persistence changed running configuration")
	}
}
