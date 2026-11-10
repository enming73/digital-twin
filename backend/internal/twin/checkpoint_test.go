package twin

import (
	"fmt"
	"testing"
	"time"

	"smt-digital-twin/backend/internal/contract"
)

type captureCheckpoint struct{ values []*Checkpoint }

func (c *captureCheckpoint) SetIntegrity(_, _, _ string) error { return nil }
func (c *captureCheckpoint) Commit(_ contract.Event, _ time.Time, _ Snapshot, _ []TraceEntry, checkpoint *Checkpoint) error {
	if checkpoint != nil {
		c.values = append(c.values, checkpoint)
	}
	return nil
}

func TestCheckpointIsSentAtFiveHundredEvents(t *testing.T) {
	j := &captureCheckpoint{}
	s := NewStoreWithJournal("epoch", map[string][]string{"r": {"loader"}}, j)
	station := "loader"
	for i := 1; i <= 500; i++ {
		e := ev(i, fmt.Sprint(i), "machine.state.changed", &station, map[string]any{"observedState": "RUNNING", "flowState": "", "reason": ""})
		if _, err := s.Apply(e, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if len(j.values) != 1 || j.values[0].Snapshot.LastSequence != 500 || len(j.values[0].IDs) != 500 {
		t.Fatalf("checkpoints: %+v", j.values)
	}
}

func TestCheckpointRestoresTraceAndDeduplication(t *testing.T) {
	s := NewStore("old", map[string][]string{"r": {"loader"}})
	station := "loader"
	events := []struct {
		id, kind string
		machine  *string
		payload  map[string]any
	}{
		{"one", "workorder.started", nil, map[string]any{"workOrderId": "wo", "quantity": float64(1)}},
		{"two", "pcb.entered", &station, map[string]any{"workOrderId": "wo", "boardId": "b"}},
	}
	for i, x := range events {
		if _, err := s.Apply(ev(i+1, x.id, x.kind, x.machine, x.payload), time.Unix(int64(i+1), 0)); err != nil {
			t.Fatal(err)
		}
	}
	checkpoint, err := s.Checkpoint("r")
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreCheckpoint(checkpoint, "new")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, ok := restored.Snapshot("r")
	if !ok || snapshot.Epoch != "new" || snapshot.LastSequence != 2 || snapshot.Integrity != "uncertain" {
		t.Fatalf("snapshot: %+v", snapshot)
	}
	trace, ok := restored.BoardTrace("r", "b")
	if !ok || len(trace.Entries) != 1 || !trace.Entries[0].IngestedAt.Equal(time.Unix(2, 0)) {
		t.Fatalf("trace: %+v", trace)
	}
	result, err := restored.Apply(ev(2, "two", "pcb.entered", &station, events[1].payload), time.Now())
	if err != nil || !result.Duplicate {
		t.Fatalf("dedup: %+v %v", result, err)
	}
}

func TestCheckpointRejectsInconsistentIndex(t *testing.T) {
	s := NewStore("old", map[string][]string{"r": {"loader"}})
	if _, err := s.Apply(ev(1, "one", "workorder.started", nil, map[string]any{"workOrderId": "wo", "quantity": float64(1)}), time.Now()); err != nil {
		t.Fatal(err)
	}
	cp, err := s.Checkpoint("r")
	if err != nil {
		t.Fatal(err)
	}
	delete(cp.Sequences, 1)
	if _, err := RestoreCheckpoint(cp, "new"); err == nil {
		t.Fatal("corrupt checkpoint accepted")
	}
}
