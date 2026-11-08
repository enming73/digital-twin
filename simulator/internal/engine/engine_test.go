package engine

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"smt-digital-twin/simulator/internal/contract"

	"smt-digital-twin/simulator/internal/scenario"
)

func TestHundredBoardsCompleteWithoutLoss(t *testing.T) {
	s := scenario.Scenario{Quantity: 100, Stations: []scenario.Station{{ID: "loader", ProcessTimeMs: 10, InputCapacity: 2}, {ID: "unloader", ProcessTimeMs: 20, InputCapacity: 2}}}
	e, err := New(s, 2026, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		_, done, err := e.Next()
		if err != nil {
			t.Fatal(err)
		}
		snap := e.Snapshot()
		for _, station := range snap.Stations {
			if len(station.Queue) > station.InputCapacity {
				t.Fatal("buffer overflow")
			}
		}
		if done {
			if snap.Completed != 100 {
				t.Fatalf("completed %d", snap.Completed)
			}
			return
		}
	}
	t.Fatal("simulation failed to terminate")
}

func TestGeneratedEventsMatchContract(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	schema, err := compiler.Compile("../../../contracts/event.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	s := scenario.Scenario{Quantity: 3, DefectRate: .5, Stations: []scenario.Station{{ID: "loader", ProcessTimeMs: 10, InputCapacity: 1}, {ID: "spi", ProcessTimeMs: 20, InputCapacity: 1}, {ID: "unloader", ProcessTimeMs: 10, InputCapacity: 1}}, Faults: []scenario.Fault{{SimTimeMs: 25, MachineID: "spi", Action: "raise", Reason: "fault"}, {SimTimeMs: 100, MachineID: "spi", Action: "clear", Reason: "recovered"}}}
	e, err := New(s, 2026, "test")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		events, done, err := e.Next()
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			event.EmittedAt = "2026-11-08T12:00:00Z"
			if err := contract.Validate(event); err != nil {
				t.Fatalf("%s: %v", event.EventType, err)
			}
			data, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			var obj any
			if err := json.Unmarshal(data, &obj); err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(obj); err != nil {
				t.Fatalf("%s: %v", data, err)
			}
		}
		if done {
			return
		}
	}
	t.Fatal("did not finish")
}

func TestBoardLifecycleAndDeterminism(t *testing.T) {
	s := scenario.Scenario{Quantity: 100, DefectRate: .03, Stations: []scenario.Station{{ID: "loader", ProcessTimeMs: 3, InputCapacity: 2}, {ID: "spi", ProcessTimeMs: 5, InputCapacity: 2}, {ID: "unloader", ProcessTimeMs: 4, InputCapacity: 2}}}
	run := func() []string {
		e, err := New(s, 2026, "same-run")
		if err != nil {
			t.Fatal(err)
		}
		byBoard := map[string][]string{}
		var trace []string
		seq := 0
		var last int64
		for i := 0; i < 1000; i++ {
			batch, done, err := e.Next()
			if err != nil {
				t.Fatal(err)
			}
			snap := e.Snapshot()
			seen := map[string]bool{}
			for _, st := range snap.Stations {
				ids := append([]string(nil), st.Queue...)
				if st.BoardID != "" {
					ids = append(ids, st.BoardID)
				}
				for _, id := range ids {
					if seen[id] {
						t.Fatalf("duplicate ownership: %s", id)
					}
					seen[id] = true
				}
				if len(st.Queue) > st.InputCapacity {
					t.Fatal("overflow")
				}
			}
			if len(seen)+snap.Completed != snap.Created {
				t.Fatal("lost board")
			}
			for _, event := range batch {
				seq++
				if event.Sequence != seq || event.SimTimeMs < last {
					t.Fatal("invalid event ordering")
				}
				last = event.SimTimeMs
				trace = append(trace, fmt.Sprintf("%d:%s:%s:%v", event.SimTimeMs, event.EventType, event.EventID, event.Payload))
				if event.EventType == "pcb.entered" || event.EventType == "pcb.exited" || event.EventType == "pcb.process.completed" {
					id := event.Payload["boardId"].(string)
					byBoard[id] = append(byBoard[id], *event.MachineID+":"+event.EventType)
				}
			}
			if done {
				if len(byBoard) != 100 {
					t.Fatalf("boards %d", len(byBoard))
				}
				for id, events := range byBoard {
					if len(events) != 9 {
						t.Fatalf("%s events=%v", id, events)
					}
					for stationIndex, st := range s.Stations {
						for eventIndex, kind := range []string{"pcb.entered", "pcb.process.completed", "pcb.exited"} {
							if events[stationIndex*3+eventIndex] != st.ID+":"+kind {
								t.Fatalf("%s lifecycle %v", id, events)
							}
						}
					}
				}
				return trace
			}
		}
		t.Fatal("did not terminate")
		return nil
	}
	if a, b := run(), run(); !reflect.DeepEqual(a, b) {
		t.Fatal("events differ for same seed and scenario")
	}
}

