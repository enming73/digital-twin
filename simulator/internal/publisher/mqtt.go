package publisher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"smt-digital-twin/simulator/internal/contract"
)

type Config struct {
	URL, ClientID      string
	RetryMin, RetryMax time.Duration
}
type transport interface {
	send(context.Context, string, []byte) error
	close()
}
type mqttSink struct {
	transport transport
	slot      chan struct{}
	min, max  time.Duration
	mu        sync.Mutex
	closed    bool
}

func newMQTT(t transport, min, max time.Duration) *mqttSink {
	s := &mqttSink{transport: t, slot: make(chan struct{}, 1), min: min, max: max}
	s.slot <- struct{}{}
	return s
}
func NewMQTT(c Config) (Sink, error) {
	if c.URL == "" || c.ClientID == "" {
		return nil, errors.New("MQTT URL and client ID required")
	}
	if c.RetryMin <= 0 {
		c.RetryMin = 100 * time.Millisecond
	}
	if c.RetryMax <= 0 {
		c.RetryMax = 5 * time.Second
	}
	if c.RetryMax < c.RetryMin {
		return nil, errors.New("invalid retry bounds")
	}
	opts := mqtt.NewClientOptions().AddBroker(c.URL).SetClientID(c.ClientID).SetAutoReconnect(false).SetConnectRetry(false).SetOrderMatters(true).SetConnectTimeout(3 * time.Second)
	return newMQTT(&pahoTransport{client: mqtt.NewClient(opts)}, c.RetryMin, c.RetryMax), nil
}
func (s *mqttSink) Publish(ctx context.Context, e contract.Event) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.slot:
	}
	defer func() { s.slot <- struct{}{} }()
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return errors.New("publisher closed")
	}
	if e.EmittedAt == "" {
		e.EmittedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := contract.Validate(e); err != nil {
		return err
	}
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	topic := fmt.Sprintf("demo/smt/%s/line/events", e.RunID)
	if e.MachineID != nil {
		topic = fmt.Sprintf("demo/smt/%s/machines/%s/events", e.RunID, *e.MachineID)
	}
	delay := s.min
	for {
		if err := s.transport.send(ctx, topic, body); err == nil {
			return nil
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
		if delay < s.max {
			delay *= 2
			if delay > s.max {
				delay = s.max
			}
		}
	}
}
func (s *mqttSink) Close(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.slot:
	}
	defer func() { s.slot <- struct{}{} }()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.transport.close()
	}
	return nil
}

type pahoTransport struct{ client mqtt.Client }

func wait(ctx context.Context, t mqtt.Token) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.Done():
		return t.Error()
	}
}
func (p *pahoTransport) send(ctx context.Context, topic string, body []byte) error {
	if !p.client.IsConnectionOpen() {
		if err := wait(ctx, p.client.Connect()); err != nil {
			return err
		}
	}
	return wait(ctx, p.client.Publish(topic, 1, false, body))
}
func (p *pahoTransport) close() { p.client.Disconnect(250) }
