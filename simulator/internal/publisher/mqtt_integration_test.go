package publisher

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"smt-digital-twin/simulator/internal/contract"
)

type gatedFailureTransport struct {
	base   transport
	failed chan struct{}
	resume chan struct{}
	once   sync.Once
}

func (g *gatedFailureTransport) send(ctx context.Context, topic string, body []byte) error {
	err := g.base.send(ctx, topic, body)
	if err != nil {
		g.once.Do(func() { close(g.failed) })
		select {
		case <-g.resume:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}
func (g *gatedFailureTransport) close() { g.base.close() }

func TestIntegrationMQTTReconnect(t *testing.T) {
	if os.Getenv("W2_MQTT_INTEGRATION") != "1" {
		t.Skip("set W2_MQTT_INTEGRATION=1 to run Docker integration")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	config, err := filepath.Abs("../../../deploy/mosquitto/mosquitto.conf")
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("w2-mqtt-test-%d", time.Now().UnixNano())
	command := func(args ...string) {
		t.Helper()
		output, err := exec.Command("docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("docker %v: %v: %s", args, err, output)
		}
	}
	command("run", "-d", "--name", name, "-p", fmt.Sprintf("127.0.0.1:%d:1883", port), "-v", config+":/mosquitto/config/mosquitto.conf:ro", "eclipse-mosquitto:2.0.21")
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", name).Run() })
	broker := fmt.Sprintf("tcp://127.0.0.1:%d", port)
	connectSubscriber := func() (mqtt.Client, chan []byte) {
		t.Helper()
		received := make(chan []byte, 10)
		opts := mqtt.NewClientOptions().AddBroker(broker).SetClientID(fmt.Sprintf("subscriber-%d", time.Now().UnixNano())).SetAutoReconnect(false)
		c := mqtt.NewClient(opts)
		deadline := time.Now().Add(10 * time.Second)
		for {
			tok := c.Connect()
			tok.Wait()
			if tok.Error() == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal(tok.Error())
			}
			time.Sleep(100 * time.Millisecond)
		}
		tok := c.Subscribe("demo/smt/+/line/events", 1, func(_ mqtt.Client, m mqtt.Message) { received <- append([]byte(nil), m.Payload()...) })
		tok.Wait()
		if tok.Error() != nil {
			t.Fatal(tok.Error())
		}
		return c, received
	}
	sub, received := connectSubscriber()
	sink, err := NewMQTT(Config{URL: broker, ClientID: "integration-publisher", RetryMin: 50 * time.Millisecond, RetryMax: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	event := func(n int) contract.Event {
		return contract.Event{SchemaVersion: 1, EventID: fmt.Sprintf("event-%08d", n), RunID: "run", Source: "simulator", EventType: "workorder.started", Sequence: n, Payload: map[string]any{"synthetic": true, "workOrderId": "work-order-1", "quantity": 1}}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	schema, err := compiler.Compile("../../../contracts/event.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.Publish(ctx, event(1)); err != nil {
		t.Fatal(err)
	}
	check := func(ch chan []byte, sequence int) {
		t.Helper()
		select {
		case b := <-ch:
			var raw any
			if err := json.Unmarshal(b, &raw); err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(raw); err != nil {
				t.Fatalf("subscriber received invalid schema: %v", err)
			}
			var e contract.Event
			if err := json.Unmarshal(b, &e); err != nil {
				t.Fatal(err)
			}
			if err := contract.Validate(e); err != nil {
				t.Fatal(err)
			}
			if e.Sequence != sequence {
				t.Fatalf("sequence %d", e.Sequence)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	check(received, 1)
	late, lateReceived := connectSubscriber()
	select {
	case message := <-lateReceived:
		t.Fatalf("unexpected retained event: %s", message)
	case <-time.After(250 * time.Millisecond):
	}
	late.Disconnect(100)
	sub.Disconnect(100)
	command("stop", name)
	internal := sink.(*mqttSink)
	gate := &gatedFailureTransport{base: internal.transport, failed: make(chan struct{}), resume: make(chan struct{})}
	internal.transport = gate
	result := make(chan error, 1)
	go func() { result <- sink.Publish(ctx, event(2)) }()
	select {
	case err := <-result:
		t.Fatalf("publish finished during outage: %v", err)
	case <-gate.failed:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	command("start", name)
	sub, received = connectSubscriber()
	defer sub.Disconnect(100)
	close(gate.resume)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	check(received, 2)
	if err := sink.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
