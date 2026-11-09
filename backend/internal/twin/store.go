package twin

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"smt-digital-twin/backend/internal/contract"
)

type Board struct {
	ID          string          `json:"id"`
	WorkOrderID string          `json:"workOrderId"`
	MachineID   string          `json:"machineId,omitempty"`
	Status      string          `json:"status"`
	Inspections map[string]bool `json:"inspections"`
	Processed   bool            `json:"processed"`
}
type Machine struct {
	ObservedState string            `json:"observedState"`
	FlowState     string            `json:"flowState"`
	Reason        string            `json:"reason"`
	Alarms        map[string]string `json:"alarms"`
}
type WorkOrder struct {
	ID       string  `json:"id"`
	Quantity float64 `json:"quantity"`
	Passed   float64 `json:"passed"`
	Failed   float64 `json:"failed"`
	Status   string  `json:"status"`
}
type Snapshot struct {
	RunID          string               `json:"runId"`
	Epoch          string               `json:"epoch"`
	Revision       int                  `json:"revision"`
	LastSequence   int                  `json:"lastSequence"`
	Integrity      string               `json:"integrity"`
	Reason         string               `json:"reason,omitempty"`
	Machines       map[string]Machine   `json:"machines"`
	Boards         map[string]Board     `json:"boards"`
	WorkOrders     map[string]WorkOrder `json:"workOrders"`
	LastIngestedAt time.Time            `json:"lastIngestedAt"`
	SimTimeMs      int64                `json:"simTimeMs"`
	EmittedAt      string               `json:"emittedAt"`
}
type ApplyResult struct{ Duplicate bool }
type run struct {
	snapshot     Snapshot
	stations     []string
	traces       map[string][]TraceEntry
	ids          map[string]string
	sequences    map[int]string
	subscribers  map[chan Update]struct{}
	canEstablish bool
}
type Update struct {
	Type       string          `json:"type"`
	Epoch      string          `json:"epoch"`
	Revision   int             `json:"revision,omitempty"`
	Event      *contract.Event `json:"event,omitempty"`
	Integrity  string          `json:"integrity"`
	Reason     string          `json:"reason,omitempty"`
	IngestedAt *time.Time      `json:"ingestedAt,omitempty"`
}

func (r *run) publish(u Update) {
	for ch := range r.subscribers {
		select {
		case ch <- u:
		default:
			close(ch)
			delete(r.subscribers, ch)
		}
	}
}
func (s *Store) Subscribe(id string, capacity int) (Snapshot, <-chan Update, func(), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return Snapshot{}, nil, nil, false
	}
	if capacity < 1 {
		capacity = 1
	}
	ch := make(chan Update, capacity)
	r.subscribers[ch] = struct{}{}
	cancel := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, ok := r.subscribers[ch]; ok {
			delete(r.subscribers, ch)
			close(ch)
		}
	}
	return clone(r.snapshot), ch, cancel, true
}
func (r *run) status() {
	r.publish(Update{Type: "status", Epoch: r.snapshot.Epoch, Integrity: r.snapshot.Integrity, Reason: r.snapshot.Reason})
}

type Store struct {
	mu   sync.Mutex
	runs map[string]*run
}

// MarkUncertain records a period in which non-retained events may be lost.
func (s *Store) MarkUncertain(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.runs {
		r.canEstablish = false
		if r.snapshot.Integrity == "complete" || (r.snapshot.Integrity == "uncertain" && r.snapshot.LastSequence == 0) {
			r.snapshot.Integrity = "uncertain"
			r.snapshot.Reason = reason
			r.status()
		}
	}
}

func NewStore(epoch string, runs map[string][]string) *Store {
	s := &Store{runs: map[string]*run{}}
	for id, stations := range runs {
		r := &run{stations: append([]string(nil), stations...), traces: map[string][]TraceEntry{}, ids: map[string]string{}, sequences: map[int]string{}, subscribers: map[chan Update]struct{}{}, canEstablish: true, snapshot: Snapshot{RunID: id, Epoch: epoch, Integrity: "uncertain", Reason: "awaiting first event", Machines: map[string]Machine{}, Boards: map[string]Board{}, WorkOrders: map[string]WorkOrder{}}}
		for _, st := range stations {
			r.snapshot.Machines[st] = Machine{Alarms: map[string]string{}}
		}
		s.runs[id] = r
	}
	return s
}
func clone(s Snapshot) Snapshot {
	b, _ := json.Marshal(s)
	var out Snapshot
	_ = json.Unmarshal(b, &out)
	return out
}
func (s *Store) Snapshot(id string) (Snapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return Snapshot{}, false
	}
	return clone(r.snapshot), true
}

