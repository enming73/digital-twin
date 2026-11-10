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
	mu         sync.Mutex
	store      *twin.Store
	health     Health
	generation uint64
	durable    bool
}

func NewConsumer(store *twin.Store) *Consumer { return &Consumer{store: store} }

// NewDurableConsumer acknowledges MQTT messages only after Store.Apply commits.
func NewDurableConsumer(store *twin.Store) *Consumer { return &Consumer{store: store, durable: true} }

func (c *Consumer) processMessage(topic string, payload []byte, ack func()) error {
	event, err := contract.DecodeAndValidate(topic, payload)
	if err != nil {
		c.store.MarkUncertain("invalid MQTT message")
		return fmt.Errorf("reject MQTT message: %w", err)
	}
	if !c.store.Knows(event.RunID) {
		// The wildcard subscription sees other runs; discard without stopping.
		log.Printf("ignore event for unregistered run %s", event.RunID)
		if ack != nil {
			ack()
		}
		return nil
	}
	if _, err = c.store.Apply(event, time.Now()); err != nil {
		return fmt.Errorf("reject event %s/%s: %w", event.RunID, event.EventID, err)
	}
	if ack != nil {
		ack()
	}
	return nil
}
func (c *Consumer) Health() Health { c.mu.Lock(); defer c.mu.Unlock(); return c.health }
func (c *Consumer) Connected() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.health.Connected = true
	c.health.Subscribed = false
	c.generation++
	return c.generation
}
func (c *Consumer) Subscribed(generation uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.health.Connected || c.generation != generation {
		return
	}
	c.health.Subscribed = true
	c.health.LastError = ""
}
func (c *Consumer) Disconnected(reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.health.Connected = false
	c.generation++
	c.health.Subscribed = false
	c.health.LastError = reason
	c.store.MarkUncertain(reason)
}
func (c *Consumer) subscriptionFailed(generation uint64, reason string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.health.Connected || generation != c.generation {
		return
	}
	c.generation++
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
	if c.durable {
		opts.SetAutoAckDisabled(true)
	}
	failures := make(chan error, 1)
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) { c.Disconnected("broker disconnected: " + err.Error()) })
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		generation := c.Connected()
		token := client.SubscribeMultiple(map[string]byte{"demo/smt/+/machines/+/events": 1, "demo/smt/+/line/events": 1}, func(_ mqtt.Client, msg mqtt.Message) {
			var ack func()
			if c.durable {
				ack = msg.Ack
			}
			if err := c.processMessage(msg.Topic(), msg.Payload(), ack); err != nil {
				log.Print(err)
				if c.durable {
					c.Disconnected("durable ingestion stopped: " + err.Error())
					select {
					case failures <- err:
					default:
					}
				}
			}
		})
		if !token.WaitTimeout(10*time.Second) || token.Error() != nil {
			c.subscriptionFailed(generation, "MQTT subscribe failed")
			return
		}
		c.Subscribed(generation)
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
	select {
	case <-ctx.Done():
		client.Disconnect(250)
		return nil
	case err := <-failures:
		client.Disconnect(0)
		return err
	}
}
