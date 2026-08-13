package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	clog "github.com/charmbracelet/log"
	"github.com/labstack/echo/v4"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/db"
	"github.com/jeeftor/camspeak/internal/logging"
	"github.com/jeeftor/camspeak/internal/tts"
)

// setupTestHandlers creates a Handlers instance backed by a temp SQLite DB
// and an Echo engine with all tested routes registered. It returns the
// handlers, engine, and DB for direct inspection in tests.
func setupTestHandlers(t *testing.T) (*Handlers, *echo.Echo, *sql.DB) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg, err := config.Load(database)
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	// Set a dummy go2rtc URL so FindGo2rtcURL (which does DNS lookups) is
	// never called during tests.
	cfg.Go2rtcURL = "http://127.0.0.1:1"

	reg, err := cameras.NewRegistry(cfg, nil)
	if err != nil {
		t.Fatalf("creating registry: %v", err)
	}

	h := &Handlers{
		cfg:    cfg,
		reg:    reg,
		tts:    tts.NewClient("http://127.0.0.1:1/v1/audio/speech", "kokoro"),
		events: newEventBus(database),
		db:     database,
		tmpDir: t.TempDir(),
		log:    logging.New("test", clog.InfoLevel),
	}

	e := echo.New()
	e.HideBanner = true
	// REST routes under test
	e.GET("/api/health", h.Health)
	e.GET("/api/cameras", h.Cameras)
	e.GET("/api/playback", h.Playback)
	e.GET("/api/voices", h.Voices)
	e.POST("/api/cameras/:name/ping", h.PingCamera)
	e.GET("/api/config", h.GetConfig)
	e.GET("/api/config/settings", h.GetSettings)
	e.GET("/api/config/cameras", h.ListCamerasConfig)
	e.POST("/api/config/cameras", h.CreateCamera)
	e.POST("/api/config/cameras/detect", h.DetectCameraType)
	e.DELETE("/api/config/cameras/:name", h.DeleteCameraConfig)
	e.GET("/api/config/tts", h.ListTTSPresets)
	e.POST("/api/config/tts", h.CreateTTSPreset)
	e.GET("/api/config/rules", h.ListRules)
	e.GET("/api/config/airplay", h.GetAirPlayConfig)
	e.POST("/api/pause", h.Pause)
	e.POST("/api/resume", h.Resume)
	e.GET("/api/library/upload/jobs/:id", h.UploadJobStatus)

	return h, e, database
}

// doJSON issues a request with an optional JSON body and returns the recorder.
func doJSON(e *echo.Echo, method, target string, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

// --------------------------------------------------------------------------- //
// Tests
// --------------------------------------------------------------------------- //

func TestHealth(t *testing.T) {
	_, e, _ := setupTestHandlers(t)

	rec := doJSON(e, http.MethodGet, "/api/health", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want %q", body["status"], "ok")
	}
	if body["version"] != Version {
		t.Errorf("version = %q, want %q", body["version"], Version)
	}
}

func TestCameras(t *testing.T) {
	h, e, _ := setupTestHandlers(t)

	// Two cameras: one enabled, one disabled.
	h.cfg.Cameras["front"] = config.CameraConfig{
		Type:           "hikvision",
		IP:             "192.168.1.100",
		Enabled:        true,
		AirPlayEnabled: true,
		AirPlayName:    "Front Camera",
	}
	h.cfg.Cameras["back"] = config.CameraConfig{
		Type:    "hikvision",
		IP:      "192.168.1.101",
		Enabled: false,
	}

	rec := doJSON(e, http.MethodGet, "/api/cameras", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var cams []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &cams); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cams) != 1 {
		t.Fatalf("len(cameras) = %d, want 1", len(cams))
	}
	if cams[0]["name"] != "front" {
		t.Errorf("name = %v, want %q", cams[0]["name"], "front")
	}
	if cams[0]["type"] != "hikvision" {
		t.Errorf("type = %v, want %q", cams[0]["type"], "hikvision")
	}
	if cams[0]["ip"] != "192.168.1.100" {
		t.Errorf("ip = %v, want %q", cams[0]["ip"], "192.168.1.100")
	}
	if cams[0]["airplay_enabled"] != true {
		t.Errorf("airplay_enabled = %v, want true", cams[0]["airplay_enabled"])
	}
	if cams[0]["airplay_name"] != "Front Camera" {
		t.Errorf("airplay_name = %v, want %q", cams[0]["airplay_name"], "Front Camera")
	}
}

