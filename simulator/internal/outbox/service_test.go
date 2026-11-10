package outbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const testGeneration = "gen-1"
const testDigest = "scenario-digest"

func newSourceFixture(t *testing.T) (*Store, *httptest.Server) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "outbox.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.RegisterRun(context.Background(), "run", testGeneration, testDigest); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewSourceHandler(store))
	t.Cleanup(server.Close)
	return store, server
}

func commitEvents(t *testing.T, store *Store, events ...[]byte) {
	t.Helper()
	if err := store.CommitBoundary(context.Background(), "run", events, []byte("state"), nil); err != nil {
		t.Fatal(err)
	}
}

func event(sequence int) []byte {
	return []byte(fmt.Sprintf(`{"runId":"run","eventId":"event-%08d","sequence":%d}`, sequence, sequence))
}

func getJSON(t *testing.T, url string, want int, target any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		t.Fatalf("GET %s: status %d want %d", url, resp.StatusCode, want)
	}
	if target != nil {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSourceAPIManifestAndPages(t *testing.T) {
	store, server := newSourceFixture(t)
	commitEvents(t, store, event(1), event(2))
	commitEvents(t, store, event(3))

	var manifest struct {
		ProtocolVersion int       `json:"protocolVersion"`
		RunID           string    `json:"runId"`
		Generation      string    `json:"sourceGenerationId"`
		ScenarioDigest  string    `json:"scenarioDigest"`
		Head            int64     `json:"head"`
		Terminal        *Terminal `json:"terminal"`
	}
	getJSON(t, server.URL+"/v1/runs/run/manifest", 200, &manifest)
	if manifest.ProtocolVersion != 1 || manifest.RunID != "run" || manifest.Generation != testGeneration || manifest.ScenarioDigest != testDigest {
		t.Fatalf("manifest identity: %+v", manifest)
	}
	if manifest.Head != 3 || manifest.Terminal != nil {
		t.Fatalf("manifest head/terminal: %+v", manifest)
	}

	page := readPage(t, server.URL, 0, 2)
	if page.Head != 3 || len(page.Events) != 2 || !bytes.Equal(page.Events[0], event(1)) || !bytes.Equal(page.Events[1], event(2)) {
		t.Fatalf("first page: head=%d events=%d", page.Head, len(page.Events))
	}
	page = readPage(t, server.URL, 2, 2)
	if len(page.Events) != 1 || !bytes.Equal(page.Events[0], event(3)) {
		t.Fatalf("second page: %d events", len(page.Events))
	}
	page = readPage(t, server.URL, 3, 2)
	if len(page.Events) != 0 || page.Head != 3 {
		t.Fatalf("empty page: %d events head=%d", len(page.Events), page.Head)
	}
}

type sourcePage struct {
	Events   []json.RawMessage `json:"events"`
	Head     int64             `json:"head"`
	Terminal *Terminal         `json:"terminal"`
}

func readPage(t *testing.T, base string, after int64, limit int) sourcePage {
	t.Helper()
	var page sourcePage
	getJSON(t, fmt.Sprintf("%s/v1/runs/run/events?after=%d&limit=%d", base, after, limit), 200, &page)
	return page
}

func TestSourceAPIRejectsInvalidRequests(t *testing.T) {
	_, server := newSourceFixture(t)
	for _, query := range []string{"after=-1&limit=1", "after=0&limit=0", "after=0&limit=501", "after=abc&limit=1", "limit=1"} {
		getJSON(t, server.URL+"/v1/runs/run/events?"+query, 400, nil)
	}
	getJSON(t, server.URL+"/v1/runs/missing/manifest", 404, nil)
	getJSON(t, server.URL+"/v1/runs/missing/events?after=0&limit=1", 404, nil)
	getJSON(t, server.URL+"/v1/runs/run/manifest?protocolVersion=2", 400, nil)
}

func TestSourceAPIAcknowledgementIsMonotonicAndGenerationScoped(t *testing.T) {
	store, server := newSourceFixture(t)
	commitEvents(t, store, event(1), event(2), event(3))

	ack := func(body string, want int) int64 {
		t.Helper()
		resp, err := http.Post(server.URL+"/v1/runs/run/ack", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("ack %s: status %d want %d", body, resp.StatusCode, want)
		}
		var result struct {
			Acknowledged int64 `json:"acknowledged"`
		}
		if resp.StatusCode == 200 {
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
		}
		return result.Acknowledged
	}

	if got := ack(`{"protocolVersion":1,"generation":"gen-1","sequence":2}`, 200); got != 2 {
		t.Fatalf("ack=%d", got)
	}
	if got := ack(`{"protocolVersion":1,"generation":"gen-1","sequence":2}`, 200); got != 2 {
		t.Fatalf("idempotent ack=%d", got)
	}
	ack(`{"protocolVersion":1,"generation":"gen-1","sequence":1}`, 409)
	ack(`{"protocolVersion":1,"generation":"gen-1","sequence":4}`, 409)
	ack(`{"protocolVersion":1,"generation":"stale","sequence":3}`, 409)
	ack(`{"protocolVersion":2,"generation":"gen-1","sequence":3}`, 400)
	ack(`{"protocolVersion":1,"generation":"gen-1","sequence":-1}`, 400)
	ack(`not json`, 400)
	if got := ack(`{"protocolVersion":1,"generation":"gen-1","sequence":3}`, 200); got != 3 {
		t.Fatalf("final ack=%d", got)
	}
}

func TestSourceAPITerminalReadableWithoutNewMessages(t *testing.T) {
	store, server := newSourceFixture(t)
	commitEvents(t, store, event(1))
	terminal := &Terminal{RunID: "run", FinalSequence: 2, EventCount: 2}
	if err := store.CommitBoundary(context.Background(), "run", [][]byte{event(2)}, []byte("done"), terminal); err != nil {
		t.Fatal(err)
	}
	page := readPage(t, server.URL, 0, 500)
	if page.Terminal == nil || page.Terminal.EventCount != 2 || page.Terminal.SHA256 == "" {
		t.Fatalf("terminal not served: %+v", page.Terminal)
	}
	var manifest struct {
		Head     int64     `json:"head"`
		Terminal *Terminal `json:"terminal"`
	}
	getJSON(t, server.URL+"/v1/runs/run/manifest", 200, &manifest)
	if manifest.Terminal == nil || manifest.Head != 2 {
		t.Fatalf("manifest terminal: %+v", manifest)
	}
}

func TestSourceAPISurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.db")
	store, err := Open(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RegisterRun(context.Background(), "run", testGeneration, testDigest); err != nil {
		t.Fatal(err)
	}
	commitEvents(t, store, event(1), event(2))
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server := httptest.NewServer(NewSourceHandler(store))
	defer server.Close()
	page := readPage(t, server.URL, 0, 10)
	if len(page.Events) != 2 || page.Head != 2 {
		t.Fatalf("after restart: %d events head=%d", len(page.Events), page.Head)
	}
}

func TestSourceAPIConcurrentAppendAndRead(t *testing.T) {
	store, server := newSourceFixture(t)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 1; i <= 300; i++ {
			if err := store.CommitBoundary(context.Background(), "run", [][]byte{event(i)}, []byte("state"), nil); err != nil {
				errs <- err
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			resp, err := http.Get(server.URL + "/v1/runs/run/events?after=0&limit=500")
			if err != nil {
				errs <- err
				return
			}
			var page sourcePage
			if resp.StatusCode != 200 {
				resp.Body.Close()
				errs <- fmt.Errorf("status %d", resp.StatusCode)
				return
			}
			if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
				resp.Body.Close()
				errs <- err
				return
			}
			resp.Body.Close()
			if int64(len(page.Events)) > page.Head {
				errs <- fmt.Errorf("%d events beyond head %d", len(page.Events), page.Head)
				return
			}
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestSourceAPIPageStopsBeforeByteLimit(t *testing.T) {
	store, server := newSourceFixture(t)
	large := bytes.Repeat([]byte("x"), 300<<10)
	for i := 1; i <= 6; i++ {
		payload := []byte(fmt.Sprintf(`{"runId":"run","eventId":"e%d","sequence":%d,"data":"%s"}`, i, i, large))
		commitEvents(t, store, payload)
	}
	page := readPage(t, server.URL, 0, 6)
	if len(page.Events) >= 6 {
		t.Fatalf("page returned %d events without enforcing the byte limit", len(page.Events))
	}
	if len(page.Events) == 0 {
		t.Fatal("byte-limited page returned no events")
	}
}

func TestSourceAPIRejectsOversizedEvent(t *testing.T) {
	store, server := newSourceFixture(t)
	huge := []byte(fmt.Sprintf(`{"runId":"run","eventId":"big","sequence":1,"data":"%s"}`, bytes.Repeat([]byte("x"), 2<<20)))
	commitEvents(t, store, huge)
	getJSON(t, server.URL+"/v1/runs/run/events?after=0&limit=1", http.StatusRequestEntityTooLarge, nil)
}
