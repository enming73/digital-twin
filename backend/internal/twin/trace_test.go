package twin

import (
	"fmt"
	"smt-digital-twin/backend/internal/contract"
	"testing"
	"time"
)

func TestTraceOrderDedupAndIsolation(t *testing.T) {
	s := NewStore("epoch", map[string][]string{"r": {"spi"}})
	m := "spi"
	at := time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC)
	events := []struct {
		id, kind string
		payload  map[string]any
	}{
		{"e2", "pcb.entered", map[string]any{"workOrderId": "wo", "boardId": "p"}},
		{"e3", "pcb.process.completed", map[string]any{"workOrderId": "wo", "boardId": "p"}},
		{"e4", "pcb.inspection.completed", map[string]any{"workOrderId": "wo", "boardId": "p", "passed": false}},
		{"e5", "pcb.exited", map[string]any{"workOrderId": "wo", "boardId": "p"}},
	}
	if _, err := s.Apply(ev(1, "e1", "workorder.started", nil, map[string]any{"workOrderId": "wo", "quantity": float64(1)}), at); err != nil {
		t.Fatal(err)
	}
	for i, item := range events {
		e := ev(i+2, item.id, item.kind, &m, item.payload)
		e.SimTimeMs = 42
		e.EmittedAt = "2026-01-01T00:00:00Z"
		if _, err := s.Apply(e, at); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Apply(e, at); err != nil {
			t.Fatal(err)
		}
	}
	trace, ok := s.BoardTrace("r", "p")
	if !ok || len(trace.Entries) != 4 || trace.Revision != 5 {
		t.Fatalf("trace: %+v", trace)
	}
	for i, entry := range trace.Entries {
		if entry.Sequence != i+2 || entry.EventID != events[i].id || entry.SimTimeMs != 42 || entry.EmittedAt != "2026-01-01T00:00:00Z" || !entry.IngestedAt.Equal(at) {
			t.Fatalf("entry: %+v", entry)
		}
	}
	if trace.Entries[2].Passed == nil || *trace.Entries[2].Passed {
		t.Fatal("missing failed inspection")
	}
	trace.Route[0] = "changed"
	trace.Entries[0].EventID = "changed"
	again, _ := s.BoardTrace("r", "p")
	if again.Route[0] != "spi" || again.Entries[0].EventID != "e2" {
		t.Fatal("trace exposes mutable state")
	}
	if _, ok := s.BoardTrace("r", "missing"); ok {
		t.Fatal("unknown board")
	}
}

func TestTraceRejectedEventsLeaveHistoryUnchanged(t *testing.T) {
	for _, variant := range []string{"duplicate-id", "sequence", "other-order", "skip", "repeat-process"} {
		t.Run(variant, func(t *testing.T) {
			s := NewStore("epoch", map[string][]string{"r": {"a", "b", "c"}})
			a, c := "a", "c"
			p := map[string]any{"workOrderId": "wo", "boardId": "p"}
			events := []contract.Event{ev(1, "e1", "workorder.started", nil, map[string]any{"workOrderId": "wo", "quantity": float64(1)}), ev(2, "e2", "pcb.entered", &a, p), ev(3, "e3", "pcb.process.completed", &a, p), ev(4, "e4", "pcb.exited", &a, p)}
			for _, e := range events {
				if _, err := s.Apply(e, time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			bad := ev(5, "e5", "pcb.entered", &c, p)
			switch variant {
			case "duplicate-id":
				bad.EventID = "e2"
			case "sequence":
				bad.Sequence = 2
			case "other-order":
				bad.Payload = map[string]any{"workOrderId": "unknown", "boardId": "p"}
			case "repeat-process":
				bad = ev(5, "e5", "pcb.process.completed", &a, p)
			}
			if _, err := s.Apply(bad, time.Now()); err == nil {
				t.Fatal("invalid event accepted")
			}
			trace, ok := s.BoardTrace("r", "p")
			if !ok || trace.Integrity != "conflict" || trace.Revision != 4 || len(trace.Entries) != 3 || trace.Board.Status != "in_transit" {
				t.Fatalf("trace: %+v", trace)
			}
		})
	}
}

func TestTraceRestartHasNoHistory(t *testing.T) {
	s := NewStore("new-epoch", map[string][]string{"r": {"a"}})
	if _, ok := s.BoardTrace("r", "p"); ok {
		t.Fatal("restart fabricated trace")
	}
	snapshot, _ := s.Snapshot("r")
	if snapshot.Integrity != "uncertain" || snapshot.Epoch != "new-epoch" {
		t.Fatalf("snapshot: %+v", snapshot)
	}
}

func TestTraceRejectsRepeatedInspection(t *testing.T) {
	s := NewStore("epoch", map[string][]string{"r": {"spi"}})
	m := "spi"
	for _, e := range []struct {
		id, kind string
		p        map[string]any
	}{
		{"1", "workorder.started", map[string]any{"workOrderId": "wo", "quantity": float64(1)}},
		{"2", "pcb.entered", map[string]any{"workOrderId": "wo", "boardId": "p"}},
		{"3", "pcb.process.completed", map[string]any{"workOrderId": "wo", "boardId": "p"}},
		{"4", "pcb.inspection.completed", map[string]any{"workOrderId": "wo", "boardId": "p", "passed": false}},
	} {
		seq := int(e.id[0] - '0')
		var machine *string
		if seq > 1 {
			machine = &m
		}
		if _, err := s.Apply(ev(seq, e.id, e.kind, machine, e.p), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	_, err := s.Apply(ev(5, "5", "pcb.inspection.completed", &m, map[string]any{"workOrderId": "wo", "boardId": "p", "passed": true}), time.Now())
	if err == nil {
		t.Fatal("repeated inspection accepted")
	}
	trace, _ := s.BoardTrace("r", "p")
	if trace.Integrity != "conflict" || trace.Revision != 4 || len(trace.Entries) != 3 {
		t.Fatalf("trace: %+v", trace)
	}
}

func TestProgressReconcile(t *testing.T) {
	s := NewStore("epoch", map[string][]string{"r": {"spi"}})
	m := "spi"
	seq := 0
	apply := func(kind string, machine *string, p map[string]any) {
		seq++
		if _, err := s.Apply(ev(seq, fmt.Sprintf("e%d", seq), kind, machine, p), time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	apply("workorder.started", nil, map[string]any{"workOrderId": "wo", "quantity": float64(4)})
	for i := 1; i <= 4; i++ {
		p := map[string]any{"workOrderId": "wo", "boardId": fmt.Sprintf("p%d", i)}
		apply("pcb.entered", &m, p)
		if i == 4 {
			continue
		}
		apply("pcb.process.completed", &m, p)
		if i != 3 {
			apply("pcb.inspection.completed", &m, map[string]any{"workOrderId": "wo", "boardId": fmt.Sprintf("p%d", i), "passed": i == 1})
		}
		apply("pcb.exited", &m, p)
	}
	detail, ok := s.WorkOrderDetail("r", "wo")
	if !ok || detail.Progress.Planned != 4 || detail.Progress.Created != 4 || detail.Progress.Completed != 3 || detail.Progress.InProgress != 1 || detail.Progress.Passed != 1 || detail.Progress.Failed != 1 || detail.Progress.Uninspected != 1 {
		t.Fatalf("detail: %+v", detail)
	}
	if _, ok := s.WorkOrderDetail("r", "missing"); ok {
		t.Fatal("unknown order exists")
	}
}
