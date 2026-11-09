package twin

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os/exec"
	"smt-digital-twin/backend/internal/contract"
	"testing"
	"time"
)

func TestNormalSimulatorStream(t *testing.T) {
	cmd := exec.Command("go", "run", "./cmd/simulator", "run", "--scenario", "scenarios/normal.json", "--run-id", "r", "--speed", "100000", "--dry-run")
	cmd.Dir = "../../../simulator"
	data, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	s := NewStore("test", map[string][]string{"r": {"loader", "printer", "spi", "mounter", "reflow", "aoi", "unloader"}})
	scan := bufio.NewScanner(bytes.NewReader(data))
	for scan.Scan() {
		var event contract.Event
		if err := json.Unmarshal(scan.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Apply(event, time.Now()); err != nil {
			t.Fatalf("sequence %d %s: %v", event.Sequence, event.EventType, err)
		}
	}
	if err := scan.Err(); err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Snapshot("r")
	if len(snap.Boards) != 100 || snap.WorkOrders["work-order-1"].Status != "completed" {
		t.Fatalf("boards=%d order=%+v", len(snap.Boards), snap.WorkOrders["work-order-1"])
	}
	detail, ok := s.WorkOrderDetail("r", "work-order-1")
	if !ok || detail.Progress.Created != 100 || detail.Progress.Completed != 100 || detail.Progress.InProgress != 0 || detail.Progress.Uninspected != 0 || detail.Progress.Passed+detail.Progress.Failed != 100 || detail.Reconciliation != "matched" {
		t.Fatalf("work order: %+v", detail)
	}
	trace, ok := s.BoardTrace("r", "board-1")
	if !ok || trace.Board.WorkOrderID != "work-order-1" || len(trace.Entries) != 23 || trace.Entries[0].EventType != "pcb.entered" || trace.Entries[len(trace.Entries)-1].EventType != "pcb.exited" {
		t.Fatalf("trace: %+v", trace)
	}
	for i := 1; i < len(trace.Entries); i++ {
		if trace.Entries[i].Sequence <= trace.Entries[i-1].Sequence {
			t.Fatal("trace out of order")
		}
	}
}

func TestInspectionFixtureReconcilesFailedBoards(t *testing.T) {
	cmd := exec.Command("go", "run", "./cmd/simulator", "run", "--scenario", "scenarios/inspection.json", "--run-id", "r", "--speed", "100000", "--dry-run")
	cmd.Dir = "../../../simulator"
	data, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	s := NewStore("test", map[string][]string{"r": {"loader", "spi", "aoi", "unloader"}})
	scan := bufio.NewScanner(bytes.NewReader(data))
	for scan.Scan() {
		var e contract.Event
		if err := json.Unmarshal(scan.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Apply(e, time.Now()); err != nil {
			t.Fatalf("sequence %d: %v", e.Sequence, err)
		}
	}
	if err := scan.Err(); err != nil {
		t.Fatal(err)
	}
	detail, ok := s.WorkOrderDetail("r", "work-order-1")
	if !ok || detail.Progress.Completed != 10 || detail.Progress.Failed != 10 || detail.Progress.Passed != 0 || detail.Reconciliation != "matched" {
		t.Fatalf("detail: %+v", detail)
	}
}

func ev(seq int, id, kind string, machine *string, payload map[string]any) contract.Event {
	return contract.Event{RunID: "r", EventID: id, Sequence: seq, EventType: kind, MachineID: machine, Payload: payload}
}
func TestSequenceAndDedup(t *testing.T) {
	s := NewStore("epoch", map[string][]string{"r": {"a", "b"}, "other": {"a"}})
	first := ev(1, "e1", "workorder.started", nil, map[string]any{"workOrderId": "wo", "quantity": float64(1)})
	if _, err := s.Apply(first, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(first, time.Now()); err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Snapshot("r")
	if snap.Revision != 1 || snap.LastSequence != 1 {
		t.Fatalf("duplicate: %+v", snap)
	}
	second := ev(3, "e3", "workorder.started", nil, map[string]any{"workOrderId": "wo", "quantity": float64(1)})
	if _, err := s.Apply(second, time.Now()); err == nil {
		t.Fatal("missing sequence accepted")
	}
	snap, _ = s.Snapshot("r")
	if snap.Integrity != "gap" || snap.LastSequence != 1 {
		t.Fatalf("gap: %+v", snap)
	}
	if _, err := s.Apply(ev(2, "e2", "workorder.started", nil, map[string]any{}), time.Now()); err == nil {
		t.Fatal("frozen run accepted")
	}
	other, _ := s.Snapshot("other")
	if other.Integrity == "gap" {
		t.Fatal("gap crossed runs")
	}
}

func TestBoardTransitAndCompletion(t *testing.T) {
	s := NewStore("epoch", map[string][]string{"r": {"a", "b"}})
	a, b := "a", "b"
	events := []contract.Event{
		ev(1, "e1", "workorder.started", nil, map[string]any{"workOrderId": "wo", "quantity": float64(1)}),
		ev(2, "e2", "pcb.entered", &a, map[string]any{"workOrderId": "wo", "boardId": "p"}),
		ev(3, "e3", "pcb.process.completed", &a, map[string]any{"workOrderId": "wo", "boardId": "p"}),
		ev(4, "e4", "pcb.exited", &a, map[string]any{"workOrderId": "wo", "boardId": "p"}),
		ev(5, "e5", "pcb.entered", &b, map[string]any{"workOrderId": "wo", "boardId": "p"}),
		ev(6, "e6", "pcb.process.completed", &b, map[string]any{"workOrderId": "wo", "boardId": "p"}),
		ev(7, "e7", "pcb.exited", &b, map[string]any{"workOrderId": "wo", "boardId": "p"}),
		ev(8, "e8", "workorder.completed", nil, map[string]any{"workOrderId": "wo", "quantity": float64(1), "passed": float64(1), "failed": float64(0)}),
	}
	for i, e := range events {
		if _, err := s.Apply(e, time.Now()); err != nil {
			t.Fatalf("%d: %v", i, err)
		}
		if i == 3 {
			snap, _ := s.Snapshot("r")
			if snap.Boards["p"].Status != "in_transit" {
				t.Fatalf("board: %+v", snap.Boards["p"])
			}
		}
	}
	snap, _ := s.Snapshot("r")
	if snap.Boards["p"].Status != "completed" || snap.LastSequence != 8 {
		t.Fatalf("snapshot: %+v", snap)
	}
}

func TestConflictingDuplicatesFreezeRun(t *testing.T) {
	for _, tc := range []struct {
		name     string
		conflict contract.Event
	}{
		{"same id changed", ev(1, "e1", "workorder.started", nil, map[string]any{"workOrderId": "changed", "quantity": float64(1)})},
		{"same sequence new id", ev(1, "other", "workorder.started", nil, map[string]any{"workOrderId": "wo", "quantity": float64(1)})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStore("epoch", map[string][]string{"r": {"a"}})
			first := ev(1, "e1", "workorder.started", nil, map[string]any{"workOrderId": "wo", "quantity": float64(1)})
			if _, err := s.Apply(first, time.Now()); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Apply(tc.conflict, time.Now()); err == nil {
				t.Fatal("conflict accepted")
			}
			snapshot, _ := s.Snapshot("r")
			if snapshot.Integrity != "conflict" || snapshot.Revision != 1 {
				t.Fatalf("snapshot: %+v", snapshot)
			}
		})
	}
}

func TestSnapshotIsIndependent(t *testing.T) {
	s := NewStore("epoch", map[string][]string{"r": {"a"}})
	snapshot, _ := s.Snapshot("r")
	delete(snapshot.Machines, "a")
	again, _ := s.Snapshot("r")
	if _, ok := again.Machines["a"]; !ok {
		t.Fatal("snapshot modified store")
	}
}

func TestInvalidTransitionsFreezeWithoutChangingProjection(t *testing.T) {
	for _, tc := range []struct {
		name    string
		machine string
		order   string
	}{{"unknown station", "unknown", "wo"}, {"wrong first station", "b", "wo"}, {"unknown order", "a", "missing"}} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStore("epoch", map[string][]string{"r": {"a", "b"}})
			_, err := s.Apply(ev(1, "1", "workorder.started", nil, map[string]any{"workOrderId": "wo", "quantity": float64(1)}), time.Now())
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.Apply(ev(2, "2", "pcb.entered", &tc.machine, map[string]any{"workOrderId": tc.order, "boardId": "p"}), time.Now())
			if err == nil {
				t.Fatal("invalid transition accepted")
			}
			snap, _ := s.Snapshot("r")
			if snap.Integrity != "conflict" || snap.Revision != 1 || len(snap.Boards) != 0 {
				t.Fatalf("snapshot: %+v", snap)
			}
		})
	}
}