func (s *Store) Apply(e contract.Event, at time.Time) (ApplyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[e.RunID]
	if !ok {
		return ApplyResult{}, errors.New("unknown run")
	}
	b, err := json.Marshal(e)
	if err != nil {
		return ApplyResult{}, err
	}
	fingerprint := string(b)
	var immutable contract.Event
	if err := json.Unmarshal(b, &immutable); err != nil {
		return ApplyResult{}, err
	}
	if old, ok := r.ids[e.EventID]; ok {
		if old == fingerprint {
			return ApplyResult{Duplicate: true}, nil
		}
		r.snapshot.Integrity = "conflict"
		r.snapshot.Reason = "event ID conflict"
		r.status()
		return ApplyResult{}, errors.New(r.snapshot.Reason)
	}
	if _, ok := r.sequences[e.Sequence]; ok {
		r.snapshot.Integrity = "conflict"
		r.snapshot.Reason = "sequence conflict"
		r.status()
		return ApplyResult{}, errors.New(r.snapshot.Reason)
	}
	if r.snapshot.Integrity == "gap" || r.snapshot.Integrity == "conflict" {
		return ApplyResult{}, errors.New("run frozen")
	}
	if e.Sequence != r.snapshot.LastSequence+1 {
		r.snapshot.Integrity = "gap"
		r.snapshot.Reason = fmt.Sprintf("expected sequence %d, got %d", r.snapshot.LastSequence+1, e.Sequence)
		r.status()
		return ApplyResult{}, errors.New(r.snapshot.Reason)
	}
	next := clone(r.snapshot)
	if err := reduce(&next, r.stations, e); err != nil {
		r.snapshot.Integrity = "conflict"
		r.snapshot.Reason = err.Error()
		r.status()
		return ApplyResult{}, err
	}
	if r.canEstablish && next.LastSequence == 0 {
		next.Integrity = "complete"
		next.Reason = ""
		r.canEstablish = false
	}
	next.Revision++
	next.LastSequence = e.Sequence
	next.SimTimeMs = e.SimTimeMs
	next.EmittedAt = e.EmittedAt
	next.LastIngestedAt = at.UTC()
	r.snapshot = next
	if id, ok := e.Payload["boardId"].(string); ok {
		entry := TraceEntry{Sequence: e.Sequence, EventID: e.EventID, EventType: e.EventType, MachineID: immutable.MachineID, SimTimeMs: e.SimTimeMs, EmittedAt: e.EmittedAt, IngestedAt: at.UTC()}
		if passed, ok := e.Payload["passed"].(bool); ok {
			entry.Passed = &passed
		}
		r.traces[id] = append(r.traces[id], entry)
	}
	r.ids[e.EventID] = fingerprint
	r.sequences[e.Sequence] = e.EventID
	ingestedAt := at.UTC()
	r.publish(Update{Type: "delta", Epoch: next.Epoch, Revision: next.Revision, Event: &immutable, Integrity: next.Integrity, IngestedAt: &ingestedAt})
	return ApplyResult{}, nil
}

func reduce(s *Snapshot, stations []string, e contract.Event) error {
	p := e.Payload
	str := func(k string) string { v, _ := p[k].(string); return v }
	machine := ""
	if e.MachineID != nil {
		machine = *e.MachineID
		if _, ok := s.Machines[machine]; !ok {
			return errors.New("unknown machine")
		}
	}
	switch e.EventType {
	case "workorder.started":
		id := str("workOrderId")
		if _, ok := s.WorkOrders[id]; ok {
			return errors.New("duplicate work order")
		}
		q, _ := p["quantity"].(float64)
		s.WorkOrders[id] = WorkOrder{ID: id, Quantity: q, Status: "running"}
	case "workorder.completed":
		id := str("workOrderId")
		w, ok := s.WorkOrders[id]
		if !ok || w.Status != "running" {
			return errors.New("unknown work order")
		}
		q, _ := p["quantity"].(float64)
		passed, _ := p["passed"].(float64)
		failed, _ := p["failed"].(float64)
		count := 0
		for _, b := range s.Boards {
			if b.WorkOrderID == id && b.Status == "completed" {
				count++
			}
		}
		if w.Quantity != q || float64(count) != q || passed+failed != q {
			return errors.New("work order completion mismatch")
		}
		w.Status = "completed"
		w.Passed = passed
		w.Failed = failed
		s.WorkOrders[id] = w
	case "machine.state.changed":
		m := s.Machines[machine]
		m.ObservedState = str("observedState")
		m.FlowState = str("flowState")
		m.Reason = str("reason")
		s.Machines[machine] = m
	case "machine.alarm.raised", "machine.alarm.cleared":
		m := s.Machines[machine]
		id := str("alarmId")
		if e.EventType == "machine.alarm.raised" {
			m.Alarms[id] = str("reason")
		} else {
			delete(m.Alarms, id)
		}
		s.Machines[machine] = m
	case "pcb.entered", "pcb.exited", "pcb.process.completed", "pcb.inspection.completed":
		id, wo := str("boardId"), str("workOrderId")
		w, ok := s.WorkOrders[wo]
		if !ok || w.Status != "running" {
			return errors.New("unknown or completed work order")
		}
		b, exists := s.Boards[id]
		if e.EventType == "pcb.entered" {
			if !exists {
				if len(stations) == 0 || machine != stations[0] {
					return errors.New("board must enter first station")
				}
				b = Board{ID: id, WorkOrderID: wo, Inspections: map[string]bool{}}
			} else {
				if b.WorkOrderID != wo || b.Status != "in_transit" {
					return errors.New("invalid board entry")
				}
				prev := -1
				for i, st := range stations {
					if st == b.MachineID {
						prev = i
					}
				}
				if prev < 0 || prev+1 >= len(stations) || stations[prev+1] != machine {
					return errors.New("invalid station transition")
				}
			}
			b.MachineID = machine
			b.Status = "at_station"
			b.Processed = false
		} else {
			if !exists || b.WorkOrderID != wo || b.Status != "at_station" || b.MachineID != machine {
				return errors.New("invalid board event")
			}
			switch e.EventType {
			case "pcb.process.completed":
				if b.Processed {
					return errors.New("duplicate completion")
				}
				b.Processed = true
			case "pcb.inspection.completed":
				if !b.Processed {
					return errors.New("inspection before processing")
				}
				if _, exists := b.Inspections[machine]; exists {
					return errors.New("repeated inspection unsupported")
				}
				passed, _ := p["passed"].(bool)
				b.Inspections[machine] = passed
			case "pcb.exited":
				if !b.Processed {
					return errors.New("exit before processing")
				}
				b.Status = "in_transit"
				if len(stations) > 0 && machine == stations[len(stations)-1] {
					b.Status = "completed"
				}
			}
		}
		s.Boards[id] = b
	default:
		return errors.New("unknown event type")
	}
	return nil
}
