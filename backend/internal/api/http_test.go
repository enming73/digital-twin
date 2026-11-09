package api

import (
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
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
	consumer.Connected()
	consumer.Subscribed()
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
