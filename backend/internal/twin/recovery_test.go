package twin

import (
	"testing"
	"time"

	"smt-digital-twin/backend/internal/contract"
)

func TestRecoveryModeRequiresTerminalVerification(t *testing.T) {
	store := NewStore("epoch-1", map[string][]string{"run-1": {"loader"}})
	store.SetRequireTerminal()
	event := contract.Event{SchemaVersion: 1, EventID: "event-00000001", RunID: "run-1", Source: "simulator", EventType: "workorder.started", Sequence: 1, SimTimeMs: 0, EmittedAt: "2026-11-10T00:00:00Z", Payload: map[string]any{"workOrderId": "wo-1", "quantity": float64(1), "synthetic": true}}
	if _, err := store.Apply(event, time.Now()); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := store.Snapshot("run-1")
	if snapshot.Integrity == "complete" {
		t.Fatal("recovery mode claimed complete before a verified terminal")
	}
	if snapshot.TerminalVerified {
		t.Fatal("recovery mode marked terminal verified without a terminal")
	}
}

func TestRecoveredEventsDoNotPublishLiveDeltas(t *testing.T) {
	store := NewStore("epoch-1", map[string][]string{"run-1": {"loader"}})
	_, updates, cancel, ok := store.Subscribe("run-1", 8)
	if !ok {
		t.Fatal("subscribe failed")
	}
	defer cancel()
	if err := store.BeginRecovery("run-1"); err != nil {
		t.Fatal(err)
	}
	event := contract.Event{SchemaVersion: 1, EventID: "event-00000001", RunID: "run-1", Source: "simulator", EventType: "workorder.started", Sequence: 1, SimTimeMs: 0, EmittedAt: "2026-11-10T00:00:00Z", Payload: map[string]any{"workOrderId": "wo-1", "quantity": float64(1), "synthetic": true}}
	if _, err := store.ApplyRecovered(event, time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case update := <-updates:
		t.Fatalf("recovered event published a live update: %+v", update)
	default:
	}
}

func TestPromoteCompletePushesNewEpochSnapshot(t *testing.T) {
	store := NewStore("epoch-1", map[string][]string{"run-1": {"loader"}})
	snapshot, updates, cancel, ok := store.Subscribe("run-1", 8)
	if !ok {
		t.Fatal("subscribe failed")
	}
	if snapshot.Integrity != "uncertain" {
		t.Fatalf("unexpected initial integrity %s", snapshot.Integrity)
	}
	quantity := 1.0
	event := contract.Event{SchemaVersion: 1, EventID: "event-00000001", RunID: "run-1", Source: "simulator", EventType: "workorder.started", Sequence: 1, SimTimeMs: 0, EmittedAt: "2026-11-10T00:00:00Z", Payload: map[string]any{"workOrderId": "wo-1", "quantity": quantity, "synthetic": true}}
	if _, err := store.Apply(event, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.PromoteComplete("run-1", 1); err != nil {
		t.Fatal(err)
	}
	defer cancel()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case update := <-updates:
			if update.Type != "snapshot" {
				continue
			}
			if update.Snapshot == nil {
				t.Fatal("snapshot update without a snapshot")
			}
			if update.Epoch == "epoch-1" || update.Snapshot.Epoch != update.Epoch {
				t.Fatalf("epoch not advanced: update=%s snapshot=%s", update.Epoch, update.Snapshot.Epoch)
			}
			if update.Snapshot.Integrity != "complete" || !update.Snapshot.TerminalVerified || update.Snapshot.LastSequence != 1 {
				t.Fatalf("snapshot not verified: %+v", update.Snapshot)
			}
			return
		case <-deadline:
			t.Fatal("no snapshot update after recovery promotion")
		}
	}
}

func TestPromoteCompleteRefusesMismatchedHead(t *testing.T) {
	store := NewStore("epoch-1", map[string][]string{"run-1": {"loader"}})
	event := contract.Event{SchemaVersion: 1, EventID: "event-00000001", RunID: "run-1", Source: "simulator", EventType: "workorder.started", Sequence: 1, SimTimeMs: 0, EmittedAt: "2026-11-10T00:00:00Z", Payload: map[string]any{"workOrderId": "wo-1", "quantity": float64(1), "synthetic": true}}
	if _, err := store.Apply(event, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.PromoteComplete("run-1", 5); err == nil {
		t.Fatal("promotion accepted a head the backend has not committed")
	}
	snapshot, _ := store.Snapshot("run-1")
	if snapshot.TerminalVerified {
		t.Fatal("unverified run marked terminal")
	}
}
