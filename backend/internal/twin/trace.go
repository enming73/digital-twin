package twin

import (
	"encoding/json"
	"time"

	"smt-digital-twin/backend/internal/mes"
)

type TraceEntry struct {
	Sequence   int       `json:"sequence"`
	EventID    string    `json:"eventId"`
	EventType  string    `json:"eventType"`
	MachineID  *string   `json:"machineId"`
	SimTimeMs  int64     `json:"simTimeMs"`
	EmittedAt  string    `json:"emittedAt"`
	IngestedAt time.Time `json:"ingestedAt"`
	Passed     *bool     `json:"passed,omitempty"`
}

type Progress struct {
	Planned     int `json:"planned"`
	Created     int `json:"created"`
	InProgress  int `json:"inProgress"`
	Completed   int `json:"completed"`
	Passed      int `json:"passed"`
	Failed      int `json:"failed"`
	Uninspected int `json:"uninspected"`
}

type WorkOrderDetail struct {
	RunID          string        `json:"runId"`
	Epoch          string        `json:"epoch"`
	Revision       int           `json:"revision"`
	Integrity      string        `json:"integrity"`
	Reason         string        `json:"reason,omitempty"`
	Synthetic      bool          `json:"synthetic"`
	Order          WorkOrder     `json:"order"`
	Metadata       *mes.Metadata `json:"metadata"`
	Route          []string      `json:"route"`
	Progress       Progress      `json:"progress"`
	Reconciliation string        `json:"reconciliation"`
}

func (s *Store) WorkOrderDetail(runID, workOrderID string) (WorkOrderDetail, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runID]
	if !ok {
		return WorkOrderDetail{}, false
	}
	w, ok := r.snapshot.WorkOrders[workOrderID]
	if !ok {
		return WorkOrderDetail{}, false
	}
	d := WorkOrderDetail{RunID: runID, Epoch: r.snapshot.Epoch, Revision: r.snapshot.Revision, Integrity: r.snapshot.Integrity, Reason: r.snapshot.Reason, Synthetic: true, Order: w, Route: append([]string(nil), r.stations...), Progress: Progress{Planned: int(w.Quantity)}, Reconciliation: "pending"}
	if meta, ok := mes.LookupWorkOrder(workOrderID); ok {
		d.Metadata = &meta
	}
	for _, b := range r.snapshot.Boards {
		if b.WorkOrderID != workOrderID {
			continue
		}
		d.Progress.Created++
		if b.Status != "completed" {
			d.Progress.InProgress++
			continue
		}
		d.Progress.Completed++
		if len(b.Inspections) == 0 {
			d.Progress.Uninspected++
			continue
		}
		failed := false
		for _, passed := range b.Inspections {
			if !passed {
				failed = true
			}
		}
		if failed {
			d.Progress.Failed++
		} else {
			d.Progress.Passed++
		}
	}
	if w.Status == "completed" && d.Progress.Uninspected == 0 {
		d.Reconciliation = "matched"
		if float64(d.Progress.Passed) != w.Passed || float64(d.Progress.Failed) != w.Failed {
			d.Reconciliation = "mismatch"
		}
	} else if w.Status == "completed" {
		d.Reconciliation = "unavailable"
	}
	return d, true
}

type BoardTrace struct {
	RunID     string       `json:"runId"`
	Epoch     string       `json:"epoch"`
	Revision  int          `json:"revision"`
	Integrity string       `json:"integrity"`
	Reason    string       `json:"reason,omitempty"`
	Synthetic bool         `json:"synthetic"`
	Board     Board        `json:"board"`
	Route     []string     `json:"route"`
	Entries   []TraceEntry `json:"entries"`
}

func (s *Store) BoardTrace(runID, boardID string) (BoardTrace, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runID]
	if !ok {
		return BoardTrace{}, false
	}
	b, ok := r.snapshot.Boards[boardID]
	if !ok {
		return BoardTrace{}, false
	}
	t := BoardTrace{RunID: runID, Epoch: r.snapshot.Epoch, Revision: r.snapshot.Revision, Integrity: r.snapshot.Integrity, Reason: r.snapshot.Reason, Synthetic: true, Board: b, Route: r.stations, Entries: r.traces[boardID]}
	data, _ := json.Marshal(t)
	var copied BoardTrace
	_ = json.Unmarshal(data, &copied)
	return copied, true
}
