package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// mqttMsg is one raw MQTT message forwarded to the SSE browser.
type mqttMsg struct {
	Topic   string          `json:"topic"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Raw     string          `json:"raw,omitempty"` // non-JSON payloads
	At      time.Time       `json:"at"`
}

// mqttMsgBus fans out raw MQTT messages to SSE subscribers.
type mqttMsgBus struct {
	mu          sync.Mutex
	subscribers map[chan mqttMsg]struct{}
}

func newMQTTMsgBus() *mqttMsgBus {
	return &mqttMsgBus{subscribers: make(map[chan mqttMsg]struct{})}
}

func (b *mqttMsgBus) subscribe() chan mqttMsg {
	ch := make(chan mqttMsg, 16)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *mqttMsgBus) unsubscribe(ch chan mqttMsg) {
	b.mu.Lock()
	delete(b.subscribers, ch)
	b.mu.Unlock()
}

func (b *mqttMsgBus) publish(topic string, payload []byte) {
	msg := mqttMsg{Topic: topic, At: time.Now()}
	var raw json.RawMessage
	if json.Unmarshal(payload, &raw) == nil {
		msg.Payload = raw
	} else {
		msg.Raw = string(payload)
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subscribers {
		select {
		case ch <- msg:
		default: // drop slow subscribers
		}
	}
}

// SetMQTT wires the MQTT subscriber status and message hook into the handlers.
func (h *Handlers) SetMQTT(broker string, statusFn func() string) {
	h.mqttBroker = broker
	h.mqttStatusFn = statusFn
}

// HandleMQTTMessage is the hook called by mqtt.Subscriber for every message.
// Wire this with: mqttSub.SetMessageHook(srv.Handlers().HandleMQTTMessage)
func (h *Handlers) HandleMQTTMessage(topic string, payload []byte) {
	h.mqttMsgBus.publish(topic, payload)
}

// MQTTStatus handles GET /api/mqtt/status.
func (h *Handlers) MQTTStatus(c echo.Context) error {
	status := "not_configured"
	if h.mqttStatusFn != nil {
		status = h.mqttStatusFn()
	}
	return c.JSON(http.StatusOK, map[string]string{
		"status": status,
		"broker": h.mqttBroker,
	})
}

// MQTTEvents handles GET /api/mqtt/events — SSE stream of raw MQTT messages.
func (h *Handlers) MQTTEvents(c echo.Context) error {
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)
	c.Response().Flush()

	ch := h.mqttMsgBus.subscribe()
	defer h.mqttMsgBus.unsubscribe(ch)

	ctx := c.Request().Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg := <-ch:
			data, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			if _, err := c.Response().Write([]byte("data: " + string(data) + "\n\n")); err != nil {
				return nil
			}
			c.Response().Flush()
		}
	}
}
