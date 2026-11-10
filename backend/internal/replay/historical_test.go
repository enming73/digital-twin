package replay

import (
	"testing"
	"time"

	"smt-digital-twin/backend/internal/contract"
	"smt-digital-twin/backend/internal/persistence"
)

func TestHistoricalSameTimeAndOriginalIngestion(t *testing.T) {
	station := "loader"
	at := time.Date(2026, 11, 9, 0, 1, 0, 0, time.UTC)
	events := []persistence.StoredEvent{
		{Event: contract.Event{SchemaVersion: 1, RunID: "r", EventID: "1", Sequence: 1, EventType: "workorder.started", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1)}}, IngestedAt: at},
		{Event: contract.Event{SchemaVersion: 1, RunID: "r", EventID: "2", Sequence: 2, SimTimeMs: 5, EventType: "pcb.entered", MachineID: &station, Payload: map[string]any{"workOrderId": "wo", "boardId": "b"}}, IngestedAt: at.Add(time.Second)},
		{Event: contract.Event{SchemaVersion: 1, RunID: "r", EventID: "3", Sequence: 3, SimTimeMs: 5, EventType: "pcb.process.completed", MachineID: &station, Payload: map[string]any{"workOrderId": "wo", "boardId": "b"}}, IngestedAt: at.Add(2 * time.Second)},
	}
	store, err := ReduceHistorical(events, "r", []string{"loader"}, 5)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, ok := store.Snapshot("r")
	if !ok || snapshot.LastSequence != 3 || !snapshot.LastIngestedAt.Equal(at.Add(2*time.Second)) || !snapshot.Boards["b"].Processed {
		t.Fatalf("snapshot: %+v", snapshot)
	}
	trace, ok := store.BoardTrace("r", "b")
	if !ok || len(trace.Entries) != 2 || !trace.Entries[0].IngestedAt.Equal(at.Add(time.Second)) {
		t.Fatalf("trace: %+v", trace)
	}
	before, err := ReduceHistorical(events, "r", []string{"loader"}, 4)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ = before.Snapshot("r")
	if snapshot.LastSequence != 1 || len(snapshot.Boards) != 0 {
		t.Fatalf("early snapshot: %+v", snapshot)
	}
}

func TestHistoricalRejectsGapAndUnknownVersion(t *testing.T) {
	for _, e := range []contract.Event{
		{SchemaVersion: 1, RunID: "r", EventID: "2", Sequence: 2, EventType: "workorder.started", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1)}},
		{SchemaVersion: 2, RunID: "r", EventID: "1", Sequence: 1, EventType: "workorder.started", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1)}},
	} {
		if _, err := ReduceHistorical([]persistence.StoredEvent{{Event: e}}, "r", []string{"loader"}, 10); err == nil {
			t.Fatalf("invalid event accepted: %+v", e)
		}
	}
}
