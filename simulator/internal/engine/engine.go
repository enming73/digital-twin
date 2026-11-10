// Package engine implements the headless, virtual-time production model.
package engine

import (
	"fmt"
	"math"
	"strings"

	"smt-digital-twin/simulator/internal/contract"
	"smt-digital-twin/simulator/internal/scenario"
)

type Station struct {
	scenario.Station
	Queue                            []string
	BoardID                          string
	FinishAtMs                       int64
	Holding                          bool
	Faulted                          bool
	RemainingMs                      int64
	ObservedState, FlowState, Reason string
}
type Snapshot struct {
	SimTimeMs          int64
	Created, Completed int
	Stations           []Station
}
type Engine struct {
	config     scenario.Scenario
	runID      string
	seed       uint32
	sequence   int
	snapshot   Snapshot
	started    bool
	random     uint32
	faultIndex int
	failed     map[string]bool
}

func New(s scenario.Scenario, seed uint32, runID string) (*Engine, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if runID == "" || strings.ContainsAny(runID, "/+#") {
		return nil, fmt.Errorf("invalid run ID")
	}
	e := &Engine{config: s, runID: runID, seed: seed, random: seed, failed: map[string]bool{}}
	for _, st := range s.Stations {
		e.snapshot.Stations = append(e.snapshot.Stations, Station{Station: st})
	}
	return e, nil
}
func (e *Engine) Snapshot() Snapshot {
	s := e.snapshot
	s.Stations = append([]Station(nil), s.Stations...)
	for i := range s.Stations {
		s.Stations[i].Queue = append([]string(nil), s.Stations[i].Queue...)
	}
	return s
}

// RunID reports the run identity this engine emits for.
func (e *Engine) RunID() string { return e.runID }
func (e *Engine) emit(out *[]contract.Event, kind string, machine *string, payload map[string]any) {
	e.sequence++
	payload["synthetic"] = true
	*out = append(*out, contract.Event{SchemaVersion: 1, EventID: fmt.Sprintf("event-%08d", e.sequence), RunID: e.runID, Source: "simulator", MachineID: machine, EventType: kind, Sequence: e.sequence, SimTimeMs: e.snapshot.SimTimeMs, Payload: payload})
}
func boardPayload(id string) map[string]any {
	return map[string]any{"boardId": id, "workOrderId": "work-order-1"}
}

