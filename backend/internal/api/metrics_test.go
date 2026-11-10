package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"smt-digital-twin/backend/internal/contract"
	"smt-digital-twin/backend/internal/ingest"
	"smt-digital-twin/backend/internal/twin"
)

func TestMetricsCountAcceptedDuplicateAndGap(t *testing.T) {
	s := twin.NewStore("e", map[string][]string{"r": {"loader"}})
	e := contract.Event{RunID: "r", EventID: "one", Sequence: 1, EventType: "workorder.started", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1)}}
	if _, err := s.Apply(e, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(e, time.Now()); err != nil {
		t.Fatal(err)
	}
	e.EventID = "three"
	e.Sequence = 3
	if _, err := s.Apply(e, time.Now()); err == nil {
		t.Fatal("gap accepted")
	}
	w := httptest.NewRecorder()
	NewHandler(s, ingest.NewConsumer(s)).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	var m struct {
		Accepted     int            `json:"accepted"`
		Conflicts    int            `json:"conflicts"`
		Duplicates   int            `json:"duplicates"`
		Gaps         int            `json:"gaps"`
		Memory       uint64         `json:"goMemoryBytes"`
		LastSequence map[string]int `json:"lastSequence"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.Accepted != 1 || m.Duplicates != 1 || m.Gaps != 1 || m.Conflicts != 0 || m.Memory == 0 || m.LastSequence["r"] != 1 {
		t.Fatalf("metrics %+v", m)
	}
}

func TestMetricsCountConflicts(t *testing.T) {
	s := twin.NewStore("e", map[string][]string{"r": {"loader"}})
	e := contract.Event{RunID: "r", EventID: "one", Sequence: 1, EventType: "workorder.started", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1)}}
	if _, err := s.Apply(e, time.Now()); err != nil {
		t.Fatal(err)
	}
	e.Payload["quantity"] = float64(2)
	if _, err := s.Apply(e, time.Now()); err == nil {
		t.Fatal("conflict accepted")
	}
	w := httptest.NewRecorder()
	NewHandler(s, ingest.NewConsumer(s)).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))
	var m struct {
		Conflicts int `json:"conflicts"`
		Gaps      int `json:"gaps"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.Conflicts != 1 || m.Gaps != 0 {
		t.Fatalf("metrics %+v", m)
	}
}
