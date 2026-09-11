package api

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
)

type comparisonSpeaker struct {
	operationSpeaker
	modes  []string
	cancel context.CancelFunc
}

func (s *comparisonSpeaker) SendRaw(
	_ string,
	gain *cameras.GainController,
) (cameras.SendTiming, error) {
	s.modes = append(s.modes, "buffered")
	gain.RecordLevel(0.2)
	if s.cancel != nil {
		s.cancel()
	}
	return cameras.SendTiming{PlaybackMs: 10}, nil
}

func (s *comparisonSpeaker) StreamWithGainContext(
	_ context.Context,
	r io.Reader,
	gain *cameras.GainController,
) error {
	s.modes = append(s.modes, "streaming")
	buffer := make([]byte, 160)
	for {
		n, err := r.Read(buffer)
		if n > 0 {
			gain.RecordLevel(0.2)
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func TestSpeakerComparisonModesAndStop(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg required")
	}
	for _, scenario := range []struct {
		name          string
		stop, reverse bool
	}{{"complete", false, false}, {"reverse", false, true}, {"stop", true, false}} {
		t.Run(scenario.name, func(t *testing.T) {
			stop := scenario.stop
			h, _, _ := setupTestHandlers(t)
			pcm := bytes.Repeat([]byte{0, 32}, 2400)
			wav := make([]byte, 44+len(pcm))
			copy(wav, "RIFF")
			binary.LittleEndian.PutUint32(wav[4:], uint32(36+len(pcm)))
			copy(wav[8:], "WAVEfmt ")
			binary.LittleEndian.PutUint32(wav[16:], 16)
			binary.LittleEndian.PutUint16(wav[20:], 1)
			binary.LittleEndian.PutUint16(wav[22:], 1)
			binary.LittleEndian.PutUint32(wav[24:], 24000)
			binary.LittleEndian.PutUint32(wav[28:], 48000)
			binary.LittleEndian.PutUint16(wav[32:], 2)
			binary.LittleEndian.PutUint16(wav[34:], 16)
			copy(wav[36:], "data")
			binary.LittleEndian.PutUint32(wav[40:], uint32(len(pcm)))
			copy(wav[44:], pcm)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req map[string]any
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
				}
				if req["input"] != "same message" || req["voice"] != "same voice" {
					t.Errorf("mismatched inputs: %v", req)
				}
				if req["response_format"] == "pcm" {
					w.Header().Set("Content-Type", "audio/pcm")
					_, _ = w.Write(pcm)
				} else {
					w.Header().Set("Content-Type", "audio/wav")
					_, _ = w.Write(wav)
				}
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			speaker := &comparisonSpeaker{}
			if stop {
				speaker.cancel = cancel
			}
			op := beginOperation(ctx, "comparison-test", speaker, "tts-benchmark", "test")
			defer op.finish()
			cfg := config.Config{
				TTS: config.TTSConfig{URL: server.URL, DefaultVoice: "same voice", Streaming: false},
			}
			result, err := h.runSpeakerComparison(
				op,
				cfg,
				speakerBenchmarkRequest{
					Camera:         "comparison-test",
					Text:           "same message",
					StreamingFirst: scenario.reverse,
				},
				cameras.NewGainController(1),
				func(string, map[string]any) {},
			)
			if stop {
				if err == nil || len(speaker.modes) != 1 {
					t.Fatalf("Stop failed: %v %v", speaker.modes, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := "buffered,streaming"
			if scenario.reverse {
				want = "streaming,buffered"
			}
			if strings.Join(speaker.modes, ",") != want {
				t.Fatalf("wrong modes: %v", speaker.modes)
			}
			if result["buffered"] == nil || result["streaming"] == nil || cfg.TTS.Streaming {
				t.Fatal("missing result or changed config")
			}
		})
	}
}

func TestSpeakerBenchmarkRequiresConfirmation(t *testing.T) {
	h, e, _ := setupTestHandlers(t)
	e.POST("/benchmark", h.StartSpeakerBenchmark)
	r := httptest.NewRequest(
		http.MethodPost,
		"/benchmark",
		strings.NewReader(
			`{"camera":"front","preset":"test","text":"hello","sample_rate":24000,"channels":1}`,
		),
	)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unconfirmed playback accepted: %d", w.Code)
	}
}
