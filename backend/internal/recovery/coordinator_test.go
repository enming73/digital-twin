package recovery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"smt-digital-twin/backend/internal/contract"
	"smt-digital-twin/backend/internal/twin"
)

const runID = "run-1"

func testEvents() []contract.Event {
	machine := "loader"
	events := []contract.Event{
		{SchemaVersion: 1, EventID: "event-00000001", RunID: runID, Source: "simulator", EventType: "workorder.started", Sequence: 1, SimTimeMs: 0, EmittedAt: "2026-11-10T00:00:00Z", Payload: map[string]any{"workOrderId": "wo-1", "quantity": float64(1), "synthetic": true}},
		{SchemaVersion: 1, EventID: "event-00000002", RunID: runID, Source: "simulator", MachineID: &machine, EventType: "pcb.entered", Sequence: 2, SimTimeMs: 0, EmittedAt: "2026-11-10T00:00:01Z", Payload: map[string]any{"boardId": "board-1", "workOrderId": "wo-1", "synthetic": true}},
		{SchemaVersion: 1, EventID: "event-00000003", RunID: runID, Source: "simulator", MachineID: &machine, EventType: "pcb.process.completed", Sequence: 3, SimTimeMs: 10, EmittedAt: "2026-11-10T00:00:02Z", Payload: map[string]any{"boardId": "board-1", "workOrderId": "wo-1", "synthetic": true}},
		{SchemaVersion: 1, EventID: "event-00000004", RunID: runID, Source: "simulator", MachineID: &machine, EventType: "pcb.exited", Sequence: 4, SimTimeMs: 10, EmittedAt: "2026-11-10T00:00:03Z", Payload: map[string]any{"boardId": "board-1", "workOrderId": "wo-1", "synthetic": true}},
		{SchemaVersion: 1, EventID: "event-00000005", RunID: runID, Source: "simulator", EventType: "workorder.completed", Sequence: 5, SimTimeMs: 10, EmittedAt: "2026-11-10T00:00:04Z", Payload: map[string]any{"workOrderId": "wo-1", "quantity": float64(1), "passed": float64(1), "failed": float64(0), "synthetic": true}},
	}
	return events
}

func marshalAll(t *testing.T, events []contract.Event) [][]byte {
	t.Helper()
	raw := make([][]byte, len(events))
	for i, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		raw[i] = data
	}
	return raw
}

func testDigest(envelopes [][]byte) string {
	h := sha256.New()
	var length [8]byte
	for _, raw := range envelopes {
		binary.BigEndian.PutUint64(length[:], uint64(len(raw)))
		h.Write(length[:])
		h.Write(raw)
	}
	return hex.EncodeToString(h.Sum(nil))
}

type fakeSource struct {
	mu         sync.Mutex
	events     [][]byte
	terminal   *Terminal
	generation string
	digest     string
	failures   int
	failErr    error
	acks       []int64
}

func newFakeSource(t *testing.T, events []contract.Event) *fakeSource {
	raw := marshalAll(t, events)
	return &fakeSource{events: raw, generation: "gen-1", digest: "scenario-digest", terminal: &Terminal{RunID: runID, FinalSequence: int64(len(raw)), EventCount: int64(len(raw)), SHA256: testDigest(raw)}}
}

func (f *fakeSource) Manifest(ctx context.Context, id string) (Manifest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failures > 0 {
		f.failures--
		return Manifest{}, f.failErr
	}
	terminal := f.terminal
	return Manifest{ProtocolVersion: 1, RunID: id, Generation: f.generation, ScenarioDigest: f.digest, Head: int64(len(f.events)), Terminal: terminal}, nil
}

func (f *fakeSource) ReadPage(ctx context.Context, id string, after int64, limit int) (Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failures > 0 {
		f.failures--
		return Page{}, f.failErr
	}
	page := Page{Head: int64(len(f.events))}
	for i := int(after); i < len(f.events) && len(page.Events) < limit; i++ {
		page.Events = append(page.Events, json.RawMessage(f.events[i]))
	}
	if after >= int64(len(f.events)) && f.terminal != nil {
		copyTerminal := *f.terminal
		page.Terminal = &copyTerminal
	}
	return page, nil
}

