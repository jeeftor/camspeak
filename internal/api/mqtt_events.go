package api

import (
	"encoding/json"
	"net/http"
	"sort"
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

// seenTopic tracks the latest message seen on a topic.
type seenTopic struct {
	Topic   string          `json:"topic"`
	Count   int             `json:"count"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Raw     string          `json:"raw,omitempty"`
	At      time.Time       `json:"at"`
}

// mqttMsgBus fans out raw MQTT messages to SSE subscribers and tracks seen topics.
type mqttMsgBus struct {
	mu          sync.Mutex
	subscribers map[chan mqttMsg]struct{}
	topicMu     sync.Mutex
	seenTopics  map[string]*seenTopic
}

func newMQTTMsgBus() *mqttMsgBus {
	return &mqttMsgBus{
		subscribers: make(map[chan mqttMsg]struct{}),
		seenTopics:  make(map[string]*seenTopic),
	}
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

	// Track seen topics
	b.topicMu.Lock()
	st, ok := b.seenTopics[topic]
	if !ok {
		st = &seenTopic{Topic: topic}
		b.seenTopics[topic] = st
	}
	st.Count++
	st.At = msg.At
	st.Payload = msg.Payload
	st.Raw = msg.Raw
	b.topicMu.Unlock()

	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subscribers {
		select {
		case ch <- msg:
		default: // drop slow subscribers
		}
	}
}

// SeenTopics returns a snapshot of all topics seen since startup, sorted by topic name.
func (b *mqttMsgBus) SeenTopics() []seenTopic {
	b.topicMu.Lock()
	defer b.topicMu.Unlock()
	out := make([]seenTopic, 0, len(b.seenTopics))
	for _, st := range b.seenTopics {
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Topic < out[j].Topic })
	return out
}

// SetMQTT wires the MQTT subscriber status and message hook into the handlers.
func (h *Handlers) SetMQTT(broker string, statusFn func() string) {
	h.mqttBroker = broker
	h.mqttStatusFn = statusFn
}

// SetMQTTSubscribeFn wires a callback that subscribes to an additional MQTT topic at runtime.
func (h *Handlers) SetMQTTSubscribeFn(fn func(string) error) {
	h.mqttSubscribeFn = fn
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

// MQTTTopics handles GET /api/mqtt/topics — returns all topics seen since startup.
func (h *Handlers) MQTTTopics(c echo.Context) error {
	return c.JSON(http.StatusOK, h.mqttMsgBus.SeenTopics())
}

// MQTTSubscribe handles POST /api/mqtt/subscribe — dynamically subscribes to a topic.
func (h *Handlers) MQTTSubscribe(c echo.Context) error {
	var req struct {
		Topic string `json:"topic"`
	}
	if err := c.Bind(&req); err != nil || req.Topic == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "topic required")
	}
	if h.mqttSubscribeFn == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "MQTT not configured")
	}
	if err := h.mqttSubscribeFn(req.Topic); err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "subscribed", "topic": req.Topic})
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