func TestRejectInvalidRunID(t *testing.T) {
	s := scenario.Scenario{Quantity: 1, Stations: []scenario.Station{{ID: "loader", ProcessTimeMs: 1, InputCapacity: 1}}}
	if _, err := New(s, 1, "invalid/run"); err == nil {
		t.Fatal("accepted MQTT topic separator")
	}
}

func TestSettlingLeavesNoTransferableHeldBoard(t *testing.T) {
	s := scenario.Scenario{Quantity: 20, Stations: []scenario.Station{{ID: "one", ProcessTimeMs: 5, InputCapacity: 1}, {ID: "two", ProcessTimeMs: 8, InputCapacity: 1}, {ID: "three", ProcessTimeMs: 11, InputCapacity: 1}}}
	e, err := New(s, 1, "run")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 500; i++ {
		_, done, err := e.Next()
		if err != nil {
			t.Fatal(err)
		}
		snapshot := e.Snapshot()
		for j := 0; j < len(snapshot.Stations)-1; j++ {
			st, next := snapshot.Stations[j], snapshot.Stations[j+1]
			if st.Holding && !st.Faulted && len(next.Queue) < next.InputCapacity {
				t.Fatalf("transferable board at %d: %s", snapshot.SimTimeMs, st.BoardID)
			}
		}
		if done {
			return
		}
	}
	t.Fatal("did not complete")
}

func TestCompletionBeforeSimultaneousFaultAndRecovery(t *testing.T) {
	s := scenario.Scenario{Quantity: 1, Stations: []scenario.Station{{ID: "loader", ProcessTimeMs: 10, InputCapacity: 1}}, Faults: []scenario.Fault{{SimTimeMs: 10, MachineID: "loader", Action: "raise", Reason: "fault"}, {SimTimeMs: 20, MachineID: "loader", Action: "clear", Reason: "recovered"}}}
	e, err := New(s, 1, "run")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = e.Next()
	if err != nil {
		t.Fatal(err)
	}
	batch, done, err := e.Next()
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Fatal("fault should hold completed work")
	}
	if len(batch) < 2 || batch[0].EventType != "pcb.process.completed" || batch[1].EventType != "machine.alarm.raised" {
		t.Fatalf("wrong boundary order: %v", batch)
	}
	if !e.Snapshot().Stations[0].Holding {
		t.Fatal("completed board not held")
	}
	_, done, err = e.Next()
	if err != nil || !done {
		t.Fatalf("recovery done=%v err=%v", done, err)
	}
}
func TestPermanentFaultReportsDeadlock(t *testing.T) {
	s := scenario.Scenario{Quantity: 1, Stations: []scenario.Station{{ID: "loader", ProcessTimeMs: 10, InputCapacity: 1}}, Faults: []scenario.Fault{{SimTimeMs: 5, MachineID: "loader", Action: "raise", Reason: "fault"}}}
	e, err := New(s, 1, "run")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = e.Next(); err != nil {
		t.Fatal(err)
	}
	if _, _, err = e.Next(); err != nil {
		t.Fatal(err)
	}
	if _, _, err = e.Next(); err == nil {
		t.Fatal("expected deadlock")
	}
}

