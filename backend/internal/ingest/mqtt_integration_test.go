package ingest

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"smt-digital-twin/backend/internal/twin"
)

func TestIntegrationW2AndDisconnect(t *testing.T) {
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
	name := fmt.Sprintf("w3-test-%d", time.Now().UnixNano())
	cmd := exec.Command("docker", "run", "-d", "--name", name, "-p", fmt.Sprintf("127.0.0.1:%d:1883", port), "-v", config+":/mosquitto/config/mosquitto.conf:ro", "eclipse-mosquitto:2.0.21")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("docker: %s %v", out, err)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	store := twin.NewStore("epoch", map[string][]string{"demo-run": {"loader", "printer", "spi", "mounter", "reflow", "aoi", "unloader"}, "duplicate": {"loader"}})
	consumer := NewConsumer(store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- consumer.Run(ctx, fmt.Sprintf("tcp://127.0.0.1:%d", port)) }()
	wait := func(cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for !cond() {
			if time.Now().After(deadline) {
				logs, _ := exec.Command("docker", "logs", name).CombinedOutput()
				t.Logf("broker logs: %s", logs)
				t.Fatalf("timed out: health=%+v", consumer.Health())
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	wait(func() bool { return consumer.Health().Subscribed })
	publisher := mqtt.NewClient(mqtt.NewClientOptions().AddBroker(fmt.Sprintf("tcp://127.0.0.1:%d", port)).SetClientID("w3-test-publisher"))
	token := publisher.Connect()
	token.Wait()
	if token.Error() != nil {
		t.Fatal(token.Error())
	}
	defer publisher.Disconnect(100)
	body := `{"schemaVersion":1,"eventId":"one","runId":"duplicate","source":"simulator","machineId":null,"eventType":"workorder.started","sequence":1,"simTimeMs":0,"emittedAt":"2026-11-08T12:00:00Z","payload":{"synthetic":true,"workOrderId":"wo","quantity":1}}`
	for range 2 {
		token = publisher.Publish("demo/smt/duplicate/line/events", 1, false, body)
		token.Wait()
		if token.Error() != nil {
			t.Fatal(token.Error())
		}
	}
	wait(func() bool { s, _ := store.Snapshot("duplicate"); return s.Revision == 1 })
	token = publisher.Publish("demo/smt/duplicate/line/events", 1, false, body)
	token.Wait()
	time.Sleep(100 * time.Millisecond)
	ds, _ := store.Snapshot("duplicate")
	if ds.Revision != 1 {
		t.Fatalf("duplicate revision: %d", ds.Revision)
	}
	sim := exec.Command("go", "run", "./cmd/simulator", "run", "--scenario", "scenarios/normal.json", "--speed", "100000", "--mqtt-url", fmt.Sprintf("tcp://127.0.0.1:%d", port))
	sim.Dir = "../../../simulator"
	if out, err := sim.CombinedOutput(); err != nil {
		t.Fatalf("simulator: %s %v", out, err)
	}
	wait(func() bool {
		snap, _ := store.Snapshot("demo-run")
		return snap.WorkOrders["work-order-1"].Status == "completed"
	})
	snap, _ := store.Snapshot("demo-run")
	if len(snap.Boards) != 100 || snap.Integrity != "complete" {
		t.Fatalf("boards=%d integrity=%s reason=%s", len(snap.Boards), snap.Integrity, snap.Reason)
	}
	if out, err := exec.Command("docker", "stop", name).CombinedOutput(); err != nil {
		t.Fatalf("docker stop: %s %v", out, err)
	}
	wait(func() bool { snap, _ := store.Snapshot("demo-run"); return snap.Integrity == "uncertain" })
	if out, err := exec.Command("docker", "start", name).CombinedOutput(); err != nil {
		t.Fatalf("docker start: %s %v", out, err)
	}
	wait(func() bool { return consumer.Health().Subscribed })
	snap, _ = store.Snapshot("demo-run")
	if snap.Integrity != "uncertain" {
		t.Fatal(snap.Integrity)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("consumer did not stop")
	}
}
