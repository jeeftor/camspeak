package mqtt

import (
	"encoding/json"
	"sync"
	"testing"

	clog "github.com/charmbracelet/log"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/logging"
)

// fakeMessage implements paho.Message for testing.
type fakeMessage struct {
	topic   string
	payload []byte
}

func (m *fakeMessage) Topic() string     { return m.topic }
func (m *fakeMessage) Payload() []byte   { return m.payload }
func (m *fakeMessage) Qos() byte         { return 0 }
func (m *fakeMessage) Retained() bool    { return false }
func (m *fakeMessage) MessageID() uint16 { return 0 }
func (m *fakeMessage) Duplicate() bool   { return false }
func (m *fakeMessage) Ack()              {}

func newSubscriber(rules []config.Rule, speak SpeakFunc, announce AnnounceFunc) *Subscriber {
	s := &Subscriber{
		cfg:      config.MQTTConfig{Broker: "test"},
		rules:    rules,
		speak:    speak,
		announce: announce,
		log:      logging.New("test", clog.DebugLevel),
	}
	return s
}

// TestAnnounceRuleDispatch verifies that when an MQTT message matches an
// announce rule (source_camera != ""), the AnnounceFunc is called with the
// correct source camera, targets, prompt, and voice.
func TestAnnounceRuleDispatch(t *testing.T) {
	var mu sync.Mutex
	var gotSource string
	var gotTargets []string
	var gotPrompt string
	var gotVoice string
	var callCount int

	announce := func(source string, targets []string, prompt, voice string) {
		mu.Lock()
		defer mu.Unlock()
		gotSource = source
		gotTargets = targets
		gotPrompt = prompt
		gotVoice = voice
		callCount++
	}

	speak := func(cameras []string, text, preset, voice string, loop int) {
		t.Error("speak should not be called for announce rule")
	}

	rules := []config.Rule{
		{
			Topic:        "frigate/events",
			Filter:       map[string]string{"type": "person"},
			Cameras:      []string{"frontyard", "backyard"},
			SourceCamera: "doorbell",
			Prompt:       "Describe who is at the door",
			Voice:        "af_sky",
		},
	}

	s := newSubscriber(rules, speak, announce)

	payload, _ := json.Marshal(map[string]any{
		"type": "person",
		"zone": "front",
	})
	s.handleMessage(nil, &fakeMessage{topic: "frigate/events", payload: payload})

	if callCount != 1 {
		t.Fatalf("announce call count = %d, want 1", callCount)
	}
	if gotSource != "doorbell" {
		t.Errorf("source = %q, want %q", gotSource, "doorbell")
	}
	if len(gotTargets) != 2 {
		t.Fatalf("targets len = %d, want 2", len(gotTargets))
	}
	if gotTargets[0] != "frontyard" || gotTargets[1] != "backyard" {
		t.Errorf("targets = %v, want [frontyard backyard]", gotTargets)
	}
	if gotPrompt != "Describe who is at the door" {
		t.Errorf("prompt = %q, want %q", gotPrompt, "Describe who is at the door")
	}
	if gotVoice != "af_sky" {
		t.Errorf("voice = %q, want %q", gotVoice, "af_sky")
	}
}