func TestSeededInspectionAndFaultRecovery(t *testing.T) {
	s := scenario.Scenario{Quantity: 8, DefectRate: 1, Stations: []scenario.Station{{ID: "loader", ProcessTimeMs: 10, InputCapacity: 1}, {ID: "spi", ProcessTimeMs: 20, InputCapacity: 1}, {ID: "unloader", ProcessTimeMs: 10, InputCapacity: 1}}, Faults: []scenario.Fault{{SimTimeMs: 25, MachineID: "spi", Action: "raise", Reason: "test"}, {SimTimeMs: 125, MachineID: "spi", Action: "clear", Reason: "test"}}}
	run := func() []string {
		e, err := New(s, 2026, "run-1")
		if err != nil {
			t.Fatal(err)
		}
		var events []string
		inspections, failed, alarms := 0, 0, 0
		for i := 0; i < 500; i++ {
			batch, done, err := e.Next()
			if err != nil {
				t.Fatal(err)
			}
			for _, event := range batch {
				events = append(events, event.EventType)
				if event.EventType == "pcb.inspection.completed" {
					inspections++
					if event.Payload["passed"] == false {
						failed++
					}
				}
				if event.EventType == "machine.alarm.raised" || event.EventType == "machine.alarm.cleared" {
					alarms++
				}
			}
			if done {
				if inspections != 8 || failed != 8 || alarms != 2 {
					t.Fatalf("inspections=%d failed=%d alarms=%d", inspections, failed, alarms)
				}
				return events
			}
		}
		t.Fatal("did not complete")
		return nil
	}
	if a, b := run(), run(); !reflect.DeepEqual(a, b) {
		t.Fatal("nondeterministic events")
	}
}

func TestFaultPropagatesOnlyAfterBuffersFillAndDrain(t *testing.T) {
	s := scenario.Scenario{Quantity: 20, Stations: []scenario.Station{{ID: "loader", ProcessTimeMs: 20, InputCapacity: 1}, {ID: "mounter", ProcessTimeMs: 10, InputCapacity: 1}, {ID: "unloader", ProcessTimeMs: 20, InputCapacity: 1}}, Faults: []scenario.Fault{{SimTimeMs: 85, MachineID: "mounter", Action: "raise", Reason: "fault"}, {SimTimeMs: 300, MachineID: "mounter", Action: "clear", Reason: "recovered"}}}
	e, err := New(s, 1, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	sawBlocked, sawStarved := false, false
	for i := 0; i < 500; i++ {
		_, done, err := e.Next()
		if err != nil {
			t.Fatal(err)
		}
		snap := e.Snapshot()
		for _, st := range snap.Stations {
			if st.ID == "loader" && st.FlowState == "BLOCKED" && snap.SimTimeMs >= 85 && snap.SimTimeMs < 300 {
				sawBlocked = true
				if len(snap.Stations[1].Queue) != 1 {
					t.Fatal("blocked without full downstream queue")
				}
			}
			if st.ID == "unloader" && st.FlowState == "STARVED" && snap.SimTimeMs > 85 && snap.SimTimeMs < 300 {
				sawStarved = true
				if st.BoardID != "" || len(st.Queue) != 0 {
					t.Fatal("starved with work")
				}
			}
		}
		if done {
			if !sawBlocked || !sawStarved {
				t.Fatalf("blocked=%v starved=%v", sawBlocked, sawStarved)
			}
			return
		}
	}
	t.Fatal("did not finish")
}