func TestVoices(t *testing.T) {
	_, e, _ := setupTestHandlers(t)

	rec := doJSON(e, http.MethodGet, "/api/voices", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var voices []string
	if err := json.Unmarshal(rec.Body.Bytes(), &voices); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(voices) == 0 {
		t.Error("expected non-empty voice list")
	}
	// Verify a known Kokoro voice is present.
	found := false
	for _, v := range voices {
		if v == "af_sky" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'af_sky' in voices list")
	}
}

func TestPingCamera(t *testing.T) {
	h, e, _ := setupTestHandlers(t)

	h.cfg.Cameras["front"] = config.CameraConfig{
		Type:    "hikvision",
		IP:      "127.0.0.1",
		Enabled: true,
	}

	t.Run("nonexistent", func(t *testing.T) {
		rec := doJSON(e, http.MethodPost, "/api/cameras/nonexistent/ping", "")
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("existing_unreachable", func(t *testing.T) {
		rec := doJSON(e, http.MethodPost, "/api/cameras/front/ping", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		var body map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if body["camera"] != "front" {
			t.Errorf("camera = %v, want %q", body["camera"], "front")
		}
		if body["ok"] != false {
			t.Errorf("ok = %v, want false", body["ok"])
		}
		if body["error"] != "unreachable" {
			t.Errorf("error = %v, want %q", body["error"], "unreachable")
		}
	})
}

func TestCreateCamera(t *testing.T) {
	h, e, database := setupTestHandlers(t)

	t.Run("hikvision", func(t *testing.T) {
		body := `{"name":"front","type":"hikvision","ip":"192.168.1.100","user":"admin","pass":"secret","channel":1,"enabled":true}`
		rec := doJSON(e, http.MethodPost, "/api/config/cameras", body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
		}
		var resp map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if resp["name"] != "front" {
			t.Errorf("name = %v, want %q", resp["name"], "front")
		}

		// Verify persisted to DB.
		var camType, ip string
		err := database.QueryRow(
			"SELECT type, ip FROM cameras WHERE name = ?", "front",
		).Scan(&camType, &ip)
		if err != nil {
			t.Fatalf("querying DB: %v", err)
		}
		if camType != "hikvision" {
			t.Errorf("DB type = %q, want %q", camType, "hikvision")
		}
		if ip != "192.168.1.100" {
			t.Errorf("DB ip = %q, want %q", ip, "192.168.1.100")
		}

		// Verify in-memory config updated.
		cam, ok := h.cfg.Cameras["front"]
		if !ok {
			t.Fatal("camera not found in cfg.Cameras")
		}
		if cam.Type != "hikvision" {
			t.Errorf("cfg type = %q, want %q", cam.Type, "hikvision")
		}
	})

	t.Run("reolink_note", func(t *testing.T) {
		body := `{"name":"doorbell","type":"reolink","ip":"192.168.1.101","user":"admin","pass":"secret","enabled":false}`
		rec := doJSON(e, http.MethodPost, "/api/config/cameras", body)
		if rec.Code != http.StatusCreated {
			t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
		}

		// The note field should be set on the in-memory config.
		cam, ok := h.cfg.Cameras["doorbell"]
		if !ok {
			t.Fatal("camera not found in cfg.Cameras")
		}
		if !strings.Contains(cam.Note, "Reolink") {
			t.Errorf("note = %q, want it to contain 'Reolink'", cam.Note)
		}

		// The note should be persisted to the DB.
		var note string
		err := database.QueryRow(
			"SELECT note FROM cameras WHERE name = ?", "doorbell",
		).Scan(&note)
		if err != nil {
			t.Fatalf("querying DB for note: %v", err)
		}
		if !strings.Contains(note, "Reolink") {
			t.Errorf("DB note = %q, want it to contain 'Reolink'", note)
		}

		// Reolink stream should default to the camera name.
		if cam.Stream != "doorbell" {
			t.Errorf("stream = %q, want %q", cam.Stream, "doorbell")
		}
	})
}

func TestListCamerasConfig(t *testing.T) {
	h, e, _ := setupTestHandlers(t)

	h.cfg.Cameras["front"] = config.CameraConfig{
		Type:    "hikvision",
		IP:      "192.168.1.100",
		Enabled: true,
		Note:    "test note",
	}
	h.cfg.Cameras["back"] = config.CameraConfig{
		Type:    "reolink",
		IP:      "192.168.1.101",
		Enabled: false,
		Note:    "reolink note",
	}

	rec := doJSON(e, http.MethodGet, "/api/config/cameras", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var cams []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &cams); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(cams) != 2 {
		t.Fatalf("len = %d, want 2", len(cams))
	}

	// Build a map for easy lookup.
	byName := map[string]map[string]interface{}{}
	for _, c := range cams {
		byName[c["name"].(string)] = c
	}

	front, ok := byName["front"]
	if !ok {
		t.Fatal("front camera missing")
	}
	if front["note"] != "test note" {
		t.Errorf("front note = %v, want %q", front["note"], "test note")
	}
	if front["enabled"] != true {
		t.Errorf("front enabled = %v, want true", front["enabled"])
	}

	back, ok := byName["back"]
	if !ok {
		t.Fatal("back camera missing")
	}
	if back["note"] != "reolink note" {
		t.Errorf("back note = %v, want %q", back["note"], "reolink note")
	}
	if back["enabled"] != false {
		t.Errorf("back enabled = %v, want false", back["enabled"])
	}
}

func TestDeleteCameraConfig(t *testing.T) {
	h, e, database := setupTestHandlers(t)

	// Insert a camera via the config package so it's in the DB.
	cam := config.CameraConfig{
		Type:    "hikvision",
		IP:      "192.168.1.100",
		Enabled: true,
	}
	if err := config.SaveCamera(database, "front", cam); err != nil {
		t.Fatalf("SaveCamera: %v", err)
	}
	h.cfg.Cameras["front"] = cam

	rec := doJSON(e, http.MethodDelete, "/api/config/cameras/front", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["deleted"] != "front" {
		t.Errorf("deleted = %q, want %q", resp["deleted"], "front")
	}

	// Verify removed from in-memory config.
	if _, ok := h.cfg.Cameras["front"]; ok {
		t.Error("camera still in cfg.Cameras after delete")
	}

	// Verify removed from DB.
	var count int
	err := database.QueryRow("SELECT COUNT(*) FROM cameras WHERE name = ?", "front").Scan(&count)
	if err != nil {
		t.Fatalf("querying DB: %v", err)
	}
	if count != 0 {
		t.Errorf("DB count = %d, want 0", count)
	}
}

func TestDetectCameraType(t *testing.T) {
	_, e, _ := setupTestHandlers(t)

	// Use 127.0.0.1 — nothing should be listening on port 80 in a test
	// environment, so all probes will fail quickly (connection refused).
	body := `{"ip":"127.0.0.1","user":"admin","pass":"secret"}`
	rec := doJSON(e, http.MethodPost, "/api/config/cameras/detect", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify response structure has all required fields.
	if resp["ip"] != "127.0.0.1" {
		t.Errorf("ip = %v, want %q", resp["ip"], "127.0.0.1")
	}
	if _, ok := resp["type"]; !ok {
		t.Error("missing 'type' field in response")
	}
	if _, ok := resp["go2rtc_url"]; !ok {
		t.Error("missing 'go2rtc_url' field in response")
	}
	if _, ok := resp["note"]; !ok {
		t.Error("missing 'note' field in response")
	}
}

func TestGetConfig(t *testing.T) {
	h, e, _ := setupTestHandlers(t)

	// Set up config with secrets that should be sanitized.
	h.cfg.TTS = config.TTSConfig{
		URL:          "http://tts:8080/v1/audio/speech",
		Model:        "kokoro",
		DefaultVoice: "af_sky",
		APIKey:       "super-secret-key",
	}
	h.cfg.Vision = config.VisionConfig{
		URL:    "http://vision:8080/v1/chat/completions",
		Model:  "gpt-4o",
		APIKey: "vision-secret-key",
	}
	h.cfg.MQTT = config.MQTTConfig{
		Broker: "tcp://mqtt:1883",
		User:   "mqttuser",
		Pass:   "mqtt-secret-pass",
	}
	h.cfg.Cameras["front"] = config.CameraConfig{
		Type: "hikvision",
		IP:   "192.168.1.100",
		User: "admin",
		Pass: "camera-secret-pass",
	}

	rec := doJSON(e, http.MethodGet, "/api/config", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// TTS API key should be redacted.
	tts, ok := cfg["tts"].(map[string]interface{})
	if !ok {
		t.Fatal("missing tts in config")
	}
	if tts["api_key"] != nil && tts["api_key"] != "" {
		t.Errorf("tts api_key = %v, want empty", tts["api_key"])
	}

	// Vision API key should be redacted.
	vision, ok := cfg["vision"].(map[string]interface{})
	if !ok {
		t.Fatal("missing vision in config")
	}
	if vision["api_key"] != nil && vision["api_key"] != "" {
		t.Errorf("vision api_key = %v, want empty", vision["api_key"])
	}

	// MQTT password should be redacted.
	mqtt, ok := cfg["mqtt"].(map[string]interface{})
	if !ok {
		t.Fatal("missing mqtt in config")
	}
	if mqtt["pass"] != nil && mqtt["pass"] != "" {
		t.Errorf("mqtt pass = %v, want empty", mqtt["pass"])
	}

	// Camera password should be redacted.
	cams, ok := cfg["cameras"].(map[string]interface{})
	if !ok {
		t.Fatal("missing cameras in config")
	}
	front, ok := cams["front"].(map[string]interface{})
	if !ok {
		t.Fatal("missing front camera in config")
	}
	if front["pass"] != nil && front["pass"] != "" {
		t.Errorf("camera pass = %v, want empty", front["pass"])
	}
}

func TestGetSettings(t *testing.T) {
	h, e, _ := setupTestHandlers(t)

	h.cfg.FrigateURL = "http://frigate:5000"
	h.cfg.Go2rtcURL = "http://go2rtc:1984"
	h.cfg.AdvertiseIP = "192.168.1.50"

	rec := doJSON(e, http.MethodGet, "/api/config/settings", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if settings["frigate_url"] != "http://frigate:5000" {
		t.Errorf("frigate_url = %v, want %q", settings["frigate_url"], "http://frigate:5000")
	}
	if settings["go2rtc_url"] != "http://go2rtc:1984" {
		t.Errorf("go2rtc_url = %v, want %q", settings["go2rtc_url"], "http://go2rtc:1984")
	}
	if settings["advertise_ip"] != "192.168.1.50" {
		t.Errorf("advertise_ip = %v, want %q", settings["advertise_ip"], "192.168.1.50")
	}
}

func TestListTTSPresets(t *testing.T) {
	_, e, database := setupTestHandlers(t)

	// config.Load seeds default presets, so there should be at least one.
	rec := doJSON(e, http.MethodGet, "/api/config/tts", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	presets, ok := resp["presets"].([]interface{})
	if !ok {
		t.Fatal("missing or invalid 'presets' field")
	}
	if len(presets) == 0 {
		t.Error("expected at least one default TTS preset")
	}

	// Verify the first preset has expected fields.
	first := presets[0].(map[string]interface{})
	for _, key := range []string{"name", "endpoint", "model"} {
		if _, ok := first[key]; !ok {
			t.Errorf("preset missing %q field", key)
		}
	}

	// Verify 'active' field is present.
	if _, ok := resp["active"]; !ok {
		t.Error("missing 'active' field in response")
	}

	// Verify a preset is actually in the DB.
	var count int
	err := database.QueryRow("SELECT COUNT(*) FROM tts_presets").Scan(&count)
	if err != nil {
		t.Fatalf("querying DB: %v", err)
	}
	if count == 0 {
		t.Error("expected TTS presets in DB")
	}
}

func TestCreateTTSPreset(t *testing.T) {
	_, e, database := setupTestHandlers(t)

	body := `{"name":"test-preset","endpoint":"http://tts:8080/v1/audio/speech","model":"kokoro","default_voice":"af_sky","description":"test preset"}`
	rec := doJSON(e, http.MethodPost, "/api/config/tts", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp config.TTSPreset
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Name != "test-preset" {
		t.Errorf("name = %q, want %q", resp.Name, "test-preset")
	}
	if resp.Endpoint != "http://tts:8080/v1/audio/speech" {
		t.Errorf("endpoint = %q, want %q", resp.Endpoint, "http://tts:8080/v1/audio/speech")
	}
	if resp.Model != "kokoro" {
		t.Errorf("model = %q, want %q", resp.Model, "kokoro")
	}
	if resp.DefaultVoice != "af_sky" {
		t.Errorf("default_voice = %q, want %q", resp.DefaultVoice, "af_sky")
	}

	// Verify persisted to DB.
	var endpoint string
	err := database.QueryRow(
		"SELECT endpoint FROM tts_presets WHERE name = ?", "test-preset",
	).Scan(&endpoint)
	if err != nil {
		t.Fatalf("querying DB: %v", err)
	}
	if endpoint != "http://tts:8080/v1/audio/speech" {
		t.Errorf("DB endpoint = %q, want %q", endpoint, "http://tts:8080/v1/audio/speech")
	}
}

func TestCreateTTSPresetValidation(t *testing.T) {
	_, e, _ := setupTestHandlers(t)

	t.Run("missing_name", func(t *testing.T) {
		body := `{"endpoint":"http://tts:8080/v1/audio/speech","model":"kokoro"}`
		rec := doJSON(e, http.MethodPost, "/api/config/tts", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("missing_endpoint", func(t *testing.T) {
		body := `{"name":"bad","model":"kokoro"}`
		rec := doJSON(e, http.MethodPost, "/api/config/tts", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})
}

func TestListRules(t *testing.T) {
	h, e, database := setupTestHandlers(t)

	// Insert a rule directly into the DB.
	_, err := database.Exec(
		`INSERT INTO rules (topic, filter, cameras, preset, text, voice, enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"frigate/events", `{"zone":"front"}`, "front", "", "Hello", "af_sky", 1,
	)
	if err != nil {
		t.Fatalf("inserting rule: %v", err)
	}

	// Reload config to pick up the rule.
	cfg, err := config.Load(database)
	if err != nil {
		t.Fatalf("reloading config: %v", err)
	}
	cfg.Go2rtcURL = "http://127.0.0.1:1"
	h.cfg = cfg

	rec := doJSON(e, http.MethodGet, "/api/config/rules", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var rules []config.Rule
	if err := json.Unmarshal(rec.Body.Bytes(), &rules); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(rules) == 0 {
		t.Fatal("expected at least one rule")
	}

	// Find our rule.
	var found bool
	for _, r := range rules {
		if r.Topic == "frigate/events" && r.Text == "Hello" {
			found = true
			if !r.Enabled {
				t.Error("rule should be enabled")
			}
			break
		}
	}
	if !found {
		t.Error("inserted rule not found in response")
	}
}

func TestListRulesEmpty(t *testing.T) {
	_, e, _ := setupTestHandlers(t)

	rec := doJSON(e, http.MethodGet, "/api/config/rules", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var rules []config.Rule
	if err := json.Unmarshal(rec.Body.Bytes(), &rules); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(rules) != 0 {
		t.Errorf("len = %d, want 0", len(rules))
	}
}

func TestGetAirPlayConfig(t *testing.T) {
	h, e, _ := setupTestHandlers(t)

	h.cfg.AirPlay = config.AirPlayConfig{
		Enabled:        true,
		BasePort:       5100,
		PrimeSilenceMs: 500,
		Model:          "RealityDevice14,1",
		Gain:           1.5,
	}
	h.cfg.Cameras["front"] = config.CameraConfig{
		Type:           "hikvision",
		IP:             "192.168.1.100",
		Enabled:        true,
		AirPlayEnabled: true,
		AirPlayName:    "Front Camera",
		AirPlayModel:   "AppleTV6,2",
	}

	rec := doJSON(e, http.MethodGet, "/api/config/airplay", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp["enabled"] != true {
		t.Errorf("enabled = %v, want true", resp["enabled"])
	}
	if resp["base_port"] != float64(5100) {
		t.Errorf("base_port = %v, want 5100", resp["base_port"])
	}
	if resp["prime_silence_ms"] != float64(500) {
		t.Errorf("prime_silence_ms = %v, want 500", resp["prime_silence_ms"])
	}
	if resp["model"] != "RealityDevice14,1" {
		t.Errorf("model = %v, want %q", resp["model"], "RealityDevice14,1")
	}
	if resp["gain"] != 1.5 {
		t.Errorf("gain = %v, want 1.5", resp["gain"])
	}

	perCamera, ok := resp["per_camera"].([]interface{})
	if !ok {
		t.Fatal("missing or invalid 'per_camera' field")
	}
	if len(perCamera) != 1 {
		t.Fatalf("per_camera len = %d, want 1", len(perCamera))
	}
	cam := perCamera[0].(map[string]interface{})
	if cam["name"] != "front" {
		t.Errorf("per_camera name = %v, want %q", cam["name"], "front")
	}
	if cam["airplay_enabled"] != true {
		t.Errorf("per_camera airplay_enabled = %v, want true", cam["airplay_enabled"])
	}
	if cam["airplay_name"] != "Front Camera" {
		t.Errorf("per_camera airplay_name = %v, want %q", cam["airplay_name"], "Front Camera")
	}
	if cam["airplay_model"] != "AppleTV6,2" {
		t.Errorf("per_camera airplay_model = %v, want %q", cam["airplay_model"], "AppleTV6,2")
	}
}

// resetStreams clears the package-level activeStreams map between tests.
func resetStreams(t *testing.T) {
	t.Helper()
	activeStreamsMu.Lock()
	activeStreams = make(map[string]*streamSession)
	activeStreamsMu.Unlock()
}

// resetPlayback clears the package-level playbackStates map between tests.
func resetPlayback(t *testing.T) {
	t.Helper()
	playbackStatesMu.Lock()
	playbackStates = make(map[string]*PlaybackState)
	playbackStatesMu.Unlock()
}

// addFakeStream inserts a streamSession with no live process so the pause/resume
// state machine can be exercised without spawning ffmpeg. With a nil cmd
// process the helpers skip signaling and only flip the paused flag.
func addFakeStream(t *testing.T, camera, streamURL string) {
	t.Helper()
	activeStreamsMu.Lock()
	activeStreams[camera] = &streamSession{url: streamURL, started: now()}
	activeStreamsMu.Unlock()
}

func TestPauseNoStream(t *testing.T) {
	_, e, _ := setupTestHandlers(t)
	resetStreams(t)

	rec := doJSON(e, http.MethodPost, "/api/pause", `{"camera":"front"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestResumeNoStream(t *testing.T) {
	_, e, _ := setupTestHandlers(t)
	resetStreams(t)

	rec := doJSON(e, http.MethodPost, "/api/resume", `{"camera":"front"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestPauseResumeAllEmpty(t *testing.T) {
	_, e, _ := setupTestHandlers(t)
	resetStreams(t)

	rec := doJSON(e, http.MethodPost, "/api/pause", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("pause-all status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "paused" {
		t.Errorf("status = %v, want %q", body["status"], "paused")
	}
	cams, _ := body["cameras"].([]interface{})
	if len(cams) != 0 {
		t.Errorf("cameras len = %d, want 0", len(cams))
	}

	rec = doJSON(e, http.MethodPost, "/api/resume", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("resume-all status = %d, want %d", rec.Code, http.StatusOK)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "resumed" {
		t.Errorf("status = %v, want %q", body["status"], "resumed")
	}
}

func TestPauseResumeSingleStream(t *testing.T) {
	_, e, _ := setupTestHandlers(t)
	resetStreams(t)
	addFakeStream(t, "front", "http://example/live")

	// Pause → paused.
	rec := doJSON(e, http.MethodPost, "/api/pause", `{"camera":"front"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("pause status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "paused" {
		t.Errorf("status = %q, want %q", body["status"], "paused")
	}
	if body["camera"] != "front" {
		t.Errorf("camera = %q, want %q", body["camera"], "front")
	}

	// Session must be marked paused.
	activeStreamsMu.Lock()
	paused := activeStreams["front"].paused
	activeStreamsMu.Unlock()
	if !paused {
		t.Error("session.paused = false, want true")
	}

	// Pause again → already-paused.
	rec = doJSON(e, http.MethodPost, "/api/pause", `{"camera":"front"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("re-pause status = %d, want %d", rec.Code, http.StatusOK)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "already-paused" {
		t.Errorf("status = %q, want %q", body["status"], "already-paused")
	}

	// Resume → resumed.
	rec = doJSON(e, http.MethodPost, "/api/resume", `{"camera":"front"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("resume status = %d, want %d", rec.Code, http.StatusOK)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "resumed" {
		t.Errorf("status = %q, want %q", body["status"], "resumed")
	}

	activeStreamsMu.Lock()
	paused = activeStreams["front"].paused
	activeStreamsMu.Unlock()
	if paused {
		t.Error("session.paused = true, want false")
	}

	// Resume again → not-paused.
	rec = doJSON(e, http.MethodPost, "/api/resume", `{"camera":"front"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("re-resume status = %d, want %d", rec.Code, http.StatusOK)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "not-paused" {
		t.Errorf("status = %q, want %q", body["status"], "not-paused")
	}
}

func TestPauseAllMultipleStreams(t *testing.T) {
	_, e, _ := setupTestHandlers(t)
	resetStreams(t)
	addFakeStream(t, "front", "http://example/a")
	addFakeStream(t, "back", "http://example/b")

	rec := doJSON(e, http.MethodPost, "/api/pause", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("pause-all status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "paused" {
		t.Errorf("status = %v, want %q", body["status"], "paused")
	}
	cams, _ := body["cameras"].([]interface{})
	if len(cams) != 2 {
		t.Errorf("cameras len = %d, want 2", len(cams))
	}

	// Both sessions paused.
	activeStreamsMu.Lock()
	front := activeStreams["front"].paused
	back := activeStreams["back"].paused
	activeStreamsMu.Unlock()
	if !front || !back {
		t.Errorf("paused flags = (%v, %v), want (true, true)", front, back)
	}

	// Second pause-all is a no-op (already paused).
	rec = doJSON(e, http.MethodPost, "/api/pause", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cams, _ = body["cameras"].([]interface{})
	if len(cams) != 0 {
		t.Errorf("second pause-all cameras len = %d, want 0", len(cams))
	}

	// Resume-all resumes both.
	rec = doJSON(e, http.MethodPost, "/api/resume", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cams, _ = body["cameras"].([]interface{})
	if len(cams) != 2 {
		t.Errorf("resume-all cameras len = %d, want 2", len(cams))
	}
}

func TestPlaybackIdle(t *testing.T) {
	h, e, _ := setupTestHandlers(t)
	resetPlayback(t)

	h.cfg.Cameras["front"] = config.CameraConfig{
		Type:    "hikvision",
		IP:      "192.168.1.100",
		Enabled: true,
	}

	rec := doJSON(e, http.MethodGet, "/api/playback", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ps, ok := body["front"]
	if !ok {
		t.Fatal("missing 'front' in playback response")
	}
	if ps["state"] != "idle" {
		t.Errorf("state = %v, want %q", ps["state"], "idle")
	}
}

func TestPlaybackPlayingStream(t *testing.T) {
	h, e, _ := setupTestHandlers(t)
	resetPlayback(t)
	resetStreams(t)

	h.cfg.Cameras["front"] = config.CameraConfig{
		Type:    "hikvision",
		IP:      "192.168.1.100",
		Enabled: true,
	}

	// Simulate a stream that's playing.
	setPlayback("front", "stream", "http://example/live.m3u")

	rec := doJSON(e, http.MethodGet, "/api/playback", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ps := body["front"]
	if ps["state"] != "playing" {
		t.Errorf("state = %v, want %q", ps["state"], "playing")
	}
	if ps["source"] != "stream" {
		t.Errorf("source = %v, want %q", ps["source"], "stream")
	}
	if ps["detail"] != "http://example/live.m3u" {
		t.Errorf("detail = %v, want %q", ps["detail"], "http://example/live.m3u")
	}
}

func TestPlaybackPaused(t *testing.T) {
	h, e, _ := setupTestHandlers(t)
	resetPlayback(t)
	resetStreams(t)

	h.cfg.Cameras["front"] = config.CameraConfig{
		Type:    "hikvision",
		IP:      "192.168.1.100",
		Enabled: true,
	}

	setPlayback("front", "stream", "http://example/live")
	setPlaybackPaused("front", true)

	rec := doJSON(e, http.MethodGet, "/api/playback", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ps := body["front"]
	if ps["state"] != "paused" {
		t.Errorf("state = %v, want %q", ps["state"], "paused")
	}
	if ps["paused_at"] == nil {
		t.Error("paused_at = nil, want a timestamp")
	}
}

func TestPlaybackClearedAfterStop(t *testing.T) {
	h, e, _ := setupTestHandlers(t)
	resetPlayback(t)
	resetStreams(t)

	h.cfg.Cameras["front"] = config.CameraConfig{
		Type:    "hikvision",
		IP:      "192.168.1.100",
		Enabled: true,
	}

	setPlayback("front", "stream", "http://example/live")
	clearPlayback("front")

	rec := doJSON(e, http.MethodGet, "/api/playback", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	ps := body["front"]
	if ps["state"] != "idle" {
		t.Errorf("state = %v, want %q", ps["state"], "idle")
	}
}

func TestPlaybackOnlyEnabledCameras(t *testing.T) {
	h, e, _ := setupTestHandlers(t)
	resetPlayback(t)

	h.cfg.Cameras["front"] = config.CameraConfig{
		Type:    "hikvision",
		IP:      "192.168.1.100",
		Enabled: true,
	}
	h.cfg.Cameras["back"] = config.CameraConfig{
		Type:    "hikvision",
		IP:      "192.168.1.101",
		Enabled: false,
	}

	rec := doJSON(e, http.MethodGet, "/api/playback", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := body["front"]; !ok {
		t.Error("missing 'front' (enabled camera) in playback response")
	}
	if _, ok := body["back"]; ok {
		t.Error("disabled camera 'back' should not appear in playback response")
	}
}

// --- Upload job tracker tests ---

func TestUploadJobNotFound(t *testing.T) {
	_, e, _ := setupTestHandlers(t)
	resetUploadJobs(t)

	rec := doJSON(e, http.MethodGet, "/api/library/upload/jobs/nonexistent", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestUploadJobLifecycle(t *testing.T) {
	_, e, _ := setupTestHandlers(t)
	resetUploadJobs(t)

	// Create a job manually.
	job := newUploadJob("test-audio", "uploads", "recording.mp3")
	id := job.ID

	// Initially: transcoding, 0%.
	rec := doJSON(e, http.MethodGet, "/api/library/upload/jobs/"+id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != JobTranscoding {
		t.Errorf("status = %v, want %q", body["status"], JobTranscoding)
	}
	if body["name"] != "test-audio" {
		t.Errorf("name = %v, want %q", body["name"], "test-audio")
	}
	if body["filename"] != "recording.mp3" {
		t.Errorf("filename = %v, want %q", body["filename"], "recording.mp3")
	}

	// Update progress.
	updateUploadJob(id, 42.5, "Transcoding")
	rec = doJSON(e, http.MethodGet, "/api/library/upload/jobs/"+id, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["percent"] != 42.5 {
		t.Errorf("percent = %v, want 42.5", body["percent"])
	}
	if body["step"] != "Transcoding" {
		t.Errorf("step = %v, want %q", body["step"], "Transcoding")
	}

	// Complete the job.
	completeUploadJob(id, nil)
	rec = doJSON(e, http.MethodGet, "/api/library/upload/jobs/"+id, "")
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != JobDone {
		t.Errorf("status = %v, want %q", body["status"], JobDone)
	}
	if body["percent"] != float64(100) {
		t.Errorf("percent = %v, want 100", body["percent"])
	}
}

func TestUploadJobError(t *testing.T) {
	_, e, _ := setupTestHandlers(t)
	resetUploadJobs(t)

	job := newUploadJob("bad-audio", "uploads", "corrupt.mp3")
	id := job.ID

	failUploadJob(id, "ffmpeg: invalid data found")

	rec := doJSON(e, http.MethodGet, "/api/library/upload/jobs/"+id, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != JobError {
		t.Errorf("status = %v, want %q", body["status"], JobError)
	}
	if body["error"] != "ffmpeg: invalid data found" {
		t.Errorf("error = %v, want %q", body["error"], "ffmpeg: invalid data found")
	}
}

func TestUploadJobCleanup(t *testing.T) {
	resetUploadJobs(t)

	job := newUploadJob("old-audio", "uploads", "old.mp3")
	completeUploadJob(job.ID, nil)

	// Manually set done_at to the past to simulate an old job.
	uploadJobsMu.Lock()
	old := time.Now().Add(-20 * time.Minute)
	uploadJobs[job.ID].DoneAt = &old
	uploadJobsMu.Unlock()

	cleanupOldUploadJobs(10 * time.Minute)

	if getUploadJob(job.ID) != nil {
		t.Error("old job should have been cleaned up")
	}
}
