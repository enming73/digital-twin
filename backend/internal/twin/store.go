package twin

import (
	"crypto/sha256"
	"encoding/hex"
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
	RunID            string               `json:"runId"`
	Epoch            string               `json:"epoch"`
	Revision         int                  `json:"revision"`
	LastSequence     int                  `json:"lastSequence"`
	Integrity        string               `json:"integrity"`
	Reason           string               `json:"reason,omitempty"`
	TerminalVerified bool                 `json:"terminalVerified"`
	Machines         map[string]Machine   `json:"machines"`
	Boards           map[string]Board     `json:"boards"`
	WorkOrders       map[string]WorkOrder `json:"workOrders"`
	LastIngestedAt   time.Time            `json:"lastIngestedAt"`
	SimTimeMs        int64                `json:"simTimeMs"`
	EmittedAt        string               `json:"emittedAt"`
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
	recovering   bool
}

// ErrRecovering rejects live events while the durable source is catching up.
var ErrRecovering = errors.New("run is recovering from the durable source")

type Update struct {
	Type       string          `json:"type"`
	Epoch      string          `json:"epoch"`
	Revision   int             `json:"revision,omitempty"`
	Event      *contract.Event `json:"event,omitempty"`
	Integrity  string          `json:"integrity"`
	Reason     string          `json:"reason,omitempty"`
	IngestedAt *time.Time      `json:"ingestedAt,omitempty"`
	Snapshot   *Snapshot       `json:"snapshot,omitempty"`
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
	mu              sync.Mutex
	runs            map[string]*run
	journal         Journal
	metrics         Metrics
	requireTerminal bool
}

// SetRequireTerminal makes the store refuse to advertise completeness until a
// verified source terminal has been promoted. W3-only deployments keep the
// legacy contiguous-from-one behavior.
func (s *Store) SetRequireTerminal() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requireTerminal = true
}

type Metrics struct {
	Accepted      uint64         `json:"accepted"`
	Duplicates    uint64         `json:"duplicates"`
	Conflicts     uint64         `json:"conflicts"`
	Gaps          uint64         `json:"gaps"`
	StorageErrors uint64         `json:"storageErrors"`
	LastSequence  map[string]int `json:"lastSequence"`
}

func (s *Store) Metrics() Metrics {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := s.metrics
	result.LastSequence = make(map[string]int, len(s.runs))
	for id, run := range s.runs {
		result.LastSequence[id] = run.snapshot.LastSequence
	}
	return result
}

// Journal commits a validated candidate before it becomes visible to readers.
// Implementations must not call back into Store while Commit is running.
type Journal interface {
	Commit(event contract.Event, ingestedAt time.Time, candidate Snapshot, trace []TraceEntry, checkpoint *Checkpoint) error
	SetIntegrity(runID, integrity, reason string) error
}

type recoveredJournal interface {
	CommitRecovered(event contract.Event, ingestedAt time.Time, candidate Snapshot, trace []TraceEntry, checkpoint *Checkpoint) error
}

func (s *Store) reject(r *run, integrity, reason string) error {
	if s.journal != nil {
		if err := s.journal.SetIntegrity(r.snapshot.RunID, integrity, reason); err != nil {
			s.metrics.StorageErrors++
			return fmt.Errorf("persist integrity: %w", err)
		}
	}
	if integrity == "gap" {
		s.metrics.Gaps++
	} else if integrity == "conflict" {
		s.metrics.Conflicts++
	}
	r.snapshot.Integrity = integrity
	r.snapshot.Reason = reason
	r.status()
	return errors.New(reason)
}

