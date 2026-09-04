package frigate

import "testing"

func TestClassifyCamera(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/Streaming/Channels/101", "hikvision"},
		{"/h264Preview_01_main", "reolink"},
		{"/Preview_01_main", "reolink"},
		{"/flv?port=1935", "reolink"},
		{"/stream0", "hikvision"},
		{"/unknown", "hikvision"}, // default
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := classifyCamera(tc.path)
			if got != tc.want {
				t.Errorf("classifyCamera(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestExtractChannel(t *testing.T) {
	cases := []struct {
		path string
		want int
	}{
		{"/Streaming/Channels/101", 1},
		{"/Streaming/Channels/201", 2},
		{"/Preview_01_main", 1},
		{"/Preview_02_sub", 2},
		{"/unknown", 1}, // default
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := extractChannel(tc.path)
			if got != tc.want {
				t.Errorf("extractChannel(%q) = %d, want %d", tc.path, got, tc.want)
			}
		})
	}
}

func TestStripStreamSuffix(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"front_main", "front"},
		{"front_sub", "front"},
		{"backyard_main", "backyard"},
		{"doorbell", "doorbell"}, // no suffix
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripStreamSuffix(tc.name)
			if got != tc.want {
				t.Errorf("stripStreamSuffix(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestExtractGo2rtcStreamName(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{"localhost restream", "rtsp://localhost:8554/front_door", "front_door"},
		{"127.0.0.1 restream", "rtsp://127.0.0.1:8554/backyard", "backyard"},
		{"non-local host", "rtsp://192.168.1.1:8554/cam", ""},
		{"no path", "rtsp://localhost:8554", ""},
		{"invalid url", "://bad", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractGo2rtcStreamName(tc.path)
			if got != tc.want {
				t.Errorf("extractGo2rtcStreamName(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestParseRTSP(t *testing.T) {
	t.Run("hikvision with credentials", func(t *testing.T) {
		cam, ok := parseRTSP(
			"rtsp://admin:pass@192.168.1.100:554/Streaming/Channels/101",
			"front_main",
		)
		if !ok {
			t.Fatal("parseRTSP returned ok=false")
		}
		if cam.IP != "192.168.1.100" {
			t.Errorf("IP = %q, want %q", cam.IP, "192.168.1.100")
		}
		if cam.User != "admin" {
			t.Errorf("User = %q, want %q", cam.User, "admin")
		}
		if cam.Pass != "pass" {
			t.Errorf("Pass = %q, want %q", cam.Pass, "pass")
		}
		if cam.Type != "hikvision" {
			t.Errorf("Type = %q, want %q", cam.Type, "hikvision")
		}
		if cam.Channel != 1 {
			t.Errorf("Channel = %d, want 1", cam.Channel)
		}
		// Stream suffix stripped from frigate name
		if cam.Name != "front" {
			t.Errorf("Name = %q, want %q", cam.Name, "front")
		}
	})

	t.Run("reolink", func(t *testing.T) {
		cam, ok := parseRTSP("rtsp://admin:pass@192.168.1.50:554/h264Preview_01_main", "door_main")
		if !ok {
			t.Fatal("parseRTSP returned ok=false")
		}
		if cam.Type != "reolink" {
			t.Errorf("Type = %q, want %q", cam.Type, "reolink")
		}
		if cam.Channel != 1 {
			t.Errorf("Channel = %d, want 1", cam.Channel)
		}
	})

	t.Run("strips fragment", func(t *testing.T) {
		cam, ok := parseRTSP(
			"rtsp://admin:pass@192.168.1.100:554/Streaming/Channels/101#backchannel=1",
			"front_main",
		)
		if !ok {
			t.Fatal("parseRTSP returned ok=false")
		}
		if cam.IP != "192.168.1.100" {
			t.Errorf("IP = %q, want %q", cam.IP, "192.168.1.100")
		}
	})

	t.Run("invalid URL", func(t *testing.T) {
		_, ok := parseRTSP("://bad", "test")
		if ok {
			t.Error("expected ok=false for invalid URL")
		}
	})
}
