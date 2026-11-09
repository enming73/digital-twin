package ingest

import (
	"smt-digital-twin/backend/internal/contract"
	"smt-digital-twin/backend/internal/twin"
	"testing"
	"time"
)

func TestDisconnectMakesRunUncertainAndSubsequentEventsDoNotClearIt(t *testing.T) {
	store := twin.NewStore("epoch", map[string][]string{"r": {"a"}})
	c := NewConsumer(store)
	if c.Health().Subscribed {
		t.Fatal("unconnected consumer ready")
	}
	c.Connected()
	c.Subscribed()
	if !c.Health().Subscribed {
		t.Fatal("subscription not ready")
	}
	c.Disconnected("lost broker")
	snap, _ := store.Snapshot("r")
	if snap.Integrity != "uncertain" {
		t.Fatalf("integrity = %s", snap.Integrity)
	}
	e := contract.Event{RunID: "r", EventID: "1", Sequence: 1, EventType: "workorder.started", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1)}}
	if _, err := store.Apply(e, time.Now()); err != nil {
		t.Fatal(err)
	}
	snap, _ = store.Snapshot("r")
	if snap.Integrity != "uncertain" {
		t.Fatalf("integrity = %s", snap.Integrity)
	}
}
