// Package mqtt subscribes to Frigate MQTT events and fires auto-speak rules.
package mqtt

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	clog "github.com/charmbracelet/log"
	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/jeeftor/camspeak/internal/config"
)

// SpeakFunc is called when a rule matches. It handles TTS or preset playback.
type SpeakFunc func(cameras []string, text, preset, voice string)

// MsgHook is called for every received MQTT message before rule matching.
type MsgHook func(topic string, payload []byte)

// Subscriber listens to MQTT and triggers SpeakFunc on rule matches.
type Subscriber struct {
	cfg     config.MQTTConfig
	rules   []config.Rule
	speak   SpeakFunc
	msgHook MsgHook
	client  paho.Client
	log     *clog.Logger
}

// New creates a Subscriber. Call Start() to connect.
func New(cfg config.MQTTConfig, rules []config.Rule, fn SpeakFunc) *Subscriber {
	return &Subscriber{
		cfg:   cfg,
		rules: rules,
		speak: fn,
		log:   clog.NewWithOptions(os.Stderr, clog.Options{Prefix: "mqtt"}),
	}
}

// Start connects to the MQTT broker and subscribes to all rule topics.
func (s *Subscriber) Start() error {
	if s.cfg.Broker == "" {
		s.log.Info("no MQTT broker configured, skipping")

		return nil
	}

	opts := paho.NewClientOptions().
		AddBroker(s.cfg.Broker).
		SetClientID("camspeak").
		SetUsername(s.cfg.User).
		SetPassword(s.cfg.Pass).
		SetAutoReconnect(true).
		SetOnConnectHandler(func(c paho.Client) {
			s.log.Info("connected", "broker", s.cfg.Broker)
			s.subscribeAll(c)
		}).
		SetConnectionLostHandler(func(_ paho.Client, err error) {
			s.log.Warn("connection lost", "err", err)
		})

	s.client = paho.NewClient(opts)
	if token := s.client.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("connecting to MQTT broker %s: %w", s.cfg.Broker, token.Error())
	}

	return nil
}

// Status returns the MQTT connection state: "not_configured", "connected", or "disconnected".
func (s *Subscriber) Status() string {
	if s.cfg.Broker == "" {
		return "not_configured"
	}
	if s.client != nil && s.client.IsConnected() {
		return "connected"
	}
	return "disconnected"
}

// Broker returns the configured broker URL (empty if not configured).
func (s *Subscriber) Broker() string { return s.cfg.Broker }

// SetMessageHook registers a callback invoked for every received MQTT message.
func (s *Subscriber) SetMessageHook(fn MsgHook) { s.msgHook = fn }

// SubscribeTopic dynamically subscribes to an additional topic at runtime.
// Safe to call after Start(); no-op if not connected.
func (s *Subscriber) SubscribeTopic(topic string) error {
	if s.client == nil || !s.client.IsConnected() {
		return fmt.Errorf("MQTT not connected")
	}
	token := s.client.Subscribe(topic, 1, s.handleMessage)
	token.Wait()
	if err := token.Error(); err != nil {
		return fmt.Errorf("subscribing to %s: %w", topic, err)
	}
	s.log.Info("subscribed (dynamic)", "topic", topic)
	return nil
}

// Stop disconnects from the broker.
func (s *Subscriber) Stop() {
	if s.client != nil && s.client.IsConnected() {
		s.client.Disconnect(500)
	}
}

func (s *Subscriber) subscribeAll(c paho.Client) {
	topics := map[string]struct{}{}
	for _, rule := range s.rules {
		topics[rule.Topic] = struct{}{}
	}

	for topic := range topics {
		c.Subscribe(topic, 1, s.handleMessage)
		s.log.Info("subscribed", "topic", topic)
	}
}

func (s *Subscriber) handleMessage(_ paho.Client, msg paho.Message) {
	if s.msgHook != nil {
		s.msgHook(msg.Topic(), msg.Payload())
	}

	var payload map[string]any
	if err := json.Unmarshal(msg.Payload(), &payload); err != nil {
		return
	}

	for _, rule := range s.rules {
		if rule.Topic != msg.Topic() && !matchTopic(rule.Topic, msg.Topic()) {
			continue
		}

		if !matchFilter(rule.Filter, payload) {
			continue
		}

		s.log.Info("rule matched", "topic", msg.Topic(), "cameras", rule.Cameras)
		s.speak(rule.Cameras, rule.Text, rule.Preset, rule.Voice)
	}
}

// matchFilter checks that all filter key=value pairs are present in the payload.
// Values are compared type-aware: JSON numbers, booleans, and strings are
// compared in their native form before falling back to string comparison.
func matchFilter(filter map[string]string, payload map[string]any) bool {
	for k, v := range filter {
		val := nestedGet(payload, strings.Split(k, "."))
		if !valueMatches(val, v) {
			return false
		}
	}

	return true
}

// valueMatches compares a JSON-decoded value against a filter string.
func valueMatches(val any, filter string) bool {
	switch v := val.(type) {
	case string:
		return v == filter
	case bool:
		return strconv.FormatBool(v) == filter
	case float64:
		// Handle integer and float comparisons
		if i := int64(v); float64(i) == v {
			return strconv.FormatInt(i, 10) == filter
		}

		return fmt.Sprintf("%g", v) == filter
	case nil:
		return filter == ""
	default:
		return fmt.Sprint(val) == filter
	}
}

// nestedGet traverses nested maps by key path.
func nestedGet(m map[string]any, keys []string) any {
	if len(keys) == 0 {
		return nil
	}

	val, ok := m[keys[0]]
	if !ok {
		return nil
	}

	if len(keys) == 1 {
		return val
	}

	if nested, ok := val.(map[string]any); ok {
		return nestedGet(nested, keys[1:])
	}

	return nil
}

// matchTopic does basic MQTT wildcard matching (# and +).
func matchTopic(pattern, topic string) bool {
	pp := strings.Split(pattern, "/")
	tp := strings.Split(topic, "/")

	return matchParts(pp, tp)
}

func matchParts(pp, tp []string) bool {
	if len(pp) == 0 && len(tp) == 0 {
		return true
	}

	if len(pp) == 0 {
		return false
	}

	if pp[0] == "#" {
		return true
	}

	if len(tp) == 0 {
		return false
	}

	if pp[0] == "+" || pp[0] == tp[0] {
		return matchParts(pp[1:], tp[1:])
	}

	return false
}
