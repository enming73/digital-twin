package ingest

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"smt-digital-twin/backend/internal/contract"
	"smt-digital-twin/backend/internal/twin"
)

type testJournal struct{ fail bool }

func (j *testJournal) Commit(_ contract.Event, _ time.Time, _ twin.Snapshot, _ []twin.TraceEntry, _ *twin.Checkpoint) error {
	if j.fail {
		return errors.New("database unavailable")
	}
	return nil
}

func TestIntegrationDurableFailureStopsIngestion(t *testing.T) {
	if os.Getenv("W3_MQTT_INTEGRATION") != "1" {
		t.Skip("set W3_MQTT_INTEGRATION=1")
	}
	schema, err := filepath.Abs("../../../contracts/event.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TWIN_SCHEMA_PATH", schema)
	config, err := filepath.Abs("../../../deploy/mosquitto/mosquitto.conf")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	name := fmt.Sprintf("w6-ack-test-%d", time.Now().UnixNano())
	if out, err := exec.Command("docker", "run", "-d", "--rm", "--name", name, "-p", fmt.Sprintf("127.0.0.1:%d:1883", port), "-v", config+":/mosquitto/config/mosquitto.conf:ro", "eclipse-mosquitto:2.0.21").CombinedOutput(); err != nil {
		t.Fatalf("broker: %v %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "stop", name).Run() })
	store := twin.NewStoreWithJournal("epoch", map[string][]string{"r": {"loader"}}, &testJournal{fail: true})
	consumer := NewDurableConsumer(store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	url := fmt.Sprintf("tcp://127.0.0.1:%d", port)
	go func() { done <- consumer.Run(ctx, url) }()
	deadline := time.Now().Add(20 * time.Second)
	for !consumer.Health().Subscribed {
		if time.Now().After(deadline) {
			t.Fatal("subscriber not ready")
		}
		time.Sleep(20 * time.Millisecond)
	}
	publisher := mqtt.NewClient(mqtt.NewClientOptions().AddBroker(url).SetClientID("w6-test-publisher"))
	token := publisher.Connect()
	token.Wait()
	if token.Error() != nil {
		t.Fatal(token.Error())
	}
	defer publisher.Disconnect(100)
	body := `{"schemaVersion":1,"eventId":"one","runId":"r","source":"simulator","machineId":null,"eventType":"workorder.started","sequence":1,"simTimeMs":0,"emittedAt":"2026-11-08T12:00:00Z","payload":{"synthetic":true,"workOrderId":"wo","quantity":1}}`
	token = publisher.Publish("demo/smt/r/line/events", 1, false, body)
	token.Wait()
	if token.Error() != nil {
		t.Fatal(token.Error())
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("ingestion stopped without error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ingestion did not stop")
	}
	if consumer.Health().Subscribed {
		t.Fatal("failed ingestion still marked ready")
	}
	snapshot, _ := store.Snapshot("r")
	if snapshot.LastSequence != 0 {
		t.Fatal("failed storage changed projection")
	}
}
func (j *testJournal) SetIntegrity(_, _, _ string) error {
	if j.fail {
		return errors.New("database unavailable")
	}
	return nil
}

func TestIntegrationDurableReconnectAfterBrokerRestart(t *testing.T) {
	if os.Getenv("W3_MQTT_INTEGRATION") != "1" {
		t.Skip("set W3_MQTT_INTEGRATION=1")
	}
	schema, err := filepath.Abs("../../../contracts/event.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TWIN_SCHEMA_PATH", schema)
	config, err := filepath.Abs("../../../deploy/mosquitto/mosquitto.conf")
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	name := fmt.Sprintf("w6-reconnect-test-%d", time.Now().UnixNano())
	if out, err := exec.Command("docker", "run", "-d", "--name", name, "-p", fmt.Sprintf("127.0.0.1:%d:1883", port), "-v", config+":/mosquitto/config/mosquitto.conf:ro", "eclipse-mosquitto:2.0.21").CombinedOutput(); err != nil {
		t.Fatalf("broker: %v %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	store := twin.NewStoreWithJournal("epoch", map[string][]string{"r": {"loader"}}, &testJournal{})
	consumer := NewDurableConsumer(store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	url := fmt.Sprintf("tcp://127.0.0.1:%d", port)
	go func() { done <- consumer.Run(ctx, url) }()
	waitFor := func(cond func() bool, what string) {
		t.Helper()
		deadline := time.Now().Add(40 * time.Second)
		for !cond() {
			if time.Now().After(deadline) {
				t.Fatalf("timeout waiting for %s; health=%+v", what, consumer.Health())
			}
			select {
			case err := <-done:
				t.Fatalf("consumer exited while waiting for %s: %v", what, err)
			default:
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	waitFor(func() bool { return consumer.Health().Subscribed }, "initial subscription")
	newPublisher := func() mqtt.Client {
		client := mqtt.NewClient(mqtt.NewClientOptions().AddBroker(url).SetClientID(fmt.Sprintf("w6-reconnect-publisher-%d", time.Now().UnixNano())))
		token := client.Connect()
		token.Wait()
		if token.Error() != nil {
			t.Fatal(token.Error())
		}
		return client
	}
	publish := func(client mqtt.Client, sequence int) {
		body := fmt.Sprintf(`{"schemaVersion":1,"eventId":"e%d","runId":"r","source":"simulator","machineId":"loader","eventType":"machine.state.changed","sequence":%d,"simTimeMs":%d,"emittedAt":"2026-11-08T12:00:00Z","payload":{"synthetic":true,"observedState":"RUNNING","flowState":"NORMAL","reason":"simulated"}}`, sequence, sequence, sequence)
		token := client.Publish("demo/smt/r/machines/loader/events", 1, false, body)
		token.Wait()
		if token.Error() != nil {
			t.Fatal(token.Error())
		}
	}
	publisher := newPublisher()
	publish(publisher, 1)
	waitFor(func() bool { snapshot, _ := store.Snapshot("r"); return snapshot.LastSequence == 1 }, "first event")
	publisher.Disconnect(100)
	if out, err := exec.Command("docker", "stop", name).CombinedOutput(); err != nil {
		t.Fatalf("stop broker: %v %s", err, out)
	}
	if out, err := exec.Command("docker", "start", name).CombinedOutput(); err != nil {
		t.Fatalf("start broker: %v %s", err, out)
	}
	waitFor(func() bool { return consumer.Health().Subscribed }, "resubscription")
	publisher = newPublisher()
	defer publisher.Disconnect(100)
	publish(publisher, 2)
	waitFor(func() bool { snapshot, _ := store.Snapshot("r"); return snapshot.LastSequence == 2 }, "second event")
	snapshot, _ := store.Snapshot("r")
	if snapshot.Integrity != "uncertain" {
		t.Fatalf("broker loss did not leave uncertainty: %s", snapshot.Integrity)
	}
}

func TestDurableIgnoresOtherRuns(t *testing.T) {
	schema, err := filepath.Abs("../../../contracts/event.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TWIN_SCHEMA_PATH", schema)
	store := twin.NewStoreWithJournal("epoch", map[string][]string{"r": {"loader"}}, &testJournal{})
	c := NewDurableConsumer(store)
	body := []byte(`{"schemaVersion":1,"eventId":"other-1","runId":"other","source":"simulator","machineId":null,"eventType":"workorder.started","sequence":1,"simTimeMs":0,"emittedAt":"2026-11-08T12:00:00Z","payload":{"synthetic":true,"workOrderId":"wo","quantity":1}}`)
	acked := false
	if err := c.processMessage("demo/smt/other/line/events", body, func() { acked = true }); err != nil {
		t.Fatalf("other run stopped ingestion: %v", err)
	}
	if !acked {
		t.Fatal("other-run message was not acknowledged for discard")
	}
	snapshot, _ := store.Snapshot("r")
	if snapshot.LastSequence != 0 {
		t.Fatal("other run changed registered run")
	}
}

func TestDurableRejectsSimulatedTimeRegression(t *testing.T) {
	store := twin.NewStore("epoch", map[string][]string{"r": {"loader"}})
	e := contract.Event{RunID: "r", EventID: "one", Sequence: 1, SimTimeMs: 10, EventType: "workorder.started", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1)}}
	if _, err := store.Apply(e, time.Now()); err != nil {
		t.Fatal(err)
	}
	e.EventID, e.Sequence, e.SimTimeMs = "two", 2, 9
	if _, err := store.Apply(e, time.Now()); err == nil {
		t.Fatal("regressing simulated time accepted")
	}
	snapshot, _ := store.Snapshot("r")
	if snapshot.Integrity != "conflict" || snapshot.LastSequence != 1 {
		t.Fatalf("snapshot after regression: %+v", snapshot)
	}
}

func TestDurableRejectsUnknownSchema(t *testing.T) {
	schema, err := filepath.Abs("../../../contracts/event.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TWIN_SCHEMA_PATH", schema)
	store := twin.NewStoreWithJournal("epoch", map[string][]string{"r": {"loader"}}, &testJournal{})
	c := NewDurableConsumer(store)
	body := []byte(`{"schemaVersion":2,"eventId":"one","runId":"r","source":"simulator","machineId":null,"eventType":"workorder.started","sequence":1,"simTimeMs":0,"emittedAt":"2026-11-08T12:00:00Z","payload":{"synthetic":true,"workOrderId":"wo","quantity":1}}`)
	acked := false
	if err := c.processMessage("demo/smt/r/line/events", body, func() { acked = true }); err == nil {
		t.Fatal("unknown schema accepted")
	}
	if acked {
		t.Fatal("unknown schema acknowledged")
	}
}

func TestDurableAckOnlyAfterCommitAndOnDuplicate(t *testing.T) {
	schema, err := filepath.Abs("../../../contracts/event.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TWIN_SCHEMA_PATH", schema)
	j := &testJournal{fail: true}
	store := twin.NewStoreWithJournal("epoch", map[string][]string{"r": {"loader"}}, j)
	c := NewDurableConsumer(store)
	body := []byte(`{"schemaVersion":1,"eventId":"one","runId":"r","source":"simulator","machineId":null,"eventType":"workorder.started","sequence":1,"simTimeMs":0,"emittedAt":"2026-11-08T12:00:00Z","payload":{"synthetic":true,"workOrderId":"wo","quantity":1}}`)
	acks := 0
	ack := func() { acks++ }
	if err := c.processMessage("demo/smt/r/line/events", body, ack); err == nil {
		t.Fatal("storage failure ignored")
	}
	if acks != 0 {
		t.Fatal("acknowledged uncommitted event")
	}
	j.fail = false
	if err := c.processMessage("demo/smt/r/line/events", body, ack); err != nil {
		t.Fatal(err)
	}
	if err := c.processMessage("demo/smt/r/line/events", body, ack); err != nil {
		t.Fatal(err)
	}
	if acks != 2 {
		t.Fatalf("committed event and duplicate ack count = %d", acks)
	}
	snapshot, _ := store.Snapshot("r")
	if snapshot.Revision != 1 {
		t.Fatalf("duplicate changed revision: %d", snapshot.Revision)
	}
}
