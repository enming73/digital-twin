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
	generation := c.Connected()
	c.Subscribed(generation)
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

func TestLateSubscriptionAcknowledgementCannotRestoreDisconnectedHealth(t *testing.T) {
	c := NewConsumer(twin.NewStore("epoch", map[string][]string{"r": {"a"}}))
	generation := c.Connected()
	c.Disconnected("connection lost during subscribe")
	c.Subscribed(generation)
	if h := c.Health(); h.Connected || h.Subscribed {
		t.Fatalf("stale acknowledgement made consumer ready: %+v", h)
	}
}

func TestOldAcknowledgementCannotCompleteNewConnection(t *testing.T) {
	c := NewConsumer(twin.NewStore("epoch", map[string][]string{"r": {"a"}}))
	old := c.Connected()
	c.Disconnected("lost")
	current := c.Connected()
	c.Subscribed(old)
	if c.Health().Subscribed {
		t.Fatal("old subscription acknowledged new connection")
	}
	c.Subscribed(current)
	if !c.Health().Subscribed {
		t.Fatal("current subscription not acknowledged")
	}
	c.subscriptionFailed(old, "old timeout")
	if !c.Health().Subscribed {
		t.Fatal("old subscription failure disconnected current session")
	}
}
