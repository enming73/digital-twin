package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"smt-digital-twin/backend/internal/ingest"
	"smt-digital-twin/backend/internal/twin"
)

func TestDatabaseReadinessControlsHealth(t *testing.T) {
	store := twin.NewStore("epoch", map[string][]string{"r": {"loader"}})
	consumer := ingest.NewConsumer(store)
	gen := consumer.Connected()
	consumer.Subscribed(gen)
	fail := true
	h := NewHandlerWithReadiness(store, consumer, func(context.Context) error {
		if fail {
			return errors.New("database unavailable")
		}
		return nil
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("database unavailable returned %d", w.Code)
	}
	fail = false
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("database ready returned %d", w.Code)
	}
}
