package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"smt-digital-twin/backend/internal/ingest"
	"smt-digital-twin/backend/internal/persistence"
	"smt-digital-twin/backend/internal/replay"
	"smt-digital-twin/backend/internal/twin"
)

type fakeHistory struct{}

func (fakeHistory) Replay(_ context.Context, id string, at int64) (replay.HistoricalSnapshot, error) {
	if id == "missing" {
		return replay.HistoricalSnapshot{}, persistence.ErrRunNotFound
	}
	if id == "version" {
		return replay.HistoricalSnapshot{}, persistence.ErrUnsupportedVersion
	}
	if id == "down" {
		return replay.HistoricalSnapshot{}, errors.New("database unavailable")
	}
	return replay.HistoricalSnapshot{Mode: "historical", Synthetic: true, RequestedSimTimeMs: at, Snapshot: twin.Snapshot{RunID: id}}, nil
}
func (fakeHistory) Events(_ context.Context, id string, from, to int64, after, limit int) ([]persistence.StoredEvent, error) {
	if id == "missing" {
		return nil, persistence.ErrRunNotFound
	}
	if from != 5 || to != 10 || after != 3 || limit != 2 {
		return nil, errors.New("wrong range")
	}
	return []persistence.StoredEvent{}, nil
}

func TestHistoricalHTTPValidation(t *testing.T) {
	s := twin.NewStore("e", map[string][]string{"r": {"loader"}})
	h := NewHandlerWithHistory(s, ingest.NewConsumer(s), nil, fakeHistory{})
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/api/runs/r/replay?simTimeMs=5", 200},
		{"/api/runs/r/replay?simTimeMs=-1", 400},
		{"/api/runs/r/replay?simTimeMs=no", 400},
		{"/api/runs/missing/replay?simTimeMs=0", 404},
		{"/api/runs/version/replay?simTimeMs=0", 422},
		{"/api/runs/down/replay?simTimeMs=0", 503},
		{"/api/runs/r/events?fromSimTimeMs=5&toSimTimeMs=10&afterSequence=3&limit=2", 200},
		{"/api/runs/r/events?fromSimTimeMs=10&toSimTimeMs=5", 400},
		{"/api/runs/r/events?fromSimTimeMs=0&toSimTimeMs=10&limit=1001", 400},
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != tc.status {
			t.Fatalf("%s: got %d want %d: %s", tc.path, w.Code, tc.status, w.Body.String())
		}
	}
}
