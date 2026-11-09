package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"smt-digital-twin/backend/internal/contract"
	"smt-digital-twin/backend/internal/ingest"
	"smt-digital-twin/backend/internal/twin"
)

func TestWorkOrderAndTraceHTTP(t *testing.T) {
	s := twin.NewStore("epoch", map[string][]string{"r": {"spi"}})
	m := "spi"
	for _, e := range []contract.Event{
		{RunID: "r", EventID: "1", Sequence: 1, EventType: "workorder.started", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1)}},
		{RunID: "r", EventID: "2", Sequence: 2, MachineID: &m, EventType: "pcb.entered", Payload: map[string]any{"workOrderId": "wo", "boardId": "p"}},
	} {
		if _, err := s.Apply(e, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler(s, ingest.NewConsumer(s))
	for _, path := range []string{"/api/runs/r/workorders/wo", "/api/runs/r/boards/p/trace"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		var body map[string]any
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &body) != nil || body["epoch"] != "epoch" || body["revision"] != float64(2) || body["synthetic"] != true {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/api/runs/r/workorders/missing", "/api/runs/r/boards/missing/trace", "/api/runs/missing/boards/p/trace"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 404 {
			t.Fatalf("%s: %d", path, w.Code)
		}
	}
	_, _ = s.Apply(contract.Event{RunID: "r", EventID: "gap", Sequence: 4, EventType: "pcb.exited", MachineID: &m, Payload: map[string]any{"workOrderId": "wo", "boardId": "p"}}, time.Now())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/runs/r/boards/p/trace", nil))
	var trace twin.BoardTrace
	if err := json.Unmarshal(w.Body.Bytes(), &trace); err != nil || trace.Integrity != "gap" || trace.Revision != 2 || len(trace.Entries) != 1 {
		t.Fatalf("gap: %+v %v", trace, err)
	}
}