// NextTime reports the next boundary without advancing business state.
func (e *Engine) NextTime() (int64, error) {
	if !e.started || e.snapshot.Completed == e.config.Quantity {
		return e.snapshot.SimTimeMs, nil
	}
	next := int64(math.MaxInt64)
	for _, st := range e.snapshot.Stations {
		if st.BoardID != "" && !st.Holding && !st.Faulted && st.FinishAtMs < next {
			next = st.FinishAtMs
		}
	}
	if e.faultIndex < len(e.config.Faults) && e.config.Faults[e.faultIndex].SimTimeMs < next {
		next = e.config.Faults[e.faultIndex].SimTimeMs
	}
	if next == math.MaxInt64 {
		return 0, fmt.Errorf("simulation deadlock")
	}
	return next, nil
}
func (e *Engine) Next() ([]contract.Event, bool, error) {
	var out []contract.Event
	if e.snapshot.Completed == e.config.Quantity {
		return nil, true, nil
	}
	if !e.started {
		e.started = true
		e.emit(&out, "workorder.started", nil, map[string]any{"workOrderId": "work-order-1", "quantity": e.config.Quantity})
	} else {
		next, err := e.NextTime()
		if err != nil {
			return nil, false, err
		}
		e.snapshot.SimTimeMs = next
		for i := range e.snapshot.Stations {
			st := &e.snapshot.Stations[i]
			if st.BoardID != "" && !st.Holding && !st.Faulted && st.FinishAtMs == next {
				st.Holding = true
				id := st.ID
				e.emit(&out, "pcb.process.completed", &id, boardPayload(st.BoardID))
				if id == "spi" || id == "aoi" {
					e.random = e.random*1664525 + 1013904223
					passed := float64(e.random)/4294967296 >= e.config.DefectRate
					if !passed {
						e.failed[st.BoardID] = true
					}
					payload := boardPayload(st.BoardID)
					payload["passed"] = passed
					e.emit(&out, "pcb.inspection.completed", &id, payload)
				}
			}
		}
	}
	for e.faultIndex < len(e.config.Faults) && e.config.Faults[e.faultIndex].SimTimeMs == e.snapshot.SimTimeMs {
		command := e.config.Faults[e.faultIndex]
		e.faultIndex++
		for i := range e.snapshot.Stations {
			st := &e.snapshot.Stations[i]
			if st.ID != command.MachineID {
				continue
			}
			id := st.ID
			kind := "machine.alarm.raised"
			if command.Action == "raise" {
				st.Faulted = true
				if st.BoardID != "" && !st.Holding {
					st.RemainingMs = st.FinishAtMs - e.snapshot.SimTimeMs
				}
			} else {
				kind = "machine.alarm.cleared"
				st.Faulted = false
				if st.BoardID != "" && !st.Holding {
					st.FinishAtMs = e.snapshot.SimTimeMs + st.RemainingMs
				}
				st.RemainingMs = 0
			}
			e.emit(&out, kind, &id, map[string]any{"alarmId": "fault-" + id, "reason": command.Reason})
		}
	}
	for i := len(e.snapshot.Stations) - 1; i >= 0; i-- {
		st := &e.snapshot.Stations[i]
		if st.Faulted {
			continue
		}
		if st.BoardID != "" && st.Holding {
			if i == len(e.snapshot.Stations)-1 || len(e.snapshot.Stations[i+1].Queue) < e.snapshot.Stations[i+1].InputCapacity {
				board := st.BoardID
				id := st.ID
				e.emit(&out, "pcb.exited", &id, boardPayload(board))
				st.BoardID = ""
				st.Holding = false
				if i == len(e.snapshot.Stations)-1 {
					e.snapshot.Completed++
				} else {
					next := &e.snapshot.Stations[i+1]
					next.Queue = append(next.Queue, board)
					id := next.ID
					e.emit(&out, "pcb.entered", &id, boardPayload(board))
				}
			}
		}
		if st.BoardID == "" && len(st.Queue) > 0 {
			st.BoardID = st.Queue[0]
			st.Queue = st.Queue[1:]
			st.FinishAtMs = e.snapshot.SimTimeMs + st.ProcessTimeMs
		}
	}
	first := &e.snapshot.Stations[0]
	for e.snapshot.Created < e.config.Quantity && len(first.Queue) < first.InputCapacity {
		e.snapshot.Created++
		board := fmt.Sprintf("board-%d", e.snapshot.Created)
		first.Queue = append(first.Queue, board)
		id := first.ID
		e.emit(&out, "pcb.entered", &id, boardPayload(board))
	}
	if !first.Faulted && first.BoardID == "" && len(first.Queue) > 0 {
		first.BoardID = first.Queue[0]
		first.Queue = first.Queue[1:]
		first.FinishAtMs = e.snapshot.SimTimeMs + first.ProcessTimeMs
	}
	for i := range e.snapshot.Stations {
		st := &e.snapshot.Stations[i]
		if !st.Faulted && st.BoardID == "" && len(st.Queue) > 0 {
			st.BoardID = st.Queue[0]
			st.Queue = st.Queue[1:]
			st.FinishAtMs = e.snapshot.SimTimeMs + st.ProcessTimeMs
		}
	}
	for i := range e.snapshot.Stations {
		st := &e.snapshot.Stations[i]
		observed, flow, reason := "IDLE", "NORMAL", "idle"
		if st.Faulted {
			observed = "FAULT"
			reason = "fault"
		} else if st.BoardID != "" {
			observed = "RUNNING"
			reason = "processing"
			if st.Holding && i+1 < len(e.snapshot.Stations) && len(e.snapshot.Stations[i+1].Queue) >= e.snapshot.Stations[i+1].InputCapacity {
				flow = "BLOCKED"
				reason = "downstream " + e.snapshot.Stations[i+1].ID + " full"
			}
		} else if e.snapshot.Completed < e.config.Quantity {
			flow = "STARVED"
			reason = "input empty"
		}
		if observed != st.ObservedState || flow != st.FlowState || reason != st.Reason {
			st.ObservedState = observed
			st.FlowState = flow
			st.Reason = reason
			id := st.ID
			e.emit(&out, "machine.state.changed", &id, map[string]any{"observedState": observed, "flowState": flow, "reason": reason})
		}
	}
	if e.snapshot.Completed == e.config.Quantity {
		e.emit(&out, "workorder.completed", nil, map[string]any{"workOrderId": "work-order-1", "quantity": e.config.Quantity, "passed": e.config.Quantity - len(e.failed), "failed": len(e.failed)})
		return out, true, nil
	}
	return out, false, nil
}