func (f *fakeSource) Ack(ctx context.Context, id, generation string, sequence int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if generation != f.generation {
		return errors.New("generation mismatch")
	}
	f.acks = append(f.acks, sequence)
	return nil
}

func newTestStore(t *testing.T) *twin.Store {
	t.Helper()
	return twin.NewStore("epoch-1", map[string][]string{runID: {"loader"}})
}

func applyPrefix(t *testing.T, store *twin.Store, events []contract.Event, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		if _, err := store.Apply(events[i], time.Now()); err != nil {
			t.Fatal(err)
		}
	}
}

func newCoordinator(t *testing.T, store *twin.Store, source *fakeSource) *Coordinator {
	t.Helper()
	return New(store, source, Config{RunID: runID, Generation: "gen-1", ScenarioDigest: "scenario-digest", RetryDelay: func(int) time.Duration { return time.Millisecond }})
}

func TestRecoverAppliesMissingTailAndPromotesComplete(t *testing.T) {
	events := testEvents()
	store := newTestStore(t)
	applyPrefix(t, store, events, 2)
	source := newFakeSource(t, events)
	coordinator := newCoordinator(t, store, source)
	if err := coordinator.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := store.Snapshot(runID)
	if snapshot.LastSequence != 5 || !snapshot.TerminalVerified {
		t.Fatalf("snapshot: sequence=%d verified=%v reason=%s", snapshot.LastSequence, snapshot.TerminalVerified, snapshot.Reason)
	}
	if len(source.acks) != 1 || source.acks[0] != 5 {
		t.Fatalf("acks: %v", source.acks)
	}
	if err := coordinator.Recover(context.Background()); err != nil {
		t.Fatalf("idempotent recovery: %v", err)
	}
	if len(source.acks) != 2 || source.acks[1] != 5 {
		t.Fatalf("repeat ack: %v", source.acks)
	}
}

func TestRecoverWithoutTerminalNeverClaimsComplete(t *testing.T) {
	events := testEvents()
	store := newTestStore(t)
	applyPrefix(t, store, events, 1)
	source := newFakeSource(t, events)
	source.terminal = nil
	coordinator := newCoordinator(t, store, source)
	if err := coordinator.Recover(context.Background()); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("expected incomplete, got %v", err)
	}
	snapshot, _ := store.Snapshot(runID)
	if snapshot.TerminalVerified {
		t.Fatal("completeness claimed without a terminal record")
	}
	if len(source.acks) != 0 {
		t.Fatalf("acked without terminal: %v", source.acks)
	}
}

func TestRecoverRejectsWrongIdentity(t *testing.T) {
	events := testEvents()
	for name, mutate := range map[string]func(*fakeSource){
		"generation": func(f *fakeSource) { f.generation = "other" },
		"digest":     func(f *fakeSource) { f.digest = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			store := newTestStore(t)
			applyPrefix(t, store, events, 2)
			source := newFakeSource(t, events)
			mutate(source)
			if err := newCoordinator(t, store, source).Recover(context.Background()); err == nil {
				t.Fatal("mismatched source identity accepted")
			}
			snapshot, _ := store.Snapshot(runID)
			if snapshot.TerminalVerified {
				t.Fatal("completeness claimed for a mismatched source")
			}
		})
	}
}

func TestRecoverRejectsTerminalDigestMismatch(t *testing.T) {
	events := testEvents()
	store := newTestStore(t)
	applyPrefix(t, store, events, 3)
	source := newFakeSource(t, events)
	source.terminal.SHA256 = "wrong"
	if err := newCoordinator(t, store, source).Recover(context.Background()); err == nil {
		t.Fatal("digest mismatch accepted")
	}
	snapshot, _ := store.Snapshot(runID)
	if snapshot.TerminalVerified || len(source.acks) != 0 {
		t.Fatalf("integrity=%s acks=%v", snapshot.Integrity, source.acks)
	}
}