// TestStandardRuleDispatch verifies that a standard rule (no source_camera)
// calls SpeakFunc, not AnnounceFunc.
func TestStandardRuleDispatch(t *testing.T) {
	var mu sync.Mutex
	var gotCameras []string
	var gotText string
	var gotPreset string
	var gotVoice string
	var gotLoop int
	var callCount int

	speak := func(cameras []string, text, preset, voice string, loop int) {
		mu.Lock()
		defer mu.Unlock()
		gotCameras = cameras
		gotText = text
		gotPreset = preset
		gotVoice = voice
		gotLoop = loop
		callCount++
	}

	announce := func(source string, targets []string, prompt, voice string) {
		t.Error("announce should not be called for standard rule")
	}

	rules := []config.Rule{
		{
			Topic:   "frigate/events",
			Filter:  map[string]string{"type": "car"},
			Cameras: []string{"frontyard"},
			Text:    "Vehicle detected",
			Voice:   "af_sky",
			Loop:    1,
		},
	}

	s := newSubscriber(rules, speak, announce)

	payload, _ := json.Marshal(map[string]any{"type": "car"})
	s.handleMessage(nil, &fakeMessage{topic: "frigate/events", payload: payload})

	if callCount != 1 {
		t.Fatalf("speak call count = %d, want 1", callCount)
	}
	if len(gotCameras) != 1 || gotCameras[0] != "frontyard" {
		t.Errorf("cameras = %v, want [frontyard]", gotCameras)
	}
	if gotText != "Vehicle detected" {
		t.Errorf("text = %q, want %q", gotText, "Vehicle detected")
	}
	if gotPreset != "" {
		t.Errorf("preset = %q, want empty", gotPreset)
	}
	if gotVoice != "af_sky" {
		t.Errorf("voice = %q, want %q", gotVoice, "af_sky")
	}
	if gotLoop != 1 {
		t.Errorf("loop = %d, want 1", gotLoop)
	}
}

// TestAnnounceRuleNoMatch verifies that a non-matching filter does not
// trigger the announce callback.
func TestAnnounceRuleNoMatch(t *testing.T) {
	announce := func(source string, targets []string, prompt, voice string) {
		t.Error("announce should not be called for non-matching filter")
	}
	speak := func(cameras []string, text, preset, voice string, loop int) {
		t.Error("speak should not be called for non-matching filter")
	}

	rules := []config.Rule{
		{
			Topic:        "frigate/events",
			Filter:       map[string]string{"type": "person"},
			Cameras:      []string{"front"},
			SourceCamera: "doorbell",
		},
	}

	s := newSubscriber(rules, speak, announce)
	payload, _ := json.Marshal(map[string]any{"type": "car"})
	s.handleMessage(nil, &fakeMessage{topic: "frigate/events", payload: payload})
}

// TestAnnounceRuleNilAnnounceFunc verifies that a nil announce callback
// does not panic when an announce rule matches.
func TestAnnounceRuleNilAnnounceFunc(t *testing.T) {
	speak := func(cameras []string, text, preset, voice string, loop int) {
		t.Error("speak should not be called for announce rule")
	}

	rules := []config.Rule{
		{
			Topic:        "frigate/events",
			Cameras:      []string{"front"},
			SourceCamera: "doorbell",
		},
	}

	s := newSubscriber(rules, speak, nil)
	payload, _ := json.Marshal(map[string]any{})
	s.handleMessage(nil, &fakeMessage{topic: "frigate/events", payload: payload})
}

// TestMatchFilterNested verifies nested key path matching.
func TestMatchFilterNested(t *testing.T) {
	payload := map[string]any{
		"before": map[string]any{
			"label": "person",
		},
	}
	if !matchFilter(map[string]string{"before.label": "person"}, payload) {
		t.Error("nested filter should match")
	}
	if matchFilter(map[string]string{"before.label": "car"}, payload) {
		t.Error("nested filter should not match with wrong value")
	}
}

// TestMatchTopicWildcard verifies MQTT wildcard topic matching.
func TestMatchTopicWildcard(t *testing.T) {
	tests := []struct {
		pattern, topic string
		want           bool
	}{
		{"frigate/events", "frigate/events", true},
		{"frigate/#", "frigate/events", true},
		{"frigate/#", "frigate/events/camera1", true},
		{"frigate/+", "frigate/events", true},
		{"frigate/+", "frigate/events/camera1", false},
		{"frigate/events", "frigate/alerts", false},
	}
	for _, tc := range tests {
		if got := matchTopic(tc.pattern, tc.topic); got != tc.want {
			t.Errorf("matchTopic(%q, %q) = %v, want %v", tc.pattern, tc.topic, got, tc.want)
		}
	}
}