// MarkUncertain records a period in which non-retained events may be lost.
func (s *Store) MarkUncertain(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.runs {
		r.canEstablish = false
		if r.snapshot.Integrity == "complete" || (r.snapshot.Integrity == "uncertain" && r.snapshot.LastSequence == 0) {
			if s.journal != nil {
				if err := s.journal.SetIntegrity(r.snapshot.RunID, "uncertain", reason); err != nil {
					r.snapshot.Integrity = "uncertain"
					r.snapshot.Reason = "cannot persist uncertainty: " + err.Error()
					r.status()
					continue
				}
			}
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

// NewStoreWithJournal is intended for a storage-backed writer; a nil journal
// has exactly the same in-memory behavior as NewStore.
func NewStoreWithJournal(epoch string, runs map[string][]string, journal Journal) *Store {
	s := NewStore(epoch, runs)
	s.journal = journal
	return s
}

// FinishRestore installs the journal only after the caller has reconstructed
// a private store. A process restart cannot prove source coverage is complete.
func (s *Store) FinishRestore(runID, integrity, reason string, journal Journal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.journal != nil || journal == nil {
		return errors.New("invalid restore journal")
	}
	r, ok := s.runs[runID]
	if !ok {
		return errors.New("unknown run")
	}
	if integrity != "gap" && integrity != "conflict" {
		integrity = "uncertain"
		reason = "source coverage after restart is unverified"
	}
	r.snapshot.Integrity = integrity
	r.snapshot.Reason = reason
	r.canEstablish = false
	s.journal = journal
	return nil
}
func clone(s Snapshot) Snapshot {
	b, _ := json.Marshal(s)
	var out Snapshot
	_ = json.Unmarshal(b, &out)
	return out
}

// Knows reports whether a run is registered with this store.
func (s *Store) Knows(runID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.runs[runID]
	return ok
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

// SequenceFingerprint returns the stored envelope fingerprint for a committed
// sequence so recovery can compare source bytes with the backend prefix.
func (s *Store) SequenceFingerprint(runID string, sequence int) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runID]
	if !ok {
		return "", false
	}
	id, ok := r.sequences[sequence]
	if !ok {
		return "", false
	}
	return r.ids[id], true
}

// BeginRecovery suspends live application for a run so the coordinator can
// repair the committed prefix from the durable source. A conflict is never
// thawed.
func (s *Store) BeginRecovery(runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runID]
	if !ok {
		return errors.New("unknown run")
	}
	if r.snapshot.Integrity == "conflict" {
		return errors.New("run is frozen in conflict")
	}
	r.recovering = true
	return nil
}

// EndRecovery re-enables live application; a failed recovery leaves any gap
// recorded so the run stays uncertain.
func (s *Store) EndRecovery(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.runs[runID]; ok {
		r.recovering = false
	}
}

// PromoteComplete records a verified contiguous run as complete. It refuses to
// overwrite a conflict and requires the caller to have verified the source.
func (s *Store) PromoteComplete(runID string, finalSequence int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runID]
	if !ok {
		return errors.New("unknown run")
	}
	if r.snapshot.Integrity == "conflict" {
		return errors.New("run is frozen in conflict")
	}
	if r.snapshot.LastSequence != finalSequence {
		return fmt.Errorf("verified source ends at %d, committed head is %d", finalSequence, r.snapshot.LastSequence)
	}
	if s.journal != nil {
		if err := s.journal.SetIntegrity(runID, "complete", ""); err != nil {
			s.metrics.StorageErrors++
			return fmt.Errorf("persist completion: %w", err)
		}
	}
	r.snapshot.Integrity = "complete"
	r.snapshot.Reason = ""
	r.snapshot.TerminalVerified = true
	r.snapshot.Epoch = fmt.Sprintf("%s-r%d", r.snapshot.Epoch, r.snapshot.Revision)
	snapshot := clone(r.snapshot)
	r.publish(Update{Type: "snapshot", Epoch: snapshot.Epoch, Revision: snapshot.Revision, Integrity: snapshot.Integrity, Snapshot: &snapshot})
	r.status()
	return nil
}

// RecoveryFailed keeps an unverifiable run uncertain and visible without
// creating the sticky conflict state that is reserved for data contradictions.
func (s *Store) RecoveryFailed(reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.runs {
		if r.snapshot.Integrity == "conflict" {
			continue
		}
		if s.journal != nil {
			if err := s.journal.SetIntegrity(r.snapshot.RunID, "uncertain", reason); err != nil {
				s.metrics.StorageErrors++
			}
		}
		r.snapshot.Integrity = "uncertain"
		r.snapshot.Reason = reason
		r.snapshot.TerminalVerified = false
		r.status()
	}
}

func (s *Store) Apply(e contract.Event, at time.Time) (ApplyResult, error) {
	return s.apply(e, at, false)
}

// ApplyRecovered applies an event fetched from the durable source while the
// run is suspended for recovery.
func (s *Store) ApplyRecovered(e contract.Event, at time.Time) (ApplyResult, error) {
	return s.apply(e, at, true)
}