func TestRecoverKeepsConflictFrozen(t *testing.T) {
	events := testEvents()
	store := newTestStore(t)
	applyPrefix(t, store, events, 1)
	altered := events[0]
	altered.Payload["quantity"] = float64(99)
	if _, err := store.Apply(altered, time.Now()); err == nil {
		t.Fatal("expected conflict")
	}
	source := newFakeSource(t, events)
	if err := newCoordinator(t, store, source).Recover(context.Background()); err == nil {
		t.Fatal("recovery thawed a conflict")
	}
	snapshot, _ := store.Snapshot(runID)
	if snapshot.Integrity != "conflict" {
		t.Fatalf("conflict cleared: %s", snapshot.Integrity)
	}
}

func TestRecoverReportsUnreachableSourceWithoutChangingProjection(t *testing.T) {
	events := testEvents()
	store := newTestStore(t)
	applyPrefix(t, store, events, 2)
	before, _ := store.Snapshot(runID)
	source := newFakeSource(t, events)
	source.failures = 1
	source.failErr = errors.New("source unavailable")
	err := newCoordinator(t, store, source).Recover(context.Background())
	if err == nil {
		t.Fatal("unreachable source reported success")
	}
	after, _ := store.Snapshot(runID)
	if after.LastSequence != before.LastSequence || after.TerminalVerified {
		t.Fatalf("projection changed: %+v", after)
	}
}

func TestRecoverSerializesConcurrentCalls(t *testing.T) {
	events := testEvents()
	store := newTestStore(t)
	applyPrefix(t, store, events, 2)
	source := newFakeSource(t, events)
	coordinator := newCoordinator(t, store, source)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- coordinator.Recover(context.Background())
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	snapshot, _ := store.Snapshot(runID)
	if snapshot.LastSequence != 5 {
		t.Fatalf("sequence %d", snapshot.LastSequence)
	}
	if len(source.acks) != 2 {
		t.Fatalf("acks %v", source.acks)
	}
}

func TestRecoverRejectsAlteredDuplicateInTail(t *testing.T) {
	events := testEvents()
	store := newTestStore(t)
	applyPrefix(t, store, events, 3)
	source := newFakeSource(t, events)
	altered := events[2]
	altered.Payload["boardId"] = "board-9"
	raw, err := json.Marshal(altered)
	if err != nil {
		t.Fatal(err)
	}
	source.events[2] = raw
	source.terminal.SHA256 = testDigest(source.events)
	if err := newCoordinator(t, store, source).Recover(context.Background()); err == nil {
		t.Fatal("altered duplicate accepted")
	}
	snapshot, _ := store.Snapshot(runID)
	if snapshot.TerminalVerified {
		t.Fatal("altered prefix promoted to complete")
	}
}

func TestRecoverRepairsDetectedGap(t *testing.T) {
	events := testEvents()
	store := newTestStore(t)
	applyPrefix(t, store, events, 1)
	if _, err := store.Apply(events[3], time.Now()); err == nil {
		t.Fatal("expected gap")
	}
	snapshot, _ := store.Snapshot(runID)
	if snapshot.Integrity != "gap" {
		t.Fatalf("expected gap, got %s", snapshot.Integrity)
	}
	source := newFakeSource(t, events)
	if err := newCoordinator(t, store, source).Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = store.Snapshot(runID)
	if snapshot.LastSequence != 5 || !snapshot.TerminalVerified {
		t.Fatalf("gap not repaired: sequence=%d verified=%v", snapshot.LastSequence, snapshot.TerminalVerified)
	}
}

