package twin

import (
	"testing"
	"time"
)

func TestSubscriptionStartsWithSnapshotThenReceivesAcceptedEvents(t *testing.T) {
	s := NewStore("epoch", map[string][]string{"r": {"a"}})
	snap, ch, cancel, ok := s.Subscribe("r", 2)
	if !ok {
		t.Fatal("unknown run")
	}
	defer cancel()
	if snap.Revision != 0 {
		t.Fatal(snap.Revision)
	}
	e := ev(1, "e1", "workorder.started", nil, map[string]any{"workOrderId": "wo", "quantity": float64(1)})
	if _, err := s.Apply(e, time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case update := <-ch:
		if update.Revision != 1 || update.Event.EventID != "e1" {
			t.Fatalf("update: %+v", update)
		}
	case <-time.After(time.Second):
		t.Fatal("missed event")
	}
}

func TestSlowSubscriberIsClosed(t *testing.T) {
	s := NewStore("epoch", map[string][]string{"r": {"a"}})
	_, ch, cancel, _ := s.Subscribe("r", 1)
	defer cancel()
	_, _ = s.Apply(ev(1, "1", "workorder.started", nil, map[string]any{"workOrderId": "wo", "quantity": float64(1)}), time.Now())
	s.MarkUncertain("lost")
	<-ch
	_, ok := <-ch
	if ok {
		t.Fatal("slow subscriber not closed")
	}
}

func TestPublishedEventDoesNotAliasInput(t *testing.T) {
	s := NewStore("epoch", map[string][]string{"r": {"a"}})
	_, ch, cancel, _ := s.Subscribe("r", 2)
	defer cancel()
	payload := map[string]any{"workOrderId": "wo", "quantity": float64(1)}
	e := ev(1, "1", "workorder.started", nil, payload)
	at := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	_, err := s.Apply(e, at)
	if err != nil {
		t.Fatal(err)
	}
	payload["workOrderId"] = "mutated"
	u := <-ch
	if u.Event.Payload["workOrderId"] != "wo" || !u.IngestedAt.Equal(at) {
		t.Fatalf("published update: %+v", u)
	}
}