func (s *Store) apply(e contract.Event, at time.Time, fromRecovery bool) (ApplyResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.journal != nil {
		at = at.UTC().Truncate(time.Microsecond)
	}
	r, ok := s.runs[e.RunID]
	if !ok {
		return ApplyResult{}, errors.New("unknown run")
	}
	b, err := json.Marshal(e)
	if err != nil {
		return ApplyResult{}, err
	}
	sum := sha256.Sum256(b)
	fingerprint := hex.EncodeToString(sum[:])
	var immutable contract.Event
	if err := json.Unmarshal(b, &immutable); err != nil {
		return ApplyResult{}, err
	}
	if old, ok := r.ids[e.EventID]; ok {
		if old == fingerprint {
			s.metrics.Duplicates++
			return ApplyResult{Duplicate: true}, nil
		}
		return ApplyResult{}, s.reject(r, "conflict", "event ID conflict")
	}
	if r.recovering && !fromRecovery {
		return ApplyResult{}, ErrRecovering
	}
	if _, ok := r.sequences[e.Sequence]; ok {
		return ApplyResult{}, s.reject(r, "conflict", "sequence conflict")
	}
	if r.snapshot.Integrity == "conflict" || (r.snapshot.Integrity == "gap" && !fromRecovery) {
		return ApplyResult{}, errors.New("run frozen")
	}
	if e.Sequence != r.snapshot.LastSequence+1 {
		return ApplyResult{}, s.reject(r, "gap", fmt.Sprintf("expected sequence %d, got %d", r.snapshot.LastSequence+1, e.Sequence))
	}
	if e.SimTimeMs < r.snapshot.SimTimeMs {
		return ApplyResult{}, s.reject(r, "conflict", "simulated time regression")
	}
	next := clone(r.snapshot)
	if err := reduce(&next, r.stations, e); err != nil {
		return ApplyResult{}, s.reject(r, "conflict", err.Error())
	}
	canEstablish := r.canEstablish
	if canEstablish && next.LastSequence == 0 {
		canEstablish = false
		if s.requireTerminal {
			next.Integrity = "uncertain"
			next.Reason = "awaiting verified source terminal"
		} else {
			next.Integrity = "complete"
			next.Reason = ""
		}
	}
	next.Revision++
	next.LastSequence = e.Sequence
	next.SimTimeMs = e.SimTimeMs
	next.EmittedAt = e.EmittedAt
	next.LastIngestedAt = at.UTC()
	var trace *TraceEntry
	if _, ok := e.Payload["boardId"].(string); ok {
		entry := TraceEntry{Sequence: e.Sequence, EventID: e.EventID, EventType: e.EventType, MachineID: immutable.MachineID, SimTimeMs: e.SimTimeMs, EmittedAt: e.EmittedAt, IngestedAt: at.UTC()}
		if passed, ok := e.Payload["passed"].(bool); ok {
			entry.Passed = &passed
		}
		trace = &entry
	}
	if s.journal != nil {
		var entries []TraceEntry
		if trace != nil {
			id := e.Payload["boardId"].(string)
			entries = append(append([]TraceEntry(nil), r.traces[id]...), *trace)
		}
		var checkpoint *Checkpoint
		if next.LastSequence%500 == 0 {
			candidate, err := copyCheckpoint(Checkpoint{Snapshot: next, Stations: r.stations, Traces: r.traces, IDs: r.ids, Sequences: r.sequences})
			if err != nil {
				return ApplyResult{}, err
			}
			candidate.IDs[e.EventID] = fingerprint
			candidate.Sequences[e.Sequence] = e.EventID
			if trace != nil {
				candidate.Traces[e.Payload["boardId"].(string)] = entries
			}
			checkpoint = &candidate
		}
		commit := s.journal.Commit
		if fromRecovery {
			if recovered, ok := s.journal.(recoveredJournal); ok {
				commit = recovered.CommitRecovered
			}
		}
		if err := commit(immutable, at.UTC(), clone(next), entries, checkpoint); err != nil {
			s.metrics.StorageErrors++
			return ApplyResult{}, fmt.Errorf("commit event: %w", err)
		}
	}
	r.canEstablish = canEstablish
	s.metrics.Accepted++
	r.snapshot = next
	if trace != nil {
		id := e.Payload["boardId"].(string)
		r.traces[id] = append(r.traces[id], *trace)
	}
	r.ids[e.EventID] = fingerprint
	r.sequences[e.Sequence] = e.EventID
	ingestedAt := at.UTC()
	if !fromRecovery {
		r.publish(Update{Type: "delta", Epoch: next.Epoch, Revision: next.Revision, Event: &immutable, Integrity: next.Integrity, IngestedAt: &ingestedAt})
	}
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
