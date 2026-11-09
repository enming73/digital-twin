package api

import (
	"context"
	"fmt"
	"github.com/gorilla/websocket"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"smt-digital-twin/backend/internal/ingest"
	"smt-digital-twin/backend/internal/twin"
	"strings"
	"testing"
	"time"
)

func TestIntegrationTwoWebsocketsReceiveW2Run(t *testing.T) {
	if os.Getenv("W3_MQTT_INTEGRATION") != "1" {
		t.Skip("set W3_MQTT_INTEGRATION=1")
	}
	schema, err := filepath.Abs("../../../contracts/event.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TWIN_SCHEMA_PATH", schema)
	config, _ := filepath.Abs("../../../deploy/mosquitto/mosquitto.conf")
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	name := fmt.Sprintf("w3-ws-%d", time.Now().UnixNano())
	out, err := exec.Command("docker", "run", "-d", "--name", name, "-p", fmt.Sprintf("127.0.0.1:%d:1883", port), "-v", config+":/mosquitto/config/mosquitto.conf:ro", "eclipse-mosquitto:2.0.21").CombinedOutput()
	if err != nil {
		t.Fatalf("docker: %s %v", out, err)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	store := twin.NewStore("epoch", map[string][]string{"demo-run": {"loader", "printer", "spi", "mounter", "reflow", "aoi", "unloader"}})
	consumer := ingest.NewConsumer(store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = consumer.Run(ctx, fmt.Sprintf("tcp://127.0.0.1:%d", port)) }()
	deadline := time.Now().Add(20 * time.Second)
	for !consumer.Health().Subscribed {
		if time.Now().After(deadline) {
			t.Fatalf("MQTT not ready: %+v", consumer.Health())
		}
		time.Sleep(20 * time.Millisecond)
	}
	server := httptest.NewServer(NewHandler(store, consumer))
	defer server.Close()
	type result struct {
		seq []int
		err error
	}
	results := make(chan result, 2)
	for range 2 {
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws/runs/demo-run", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		var first struct {
			Type     string `json:"type"`
			Revision int    `json:"revision"`
		}
		if err := conn.ReadJSON(&first); err != nil || first.Type != "snapshot" || first.Revision != 0 {
			t.Fatalf("initial snapshot %+v: %v", first, err)
		}
		go func(c *websocket.Conn) {
			_ = c.SetReadDeadline(time.Now().Add(45 * time.Second))
			seq := []int{}
			for {
				var msg struct {
					Type     string `json:"type"`
					Revision int    `json:"revision"`
					Event    struct {
						EventType string `json:"eventType"`
					} `json:"event"`
				}
				if err := c.ReadJSON(&msg); err != nil {
					results <- result{seq, err}
					return
				}
				if msg.Type != "delta" {
					results <- result{seq, fmt.Errorf("unexpected %s", msg.Type)}
					return
				}
				seq = append(seq, msg.Revision)
				if msg.Event.EventType == "workorder.completed" {
					results <- result{seq, nil}
					return
				}
			}
		}(conn)
	}
	cmd := exec.Command("go", "run", "./cmd/simulator", "run", "--scenario", "scenarios/normal.json", "--speed", "100000", "--mqtt-url", fmt.Sprintf("tcp://127.0.0.1:%d", port))
	cmd.Dir = "../../../simulator"
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("simulator: %s %v", out, err)
	}
	a, b := <-results, <-results
	if a.err != nil || b.err != nil {
		t.Fatalf("websockets: %v %v", a.err, b.err)
	}
	if !reflect.DeepEqual(a.seq, b.seq) || len(a.seq) < 100 {
		t.Fatalf("sequences differ: %d %d", len(a.seq), len(b.seq))
	}
	for i, n := range a.seq {
		if n != i+1 {
			t.Fatalf("revision %d at %d", n, i)
		}
	}
	snap, _ := store.Snapshot("demo-run")
	if snap.Integrity != "complete" || len(snap.Boards) != 100 {
		t.Fatalf("snapshot integrity %s boards %d", snap.Integrity, len(snap.Boards))
	}
}
