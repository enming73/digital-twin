package api

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"smt-digital-twin/backend/internal/contract"
	"smt-digital-twin/backend/internal/ingest"
	"smt-digital-twin/backend/internal/twin"
	"strings"
	"testing"
	"time"
)

func TestHTTPAndTwoWebsocketSubscribers(t *testing.T) {
	store := twin.NewStore("epoch", map[string][]string{"r": {"a"}})
	consumer := ingest.NewConsumer(store)
	server := httptest.NewServer(NewHandler(store, consumer))
	defer server.Close()
	res, err := http.Get(server.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 503 {
		t.Fatalf("health: %d", res.StatusCode)
	}
	generation := consumer.Connected()
	consumer.Subscribed(generation)
	res, err = http.Get(server.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("health: %d", res.StatusCode)
	}
	clients := make([]*websocket.Conn, 2)
	for i := range clients {
		c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws/runs/r", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		clients[i] = c
		var message map[string]any
		if err := c.ReadJSON(&message); err != nil || message["type"] != "snapshot" {
			t.Fatalf("snapshot: %v %v", message, err)
		}
	}
	_, err = store.Apply(contract.Event{RunID: "r", EventID: "one", Sequence: 1, EventType: "workorder.started", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1)}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range clients {
		_ = c.SetReadDeadline(time.Now().Add(time.Second))
		var message map[string]any
		if err := c.ReadJSON(&message); err != nil || message["type"] != "delta" || message["revision"] != float64(1) {
			t.Fatalf("delta: %v %v", message, err)
		}
	}
}

func TestRecoveryPromotionPushesSnapshotToConnectedClient(t *testing.T) {
	store := twin.NewStore("epoch", map[string][]string{"r": {"a"}})
	server := httptest.NewServer(NewHandler(store, ingest.NewConsumer(store)))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/ws/runs/r", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var initial map[string]any
	if err := conn.ReadJSON(&initial); err != nil || initial["type"] != "snapshot" {
		t.Fatalf("initial snapshot: %v %v", initial, err)
	}
	if _, err := store.Apply(contract.Event{RunID: "r", EventID: "one", Sequence: 1, EventType: "workorder.started", SchemaVersion: 1, Source: "simulator", EmittedAt: "2026-11-10T00:00:00Z", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1), "synthetic": true}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.PromoteComplete("r", 1); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for i := 0; i < 4; i++ {
		var message struct {
			Type     string         `json:"type"`
			Epoch    string         `json:"epoch"`
			Snapshot map[string]any `json:"snapshot"`
		}
		if err := conn.ReadJSON(&message); err != nil {
			t.Fatalf("read update: %v", err)
		}
		if message.Type != "snapshot" {
			continue
		}
		if message.Epoch == "epoch" || message.Snapshot["epoch"] != message.Epoch || message.Snapshot["terminalVerified"] != true {
			t.Fatalf("stale recovery snapshot: %+v", message)
		}
		return
	}
	t.Fatal("no new-epoch snapshot after recovery promotion")
}

func TestSlowWebsocketDisconnectsAndResynchronizes(t *testing.T) {
	store := twin.NewStore("epoch", map[string][]string{"r": {"a"}})
	server := httptest.NewServer(NewHandler(store, ingest.NewConsumer(store)))
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/runs/r"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var first map[string]any
	if err := conn.ReadJSON(&first); err != nil {
		t.Fatal(err)
	}
	machine := "a"
	reason := strings.Repeat("x", 128*1024)
	for i := 1; i <= 300; i++ {
		e := contract.Event{RunID: "r", EventID: fmt.Sprintf("%d", i), Sequence: i, EventType: "machine.state.changed", MachineID: &machine, Payload: map[string]any{"observedState": "RUNNING", "flowState": "NORMAL", "reason": reason}}
		if _, err := store.Apply(e, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	// Keep the real socket unread past the server's write deadline, then drain it.
	time.Sleep(6 * time.Second)
	_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
	for {
		var msg map[string]any
		err = conn.ReadJSON(&msg)
		if err != nil || msg["type"] == "resync_required" {
			break
		}
	}
	if nerr, ok := err.(net.Error); ok && nerr.Timeout() {
		t.Fatal("slow client did not disconnect")
	}
	newConn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer newConn.Close()
	var snapshot struct {
		Type     string `json:"type"`
		Revision int    `json:"revision"`
	}
	if err := newConn.ReadJSON(&snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Type != "snapshot" || snapshot.Revision != 300 {
		t.Fatalf("resync: %+v", snapshot)
	}
}

func TestAssetRegistryReportsShippedModels(t *testing.T) {
	store := twin.NewStore("epoch", map[string][]string{})
	consumer := ingest.NewConsumer(store)
	server := httptest.NewServer(NewHandler(store, consumer))
	defer server.Close()
	res, err := http.Get(server.URL + "/api/assets")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var list []struct {
		MachineID      string  `json:"machineId"`
		ModelURL       *string `json:"modelURL"`
		Representation string  `json:"representation"`
	}
	if err := json.NewDecoder(res.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 7 {
		t.Fatalf("assets: got %d entries", len(list))
	}
	for _, a := range list {
		if a.Representation != "glb" {
			t.Fatalf("%s reports %q", a.MachineID, a.Representation)
		}
		want := "/models/" + a.MachineID + ".glb"
		if a.ModelURL == nil || *a.ModelURL != want {
			t.Fatalf("%s reports model %v, want %s", a.MachineID, a.ModelURL, want)
		}
		if _, err := os.Stat("../../../public/models/" + a.MachineID + ".glb"); err != nil {
			t.Fatalf("%s is advertised but missing: %v", a.MachineID, err)
		}
	}
}
