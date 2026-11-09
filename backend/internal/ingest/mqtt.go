package ingest

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"smt-digital-twin/backend/internal/contract"
	"smt-digital-twin/backend/internal/twin"
)

type Health struct {
	Connected  bool   `json:"connected"`
	Subscribed bool   `json:"subscribed"`
	LastError  string `json:"lastError,omitempty"`
}
type Consumer struct {
	mu     sync.Mutex
	store  *twin.Store
	health Health
}

func NewConsumer(store *twin.Store) *Consumer { return &Consumer{store: store} }
func (c *Consumer) Health() Health            { c.mu.Lock(); defer c.mu.Unlock(); return c.health }
func (c *Consumer) Connected() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.health.Connected = true
	c.health.Subscribed = false
}
func (c *Consumer) Subscribed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.health.Subscribed = true
	c.health.LastError = ""
}
func (c *Consumer) Disconnected(reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.health.Connected = false
	c.health.Subscribed = false
	c.health.LastError = reason
	c.store.MarkUncertain(reason)
}

// Start blocks until cancellation. Paho's ordered callbacks apply one message at
// a time; no application queue can overflow and silently discard an event.
func Start(ctx context.Context, brokerURL string, store *twin.Store) error {
	return NewConsumer(store).Run(ctx, brokerURL)
}
func (c *Consumer) Run(ctx context.Context, brokerURL string) error {
	opts := mqtt.NewClientOptions().AddBroker(brokerURL).SetClientID(fmt.Sprintf("twin-%d", time.Now().UnixNano())).SetAutoReconnect(true).SetConnectRetry(true).SetConnectRetryInterval(250 * time.Millisecond).SetOrderMatters(true)
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) { c.Disconnected("broker disconnected: " + err.Error()) })
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		c.Connected()
		token := client.SubscribeMultiple(map[string]byte{"demo/smt/+/machines/+/events": 1, "demo/smt/+/line/events": 1}, func(_ mqtt.Client, msg mqtt.Message) {
			event, err := contract.DecodeAndValidate(msg.Topic(), msg.Payload())
			if err != nil {
				log.Printf("reject MQTT message: %v", err)
				c.store.MarkUncertain("invalid MQTT message")
				return
			}
			if _, err = c.store.Apply(event, time.Now()); err != nil {
				log.Printf("reject event %s/%s: %v", event.RunID, event.EventID, err)
			}
		})
		if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
			c.Disconnected("MQTT subscribe failed")
			return
		}
		c.Subscribed()
	})
	client := mqtt.NewClient(opts)
	token := client.Connect()
	for !token.WaitTimeout(250 * time.Millisecond) {
		select {
		case <-ctx.Done():
			client.Disconnect(250)
			return nil
		default:
		}
	}
	if err := token.Error(); err != nil {
		return err
	}
	<-ctx.Done()
	client.Disconnect(250)
	return nil
}
