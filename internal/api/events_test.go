package api

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appdb "github.com/jeeftor/camspeak/internal/db"
)

func TestPlaybackEventsPersistReplayAndStableID(t *testing.T) {
	database, err := appdb.Open(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	bus := newEventBus(database)
	ch := bus.subscribe()
	defer bus.unsubscribe(ch)
	bus.publish(
		event{
			Camera: "front",
			Action: "play",
			Text:   "It's music",
			At:     time.Now(),
			Replay: playbackReplay(
				"/api/play",
				map[string]any{
					"camera":   "front",
					"preset":   "It's music",
					"category": "alerts",
					"loop":     -1,
				},
				0,
			),
		},
	)
	live := <-ch
	history, err := bus.queryEvents(10, "front")
	if err != nil || len(history) != 1 {
		t.Fatalf("history=%v err=%v", history, err)
	}
	if live.ID == 0 || history[0].ID != live.ID {
		t.Fatal("history/live identity differs")
	}
	body := history[0].Replay.Body
	if body["gain"] != float64(0) || body["category"] != "alerts" || body["loop"] != float64(-1) ||
		body["preset"] != "It's music" {
		t.Fatalf("lost request options: %#v", body)
	}
	if _, err := database.Exec("INSERT INTO events(camera,action,text,voice,created) VALUES('old','play','old preset','',?)", time.Now()); err != nil {
		t.Fatal(err)
	}
	recent, err := bus.recentEvents(10)
	if err != nil || len(recent) != 2 || recent[0].Replay != nil {
		t.Fatalf("legacy history=%v err=%v", recent, err)
	}
}

func TestPlaybackEventURLCredentialsNeverPersistOrStream(t *testing.T) {
	database, err := appdb.Open(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	bus := newEventBus(database)
	ch := bus.subscribe()
	defer bus.unsubscribe(ch)
	raw := "https://alice:secret@example.com/radio?token=private#fragment"
	bus.publish(
		event{
			Camera: "front",
			Action: "play-stream",
			Text:   raw,
			At:     time.Now(),
			Replay: playbackReplay(
				"/api/play-stream",
				map[string]any{"camera": "front", "url": raw},
				-1,
			),
		},
	)
	live := <-ch
	history, err := bus.recentEvents(1)
	if err != nil {
		t.Fatal(err)
	}
	for _, ev := range []event{live, history[0]} {
		encoded, _ := json.Marshal(ev)
		for _, secret := range []string{"alice", "secret", "private", "fragment"} {
			if strings.Contains(string(encoded), secret) {
				t.Fatalf("credential leaked: %s", encoded)
			}
		}
		if !ev.Replay.Redacted || ev.Text != "https://example.com/radio" {
			t.Fatalf("missing redaction: %#v", ev)
		}
	}
}