func TestInspectionFailureDoesNotScrapBoardAndMachineStatesRemainSeparate(t *testing.T) {
	s := NewStore("epoch", map[string][]string{"r": {"spi"}})
	m := "spi"
	events := []contract.Event{
		ev(1, "1", "workorder.started", nil, map[string]any{"workOrderId": "wo", "quantity": float64(1)}),
		ev(2, "2", "pcb.entered", &m, map[string]any{"workOrderId": "wo", "boardId": "p"}),
		ev(3, "3", "pcb.process.completed", &m, map[string]any{"workOrderId": "wo", "boardId": "p"}),
		ev(4, "4", "pcb.inspection.completed", &m, map[string]any{"workOrderId": "wo", "boardId": "p", "passed": false}),
		ev(5, "5", "machine.state.changed", &m, map[string]any{"observedState": "RUNNING", "flowState": "BLOCKED", "reason": "downstream full"}),
		ev(6, "6", "pcb.exited", &m, map[string]any{"workOrderId": "wo", "boardId": "p"}),
	}
	for _, e := range events {
		if _, err := s.Apply(e, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	snap, _ := s.Snapshot("r")
	b := snap.Boards["p"]
	if b.Status != "completed" || b.Inspections["spi"] {
		t.Fatalf("board: %+v", b)
	}
	machine := snap.Machines["spi"]
	if machine.ObservedState != "RUNNING" || machine.FlowState != "BLOCKED" || machine.Reason != "downstream full" {
		t.Fatalf("machine: %+v", machine)
	}
}

func TestLateFirstEventMarksGap(t *testing.T) {
	s := NewStore("epoch", map[string][]string{"r": {"a"}})
	if _, err := s.Apply(ev(2, "2", "workorder.started", nil, map[string]any{}), time.Now()); err == nil {
		t.Fatal("late event accepted")
	}
	snap, _ := s.Snapshot("r")
	if snap.Integrity != "gap" || snap.LastSequence != 0 {
		t.Fatalf("snapshot: %+v", snap)
	}
}

func TestEmptyRunIsNotClaimedComplete(t *testing.T) {
	s := NewStore("epoch", map[string][]string{"r": {"a"}})
	snap, _ := s.Snapshot("r")
	if snap.Integrity != "uncertain" {
		t.Fatalf("empty run = %s", snap.Integrity)
	}
	if _, err := s.Apply(ev(1, "1", "workorder.started", nil, map[string]any{"workOrderId": "wo", "quantity": float64(1)}), time.Now()); err != nil {
		t.Fatal(err)
	}
	snap, _ = s.Snapshot("r")
	if snap.Integrity != "complete" {
		t.Fatalf("first event = %s", snap.Integrity)
	}
}
