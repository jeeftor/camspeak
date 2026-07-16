package api

import (
	"database/sql"
	"sync"
	"time"
)

// event represents a single speak action for the SSE log.
type event struct {
	Camera string    `json:"camera"`
	Action string    `json:"action"` // "speak", "play", "beep"
	Text   string    `json:"text,omitempty"`
	At     time.Time `json:"at"`
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
	// Persist to SQLite (best-effort, don't block on DB errors)
	if b.db != nil {
		_, _ = b.db.Exec(
			`INSERT INTO events (camera, action, text, created) VALUES (?, ?, ?, ?)`,
			ev.Camera, ev.Action, ev.Text, ev.At,
		)
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
		`SELECT camera, action, text, created FROM events
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
		err := rows.Scan(&ev.Camera, &ev.Action, &ev.Text, &ev.At)
		if err != nil {
			return nil, err
		}

		events = append(events, ev)
	}

	return events, rows.Err()
}
