package cameras

import (
	"testing"

	"github.com/jeeftor/camspeak/internal/config"
)

func TestNewSpeakerHikvision(t *testing.T) {
	cam := config.CameraConfig{
		Type:    "hikvision",
		IP:      "192.168.1.100",
		User:    "admin",
		Pass:    "pass",
		Channel: 1,
	}
	speaker, err := NewSpeaker(cam, "front", "", "")
	if err != nil {
		t.Fatalf("NewSpeaker: %v", err)
	}
	hc, ok := speaker.(*HikvisionClient)
	if !ok {
		t.Fatalf("expected *HikvisionClient, got %T", speaker)
	}
	if hc.ip != "192.168.1.100" {
		t.Errorf("ip = %q, want %q", hc.ip, "192.168.1.100")
	}
	if hc.channel != 1 {
		t.Errorf("channel = %d, want 1", hc.channel)
	}
}

func TestNewSpeakerReolinkWithGo2rtc(t *testing.T) {
	cam := config.CameraConfig{
		Type:   "reolink",
		IP:     "192.168.1.50",
		Stream: "doorbell_2way",
	}
	// Go2rtcStreamExists will try to connect to a non-existent go2rtc URL and
	// fail, but NewSpeaker still returns a Go2rtcClient.
	speaker, err := NewSpeaker(cam, "doorbell", "http://127.0.0.1:1", "")
	if err != nil {
		t.Fatalf("NewSpeaker: %v", err)
	}
	gc, ok := speaker.(*Go2rtcClient)
	if !ok {
		t.Fatalf("expected *Go2rtcClient, got %T", speaker)
	}
	if gc.stream != "doorbell_2way" {
		t.Errorf("stream = %q, want %q", gc.stream, "doorbell_2way")
	}
}

func TestNewSpeakerReolinkStub(t *testing.T) {
	cam := config.CameraConfig{
		Type: "reolink",
		IP:   "192.168.1.50",
	}
	speaker, err := NewSpeaker(cam, "doorbell", "", "")
	if err != nil {
		t.Fatalf("NewSpeaker: %v", err)
	}
	rc, ok := speaker.(*ReolinkClient)
	if !ok {
		t.Fatalf("expected *ReolinkClient (stub), got %T", speaker)
	}
	if rc.ip != "192.168.1.50" {
		t.Errorf("ip = %q, want %q", rc.ip, "192.168.1.50")
	}
}

func TestNewSpeakerGo2rtcNoURL(t *testing.T) {
	cam := config.CameraConfig{
		Type:   "go2rtc",
		IP:     "192.168.1.50",
		Stream: "test_stream",
	}
	_, err := NewSpeaker(cam, "cam", "", "")
	if err == nil {
		t.Fatal("expected error for go2rtc with no URL, got nil")
	}
}

func TestNewSpeakerGo2rtcNoStream(t *testing.T) {
	cam := config.CameraConfig{
		Type: "go2rtc",
		IP:   "192.168.1.50",
	}
	_, err := NewSpeaker(cam, "cam", "http://go2rtc:1984", "")
	if err == nil {
		t.Fatal("expected error for go2rtc with no stream, got nil")
	}
}

func TestNewSpeakerOnvif(t *testing.T) {
	cam := config.CameraConfig{
		Type:   "onvif",
		IP:     "192.168.1.195",
		Stream: "rtsp://admin:pass@192.168.1.195:554/stream0",
	}
	speaker, err := NewSpeaker(cam, "onvifcam", "", "")
	if err != nil {
		t.Fatalf("NewSpeaker: %v", err)
	}
	oc, ok := speaker.(*OnvifClient)
	if !ok {
		t.Fatalf("expected *OnvifClient, got %T", speaker)
	}
	if oc.rtspURL != "rtsp://admin:pass@192.168.1.195:554/stream0" {
		t.Errorf("rtspURL = %q, want %q", oc.rtspURL, "rtsp://admin:pass@192.168.1.195:554/stream0")
	}
}

func TestNewSpeakerOnvifBuildsURL(t *testing.T) {
	cam := config.CameraConfig{
		Type: "onvif",
		IP:   "192.168.1.195",
		User: "admin",
		Pass: "secret",
	}
	speaker, err := NewSpeaker(cam, "onvifcam", "", "")
	if err != nil {
		t.Fatalf("NewSpeaker: %v", err)
	}
	oc, ok := speaker.(*OnvifClient)
	if !ok {
		t.Fatalf("expected *OnvifClient, got %T", speaker)
	}
	want := "rtsp://admin:secret@192.168.1.195:554/stream0"
	if oc.rtspURL != want {
		t.Errorf("rtspURL = %q, want %q", oc.rtspURL, want)
	}
}

func TestNewSpeakerOnvifBuildsURLNoCreds(t *testing.T) {
	cam := config.CameraConfig{
		Type: "onvif",
		IP:   "192.168.1.195",
	}
	speaker, err := NewSpeaker(cam, "onvifcam", "", "")
	if err != nil {
		t.Fatalf("NewSpeaker: %v", err)
	}
	oc, ok := speaker.(*OnvifClient)
	if !ok {
		t.Fatalf("expected *OnvifClient, got %T", speaker)
	}
	want := "rtsp://192.168.1.195:554/stream0"
	if oc.rtspURL != want {
		t.Errorf("rtspURL = %q, want %q", oc.rtspURL, want)
	}
}

func TestNewSpeakerUnknownType(t *testing.T) {
	cam := config.CameraConfig{
		Type: "unknown",
		IP:   "192.168.1.1",
	}
	_, err := NewSpeaker(cam, "cam", "", "")
	if err == nil {
		t.Fatal("expected error for unknown camera type, got nil")
	}
}

func TestRegistryNames(t *testing.T) {
	cfg := &config.Config{
		Cameras: map[string]config.CameraConfig{
			"cam_a": {Type: "hikvision", IP: "10.0.0.1", Enabled: true},
			"cam_b": {Type: "hikvision", IP: "10.0.0.2", Enabled: true},
		},
	}
	r, err := NewRegistry(cfg, nil)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	names := r.Names()
	if len(names) != 2 {
		t.Fatalf("len(Names) = %d, want 2", len(names))
	}
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["cam_a"] {
		t.Error("cam_a not in Names()")
	}
	if !found["cam_b"] {
		t.Error("cam_b not in Names()")
	}
}

func TestRegistryStopNonExistent(t *testing.T) {
	cfg := &config.Config{
		Cameras: map[string]config.CameraConfig{
			"cam_a": {Type: "hikvision", IP: "10.0.0.1", Enabled: true},
		},
	}
	r, err := NewRegistry(cfg, nil)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	err = r.Stop("nonexistent")
	if err == nil {
		t.Fatal("expected error stopping non-existent camera, got nil")
	}
}

func TestFFmpegAvailable(t *testing.T) {
	// Just call it — it returns true or false depending on the system.
	_ = FFmpegAvailable()
}
