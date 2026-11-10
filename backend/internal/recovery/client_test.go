package recovery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPClientReadsManifestAndPages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/runs/run-1/manifest":
			_ = json.NewEncoder(w).Encode(Manifest{ProtocolVersion: 1, RunID: "run-1", Generation: "gen-1", ScenarioDigest: "digest", Head: 2, Terminal: &Terminal{RunID: "run-1", FinalSequence: 2, EventCount: 2, SHA256: "abc"}})
		case "/v1/runs/run-1/events":
			if r.URL.Query().Get("after") != "1" || r.URL.Query().Get("limit") != "500" {
				http.Error(w, "bad bounds", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(Page{Head: 2, Events: []json.RawMessage{json.RawMessage(`{"sequence":2}`)}})
		case "/v1/runs/run-1/ack":
			var body struct {
				ProtocolVersion int    `json:"protocolVersion"`
				Generation      string `json:"generation"`
				Sequence        int64  `json:"sequence"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ProtocolVersion != 1 || body.Generation != "gen-1" || body.Sequence != 2 {
				http.Error(w, "bad ack", http.StatusConflict)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]int64{"acknowledged": 2})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL)
	ctx := context.Background()
	manifest, err := client.Manifest(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Generation != "gen-1" || manifest.Terminal == nil || manifest.Terminal.SHA256 != "abc" {
		t.Fatalf("manifest: %+v", manifest)
	}
	page, err := client.ReadPage(ctx, "run-1", 1, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != 1 || page.Head != 2 {
		t.Fatalf("page: %+v", page)
	}
	if err := client.Ack(ctx, "run-1", "gen-1", 2); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPClientRejectsClientErrorsWithoutRetry(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "stale generation", http.StatusConflict)
	}))
	defer server.Close()
	client := NewHTTPClient(server.URL)
	client.retryDelay = func(int) time.Duration { return time.Millisecond }
	if err := client.Ack(context.Background(), "run-1", "gen-1", 2); err == nil {
		t.Fatal("client error accepted")
	}
	if requests != 1 {
		t.Fatalf("retried a non-retryable status %d times", requests)
	}
}

func TestHTTPClientRetriesServerErrors(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests < 3 {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(Manifest{ProtocolVersion: 1, RunID: "run-1"})
	}))
	defer server.Close()
	client := NewHTTPClient(server.URL)
	client.retryDelay = func(int) time.Duration { return time.Millisecond }
	if _, err := client.Manifest(context.Background(), "run-1"); err != nil {
		t.Fatal(err)
	}
	if requests != 3 {
		t.Fatalf("requests=%d", requests)
	}
}