func TestManifestProtocolVersionMismatchIsRejected(t *testing.T) {
	events := testEvents()
	store := newTestStore(t)
	source := newFakeSource(t, events)
	source.terminal = nil
	source.generation = "gen-1"
	coordinator := New(store, &versionedSource{fakeSource: source}, Config{RunID: runID, Generation: "gen-1", ScenarioDigest: "scenario-digest"})
	if err := coordinator.Recover(context.Background()); err == nil {
		t.Fatal("unsupported protocol accepted")
	}
}

type versionedSource struct{ *fakeSource }

func (v *versionedSource) Manifest(ctx context.Context, id string) (Manifest, error) {
	m, err := v.fakeSource.Manifest(ctx, id)
	m.ProtocolVersion = 99
	return m, err
}

func TestRecoverRequiresCompletedWorkOrderAsFinalEvent(t *testing.T) {
	events := testEvents()
	store := newTestStore(t)
	applyPrefix(t, store, events, 4)
	source := newFakeSource(t, events[:4])
	source.terminal = &Terminal{RunID: runID, FinalSequence: 4, EventCount: 4, SHA256: testDigest(source.events)}
	if err := newCoordinator(t, store, source).Recover(context.Background()); err == nil {
		t.Fatal("run without workorder.completed promoted")
	}
	snapshot, _ := store.Snapshot(runID)
	if snapshot.TerminalVerified {
		t.Fatal("incomplete run marked complete")
	}
}

var _ = bytes.Equal

func TestRecoverPromotesWhenLiveRunAlreadyLooksComplete(t *testing.T) {
	events := testEvents()
	store := newTestStore(t)
	applyPrefix(t, store, events, len(events))
	snapshot, _ := store.Snapshot(runID)
	if snapshot.Integrity != "complete" {
		t.Fatalf("precondition: %s", snapshot.Integrity)
	}
	source := newFakeSource(t, events)
	if err := newCoordinator(t, store, source).Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = store.Snapshot(runID)
	if !snapshot.TerminalVerified {
		t.Fatal("fully-live run was never terminal-verified")
	}
	if len(source.acks) != 1 {
		t.Fatalf("acks=%v", source.acks)
	}
}

func TestRecoverRejectsMalformedEventWithoutFreezingConflict(t *testing.T) {
	events := testEvents()
	store := newTestStore(t)
	applyPrefix(t, store, events, 1)
	source := newFakeSource(t, events)
	bad, err := json.Marshal(contract.Event{SchemaVersion: 1, EventID: "event-00000002", RunID: runID, Source: "simulator", EventType: "bogus.event", Sequence: 2, SimTimeMs: 1, EmittedAt: "2026-11-10T00:00:01Z", Payload: map[string]any{"synthetic": true}})
	if err != nil {
		t.Fatal(err)
	}
	source.events[1] = bad
	source.terminal.SHA256 = testDigest(source.events)
	if err := newCoordinator(t, store, source).Recover(context.Background()); err == nil {
		t.Fatal("malformed source event accepted")
	}
	snapshot, _ := store.Snapshot(runID)
	if snapshot.Integrity == "conflict" {
		t.Fatal("malformed response froze the run in conflict")
	}
	if snapshot.TerminalVerified {
		t.Fatal("malformed source promoted to complete")
	}
}

func TestRecoverChecksPrefixBeforeApplyingTail(t *testing.T) {
	events := testEvents()
	store := newTestStore(t)
	applyPrefix(t, store, events, 3)
	source := newFakeSource(t, events)
	altered := events[1]
	altered.Payload["quantity"] = float64(99)
	raw, err := json.Marshal(altered)
	if err != nil {
		t.Fatal(err)
	}
	source.events[1] = raw
	if err := newCoordinator(t, store, source).Recover(context.Background()); err == nil {
		t.Fatal("source with a different prefix accepted")
	}
	snapshot, _ := store.Snapshot(runID)
	if snapshot.LastSequence != 3 {
		t.Fatalf("tail applied before prefix verification: head=%d", snapshot.LastSequence)
	}
}
