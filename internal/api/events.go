package api

import (
	"database/sql"
	"encoding/json"
	"net/url"
	"sync"
	"time"
)

// event represents playback activity for the persisted history and SSE log.
type event struct {
	ID     int64        `json:"id,omitempty"`
	Replay *eventReplay `json:"replay,omitempty"`
	Camera string       `json:"camera"`
	Action string       `json:"action"` // "speak", "play", "beep"
	Text   string       `json:"text,omitempty"`
	Voice  string       `json:"voice,omitempty"`
	At     time.Time    `json:"at"`
}

// eventReplay records only the public playback request, never authentication headers.
type eventReplay struct {
	Method   string         `json:"method"`
	Path     string         `json:"path"`
	Body     map[string]any `json:"body"`
	Redacted bool           `json:"redacted,omitempty"`
}

func playbackReplay(path string, body map[string]any, gain float64) *eventReplay {
	if gain >= 0 {
		body["gain"] = gain
	}
	r := &eventReplay{Method: "POST", Path: path, Body: body}
	if raw, ok := body["url"].(string); ok {
		if u, err := url.Parse(raw); err != nil {
			body["url"] = "[invalid URL removed]"
			r.Redacted = true
		} else if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			u.User = nil
			u.RawQuery = ""
			u.Fragment = ""
			body["url"] = u.String()
			r.Redacted = true
		}
	}
	return r
}

// eventBus is a simple pub/sub for SSE clients with SQLite persistence.
type eventBus struct {
	mu          sync.Mutex
	subscribers map[chan event]struct{}
	db          *sql.DB
}

func newEventBus(db *sql.DB) *eventBus {
	return &eventBus{
		subscribers: make(map[chan event]struct{}),
		db:          db,
	}
}

func (b *eventBus) subscribe() chan event {
	ch := make(chan event, 8)

	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	return ch
}

func (b *eventBus) unsubscribe(ch chan event) {
	b.mu.Lock()
	delete(b.subscribers, ch)
	b.mu.Unlock()
}

// publish persists the event to SQLite and broadcasts to SSE subscribers.
func (b *eventBus) publish(ev event) {
	if ev.Replay == nil {
		switch ev.Action {
		case "stop", "pause", "resume", "beep":
			ev.Replay = playbackReplay("/api/"+ev.Action, map[string]any{"camera": ev.Camera}, -1)
		case "stop-all":
			ev.Replay = playbackReplay("/api/stop", map[string]any{}, -1)
		}
	}
	if ev.Action == "play-url" || ev.Action == "play-stream" || ev.Action == "pause" ||
		ev.Action == "resume" {
		clean := playbackReplay("", map[string]any{"url": ev.Text}, -1)
		ev.Text, _ = clean.Body["url"].(string)
	}
	// Persist to SQLite (best-effort, don't block on DB errors)
	if b.db != nil {
		replay, _ := json.Marshal(ev.Replay)
		result, err := b.db.Exec(
			`INSERT INTO events (camera, action, text, voice, created, replay) VALUES (?, ?, ?, ?, ?, ?)`,
			ev.Camera,
			ev.Action,
			ev.Text,
			ev.Voice,
			ev.At,
			string(replay),
		)
		if err == nil {
			ev.ID, _ = result.LastInsertId()
		}
	}

	// Broadcast to SSE subscribers
	b.mu.Lock()
	defer b.mu.Unlock()

	for ch := range b.subscribers {
		select {
		case ch <- ev:
		default: // drop if subscriber is slow
		}
	}
}

// recentEvents returns up to limit events from the SQLite log.
func (b *eventBus) recentEvents(limit int) ([]event, error) {
	if b.db == nil {
		return nil, nil
	}

	rows, err := b.db.Query(
		`SELECT id, camera, action, text, voice, created, replay FROM events
		 ORDER BY created DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var events []event

	for rows.Next() {
		var ev event
		var replay string
		err := rows.Scan(&ev.ID, &ev.Camera, &ev.Action, &ev.Text, &ev.Voice, &ev.At, &replay)
		if err != nil {
			return nil, err
		}

		_ = json.Unmarshal([]byte(replay), &ev.Replay)
		events = append(events, ev)
	}

	return events, rows.Err()
}

// queryEvents returns events from the SQLite log with optional filtering.
// limit caps the result (default 100, max 1000). camera filters by name.
func (b *eventBus) queryEvents(limit int, camera string) ([]event, error) {
	if b.db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	var (
		rows *sql.Rows
		err  error
	)
	if camera != "" {
		rows, err = b.db.Query(
			`SELECT id, camera, action, text, voice, created, replay FROM events
			 WHERE camera = ? ORDER BY created DESC LIMIT ?`,
			camera, limit,
		)
	} else {
		rows, err = b.db.Query(
			`SELECT id, camera, action, text, voice, created, replay FROM events
			 ORDER BY created DESC LIMIT ?`,
			limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []event
	for rows.Next() {
		var ev event
		var replay string
		if err := rows.Scan(&ev.ID, &ev.Camera, &ev.Action, &ev.Text, &ev.Voice, &ev.At, &replay); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(replay), &ev.Replay)
		events = append(events, ev)
	}
	return events, rows.Err()
}
