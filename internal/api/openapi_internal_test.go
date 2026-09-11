package api

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/jeeftor/camspeak/internal/config"
)

func TestOpenAPISpecValidAndHasPauseResume(t *testing.T) {
	var v map[string]any
	if err := json.Unmarshal([]byte(openAPISpec), &v); err != nil {
		t.Fatalf("openAPISpec is not valid JSON: %v", err)
	}
	paths, ok := v["paths"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid 'paths'")
	}
	for _, p := range []string{
		"/pause", "/resume", "/stop", "/play-stream", "/playback",
		"/library/upload", "/library/upload/jobs/{id}",
		"/describe/jobs", "/describe/jobs/{id}",
	} {
		if _, ok := paths[p]; !ok {
			t.Errorf("missing path %q in openapi spec", p)
		}
	}
}

// TestOpenAPICameraResponseCoverage verifies the map-based camera response too,
// including capability keys used to decide which controls clients can offer.
func TestOpenAPICameraResponseCoverage(t *testing.T) {
	h, server, _ := setupTestHandlers(t)
	h.cfg.Cameras["front"] = config.CameraConfig{Type: "hikvision", Enabled: true, Gain: 0}
	response := doJSON(server, http.MethodGet, "/api/cameras", "")
	if response.Code != http.StatusOK {
		t.Fatalf("camera response: %d: %s", response.Code, response.Body.String())
	}
	var cameras []map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &cameras); err != nil {
		t.Fatal(err)
	}
	if len(cameras) != 1 {
		t.Fatalf("expected one camera, got %d", len(cameras))
	}
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal([]byte(openAPISpec), &spec); err != nil {
		t.Fatal(err)
	}
	for field := range cameras[0] {
		if _, ok := spec.Components.Schemas["Camera"].Properties[field]; !ok {
			t.Errorf("camera response field %s is undocumented", field)
		}
	}
	var capabilities map[string]bool
	if err := json.Unmarshal(cameras[0]["capabilities"], &capabilities); err != nil {
		t.Fatal(err)
	}
	for field := range capabilities {
		if _, ok := spec.Components.Schemas["CameraCapabilities"].Properties[field]; !ok {
			t.Errorf("camera capability %s is undocumented", field)
		}
	}
}

// TestOpenAPISchemasCoverDTOFields keeps documented properties aligned with the
// request and response types that clients actually exchange.
func TestOpenAPISchemasCoverDTOFields(t *testing.T) {
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal([]byte(openAPISpec), &spec); err != nil {
		t.Fatal(err)
	}
	for name, dto := range map[string]any{
		"SpeakRequest": speakReq{}, "PlayRequest": playReq{},
		"BroadcastRequest": broadcastReq{}, "GeneratePresetRequest": genPresetReq{},
		"PlaybackState": PlaybackState{}, "UploadJob": UploadJob{},
		"DescribeJob": DescribeJob{}, "DescribeRequest": describeRequest{},
		"VisionConfig": config.VisionConfig{}, "TTSPreset": config.TTSPreset{},
	} {
		t.Run(name, func(t *testing.T) {
			properties := spec.Components.Schemas[name].Properties
			if len(properties) == 0 {
				t.Fatalf("missing schema %s", name)
			}
			typ := reflect.TypeOf(dto)
			for i := range typ.NumField() {
				field := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
				if field == "" || field == "-" {
					continue
				}
				if _, ok := properties[field]; !ok {
					t.Errorf("%s.%s is missing from OpenAPI", name, field)
				}
			}
		})
	}
}

// TestOpenAPIReferencesResolve catches broken links that otherwise leave Swagger
// or generated clients with incomplete schemas despite valid JSON.
func TestOpenAPIReferencesResolve(t *testing.T) {
	var spec map[string]any
	if err := json.Unmarshal([]byte(openAPISpec), &spec); err != nil {
		t.Fatal(err)
	}
	var walk func(any)
	walk = func(node any) {
		switch value := node.(type) {
		case map[string]any:
			if ref, ok := value["$ref"].(string); ok {
				var target any = spec
				for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
					object, ok := target.(map[string]any)
					if !ok || object[part] == nil {
						t.Errorf("unresolved reference %s", ref)
						break
					}
					target = object[part]
				}
			}
			for _, child := range value {
				walk(child)
			}
		case []any:
			for _, child := range value {
				walk(child)
			}
		}
	}
	walk(spec)
}
