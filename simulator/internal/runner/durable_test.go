package runner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"syscall"
	"testing"
	"time"

	"smt-digital-twin/simulator/internal/contract"
	"smt-digital-twin/simulator/internal/engine"
	"smt-digital-twin/simulator/internal/outbox"
	"smt-digital-twin/simulator/internal/publisher"
	"smt-digital-twin/simulator/internal/scenario"
)

func durableScenario() scenario.Scenario {
	return scenario.Scenario{
		Quantity:   6,
		DefectRate: .4,
		Stations: []scenario.Station{
			{ID: "loader", ProcessTimeMs: 5, InputCapacity: 2},
			{ID: "spi", ProcessTimeMs: 9, InputCapacity: 1},
			{ID: "unloader", ProcessTimeMs: 7, InputCapacity: 2},
		},
		Faults: []scenario.Fault{
			{SimTimeMs: 21, MachineID: "spi", Action: "raise", Reason: "test"},
			{SimTimeMs: 77, MachineID: "spi", Action: "clear", Reason: "recovered"},
		},
	}
}

type recordingSink struct {
	mu      sync.Mutex
	events  []contract.Event
	onCount func(int)
}

func (s *recordingSink) Publish(ctx context.Context, event contract.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	s.events = append(s.events, event)
	count := len(s.events)
	s.mu.Unlock()
	if s.onCount != nil {
		s.onCount(count)
	}
	return nil
}
func (s *recordingSink) Close(context.Context) error { return nil }
func (s *recordingSink) collected() []contract.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]contract.Event(nil), s.events...)
}

// flakyStore delegates to a real store but can fail a configurable number of
// commits before allowing them through, recording the publish count seen at
// each attempt so tests can prove publication never precedes commit.
type flakyStore struct {
	inner      *outbox.Store
	mu         sync.Mutex
	failures   []error
	attempts   int
	onAttempt  func(attempt int)
	publishes  func() int
	observed   []int
	committed  int
	commitErrs []error
}

func (f *flakyStore) CommitBoundary(ctx context.Context, runID string, events [][]byte, state []byte, terminal *outbox.Terminal) error {
	f.mu.Lock()
	f.attempts++
	attempt := f.attempts
	var injected error
	if len(f.failures) > 0 {
		injected = f.failures[0]
		f.failures = f.failures[1:]
	}
	if f.publishes != nil {
		f.observed = append(f.observed, f.publishes())
	}
	hook := f.onAttempt
	f.mu.Unlock()
	if hook != nil {
		hook(attempt)
	}
	if injected != nil {
		f.mu.Lock()
		f.commitErrs = append(f.commitErrs, injected)
		f.mu.Unlock()
		return injected
	}
	err := f.inner.CommitBoundary(ctx, runID, events, state, terminal)
	f.mu.Lock()
	if err == nil {
		f.committed++
	}
	f.mu.Unlock()
	return err
}

func TestDurablePausesOnCapacityAndPublishesOnlyAfterCommit(t *testing.T) {
	restoreRetry := durableRetryDelay
	durableRetryDelay = func(int) time.Duration { return time.Millisecond }
	t.Cleanup(func() { durableRetryDelay = restoreRetry })

	s := durableScenario()
	baselineStore, err := outbox.Open(filepath.Join(t.TempDir(), "baseline.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer baselineStore.Close()
	baselineEngine, err := engine.New(s, 2026, "durable-run")
	if err != nil {
		t.Fatal(err)
	}
	baselineSink := &recordingSink{}
	if err := RunDurable(context.Background(), baselineEngine, baselineStore, baselineSink, 1e9, nil); err != nil {
		t.Fatal(err)
	}

	sink := &recordingSink{}
	store := &flakyStore{inner: mustStore(t), failures: []error{outbox.ErrCapacity, outbox.ErrStorageUnavailable}}
	store.publishes = func() int { return len(sink.collected()) }
	var statuses []DurableStatus
	eng, err := engine.New(s, 2026, "durable-run")
	if err != nil {
		t.Fatal(err)
	}
	if err := RunDurableWithStatus(context.Background(), eng, store, sink, 1e9, nil, func(s DurableStatus) {
		statuses = append(statuses, s)
	}); err != nil {
		t.Fatal(err)
	}
	if store.attempts < 3 {
		t.Fatalf("expected retries after injected failures, attempts=%d", store.attempts)
	}
	for i, published := range store.observed {
		if i < len(store.commitErrs) && published != 0 {
			t.Fatalf("attempt %d attempted commit after publishing %d events", i+1, published)
		}
	}
	paused := false
	for _, status := range statuses {
		if status.Paused {
			paused = true
		}
	}
	if !paused {
		t.Fatal("storage failure was not reported as paused")
	}
	if !sameEvents(baselineSink.collected(), sink.collected()) {
		t.Fatal("durable run published different events than the uninterrupted run")
	}
	assertStoredMatchesEvents(t, store.inner, sink.collected())
	assertStoredMatchesBaseline(t, baselineStore, store.inner)
}

func TestDurableCrashAfterCommitResumesFromCheckpoint(t *testing.T) {
	restoreRetry := durableRetryDelay
	durableRetryDelay = func(int) time.Duration { return time.Millisecond }
	t.Cleanup(func() { durableRetryDelay = restoreRetry })

	s := durableScenario()
	store := mustStore(t)
	defer store.Close()
	eng, err := engine.New(s, 2026, "durable-run")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	crashed := &recordingSink{onCount: func(n int) {
		if n == 3 {
			cancel()
		}
	}}
	err = RunDurable(ctx, eng, store, crashed, 1e9, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}

	state, head, err := store.ReadState(context.Background(), "durable-run")
	if err != nil {
		t.Fatal(err)
	}
	if head == 0 {
		t.Fatal("no boundary committed before cancellation")
	}
	page, err := store.ReadPage(context.Background(), "durable-run", 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(page.Events)) < head {
		t.Fatalf("stored events %d below committed head %d", len(page.Events), head)
	}

	restored, err := engine.RestoreState(s, 2026, "durable-run", state)
	if err != nil {
		t.Fatal(err)
	}
	resumedSink := &recordingSink{}
	if err := RunDurable(context.Background(), restored, store, resumedSink, 1e9, nil); err != nil {
		t.Fatal(err)
	}

	baselineStore, err := outbox.Open(filepath.Join(t.TempDir(), "baseline.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer baselineStore.Close()
	baselineSink := &recordingSink{}
	baselineEngine, err := engine.New(s, 2026, "durable-run")
	if err != nil {
		t.Fatal(err)
	}
	if err := RunDurable(context.Background(), baselineEngine, baselineStore, baselineSink, 1e9, nil); err != nil {
		t.Fatal(err)
	}
	finalPage, err := store.ReadPage(context.Background(), "durable-run", 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	baselinePage, err := baselineStore.ReadPage(context.Background(), "durable-run", 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	if finalPage.Terminal == nil {
		t.Fatal("resumed run did not record a terminal boundary")
	}
	if finalPage.Terminal.EventCount != int64(len(baselinePage.Events)) {
		t.Fatalf("terminal count %d, baseline %d", finalPage.Terminal.EventCount, len(baselinePage.Events))
	}
	if !sameEnvelopes(baselinePage.Events, finalPage.Events) {
		t.Fatal("resumed durable log differs from uninterrupted log")
	}
	// Publication may stop mid-batch when the process is interrupted; the rest
	// of that committed batch is recovered by the backend from the source log,
	// and the resumed run publishes the remaining boundaries unchanged.
	crashedEvents := crashed.collected()
	resumedEvents := resumedSink.collected()
	baselineEvents := baselineSink.collected()
	if len(crashedEvents) > len(baselineEvents) || !sameEvents(baselineEvents[:len(crashedEvents)], crashedEvents) {
		t.Fatal("crash-phase publication was not an exact prefix of the uninterrupted run")
	}
	if len(resumedEvents) > len(baselineEvents) || !sameEvents(baselineEvents[len(baselineEvents)-len(resumedEvents):], resumedEvents) {
		t.Fatal("resumed publication was not the unchanged suffix of the uninterrupted run")
	}
}

func TestDurableStorageFailureRestoresCommittedCheckpoint(t *testing.T) {
	restoreRetry := durableRetryDelay
	durableRetryDelay = func(int) time.Duration { return time.Millisecond }
	t.Cleanup(func() { durableRetryDelay = restoreRetry })

	s := durableScenario()
	inner := mustStore(t)
	defer inner.Close()
	eng, err := engine.New(s, 2026, "durable-run")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < 3; i++ {
		events, done, err := eng.Next()
		if err != nil || done {
			t.Fatalf("boundary %d: done=%v err=%v", i, done, err)
		}
		raws := make([][]byte, 0, len(events))
		for _, event := range events {
			if event.EmittedAt == "" {
				event.EmittedAt = time.Now().UTC().Format(time.RFC3339Nano)
			}
			raw, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			raws = append(raws, raw)
		}
		post, err := eng.ExportState()
		if err != nil {
			t.Fatal(err)
		}
		if err := inner.CommitBoundary(ctx, eng.RunID(), raws, post, nil); err != nil {
			t.Fatal(err)
		}
	}
	pre, err := eng.ExportState()
	if err != nil {
		t.Fatal(err)
	}
	_, preHead, err := inner.ReadState(context.Background(), "durable-run")
	if err != nil {
		t.Fatal(err)
	}
	store := &flakyStore{inner: inner, failures: []error{outbox.ErrStorageUnavailable, outbox.ErrStorageUnavailable, outbox.ErrStorageUnavailable}}
	store.onAttempt = func(attempt int) {
		if attempt == 3 {
			cancel()
		}
	}
	err = RunDurable(ctx, eng, store, &recordingSink{}, 1e9, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation while paused, got %v", err)
	}
	state, head, err := inner.ReadState(context.Background(), "durable-run")
	if err != nil {
		t.Fatal(err)
	}
	if head != preHead {
		t.Fatalf("store head advanced from %d to %d during failed commits (attempts=%d errs=%d)", preHead, head, store.attempts, len(store.commitErrs))
	}
	after, err := eng.ExportState()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pre, after) {
		t.Fatal("engine did not restore the last committed checkpoint after storage failure")
	}
	if !reflect.DeepEqual(state, after) {
		t.Fatal("engine state diverged from the durable checkpoint")
	}
}

func TestDurableSIGKILLHelper(t *testing.T) {
	mode := os.Getenv("RUNNER_CRASH_MODE")
	if mode == "" {
		t.Skip("subprocess helper")
	}
	s, err := scenario.Load(os.Getenv("RUNNER_CRASH_SCENARIO"))
	if err != nil {
		t.Fatal(err)
	}
	e, err := engine.New(s, 2026, "durable-run")
	if err != nil {
		t.Fatal(err)
	}
	store, err := outbox.Open(os.Getenv("RUNNER_CRASH_OUTBOX"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := RunDurable(context.Background(), e, &killingStore{inner: store, mode: mode}, publisher.NewNDJSON(io.Discard), 1e9, nil); err != nil {
		t.Fatal(err)
	}
	t.Fatal("helper run was not killed")
}

type killingStore struct {
	inner *outbox.Store
	mode  string
	calls int
}

func (k *killingStore) CommitBoundary(ctx context.Context, runID string, events [][]byte, state []byte, terminal *outbox.Terminal) error {
	k.calls++
	if k.calls != 2 {
		return k.inner.CommitBoundary(ctx, runID, events, state, terminal)
	}
	switch k.mode {
	case "before":
		_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
		return errors.New("unreachable")
	case "after":
		if err := k.inner.CommitBoundary(ctx, runID, events, state, terminal); err != nil {
			return err
		}
		_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
		return errors.New("unreachable")
	default:
		return errors.New("unknown crash mode")
	}
}

func TestDurableSIGKILLRecoversFromCommittedBoundary(t *testing.T) {
	s := durableScenario()
	dir := t.TempDir()
	scenarioPath := filepath.Join(dir, "scenario.json")
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scenarioPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	baselineStore, err := outbox.Open(filepath.Join(dir, "baseline.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer baselineStore.Close()
	baselineEngine, err := engine.New(s, 2026, "durable-run")
	if err != nil {
		t.Fatal(err)
	}
	if err := RunDurable(context.Background(), baselineEngine, baselineStore, &recordingSink{}, 1e9, nil); err != nil {
		t.Fatal(err)
	}
	baselinePage, err := baselineStore.ReadPage(context.Background(), "durable-run", 0, 500)
	if err != nil {
		t.Fatal(err)
	}

	for _, mode := range []string{"before", "after"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "outbox.db")
			cmd := exec.Command(os.Args[0], "-test.run=^TestDurableSIGKILLHelper$", "-test.v")
			cmd.Env = append(os.Environ(),
				"RUNNER_CRASH_MODE="+mode,
				"RUNNER_CRASH_SCENARIO="+scenarioPath,
				"RUNNER_CRASH_OUTBOX="+path,
			)
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("helper survived: %s", output)
			}
			store, err := outbox.Open(path, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			state, head, err := store.ReadState(context.Background(), "durable-run")
			if err != nil {
				t.Fatal(err)
			}
			if head == 0 {
				t.Fatal("first boundary was not durable before the crash")
			}
			before, err := store.ReadPage(context.Background(), "durable-run", 0, 500)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "before" && int64(len(before.Events)) != head {
				t.Fatalf("uncommitted boundary leaked: %d events for head %d", len(before.Events), head)
			}
			restored, err := engine.RestoreState(s, 2026, "durable-run", state)
			if err != nil {
				t.Fatal(err)
			}
			if err := RunDurable(context.Background(), restored, store, &recordingSink{}, 1e9, nil); err != nil {
				t.Fatal(err)
			}
			page, err := store.ReadPage(context.Background(), "durable-run", 0, 500)
			if err != nil {
				t.Fatal(err)
			}
			if !sameEnvelopes(baselinePage.Events, page.Events) {
				t.Fatalf("recovered log differs from uninterrupted run (%d vs %d events)", len(baselinePage.Events), len(page.Events))
			}
			if page.Terminal == nil || page.Terminal.EventCount != int64(len(baselinePage.Events)) {
				t.Fatalf("terminal boundary missing after recovery: %+v", page.Terminal)
			}
		})
	}
}

func mustStore(t *testing.T) *outbox.Store {
	t.Helper()
	s, err := outbox.Open(filepath.Join(t.TempDir(), "outbox.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func sameEvents(a, b []contract.Event) bool {
	normalize := func(events []contract.Event) []string {
		out := make([]string, 0, len(events))
		for _, event := range events {
			event.EmittedAt = ""
			raw, err := json.Marshal(event)
			if err != nil {
				panic(err)
			}
			out = append(out, string(raw))
		}
		return out
	}
	return reflect.DeepEqual(normalize(a), normalize(b))
}

func sameEnvelopes(a, b [][]byte) bool {
	normalize := func(envelopes [][]byte) []string {
		out := make([]string, 0, len(envelopes))
		for _, raw := range envelopes {
			var event contract.Event
			if err := json.Unmarshal(raw, &event); err != nil {
				panic(err)
			}
			event.EmittedAt = ""
			normalized, err := json.Marshal(event)
			if err != nil {
				panic(err)
			}
			out = append(out, string(normalized))
		}
		return out
	}
	return reflect.DeepEqual(normalize(a), normalize(b))
}

func assertStoredMatchesEvents(t *testing.T, store *outbox.Store, events []contract.Event) {
	t.Helper()
	page, err := store.ReadPage(context.Background(), "durable-run", 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Events) != len(events) {
		t.Fatalf("stored %d envelopes for %d published events", len(page.Events), len(events))
	}
	stored := make([]contract.Event, 0, len(page.Events))
	for _, raw := range page.Events {
		var event contract.Event
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		stored = append(stored, event)
	}
	if !sameEvents(events, stored) {
		t.Fatal("stored envelopes differ from published events")
	}
}

func assertStoredMatchesBaseline(t *testing.T, baseline, actual *outbox.Store) {
	t.Helper()
	want, err := baseline.ReadPage(context.Background(), "durable-run", 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	got, err := actual.ReadPage(context.Background(), "durable-run", 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	if !sameEnvelopes(want.Events, got.Events) {
		t.Fatalf("stored log differs from uninterrupted run (%d vs %d events)", len(want.Events), len(got.Events))
	}
	if got.Terminal == nil || got.Terminal.FinalSequence != got.Head {
		t.Fatalf("terminal boundary missing or inconsistent: %+v head=%d", got.Terminal, got.Head)
	}
	if got.Terminal.EventCount != int64(len(got.Events)) {
		t.Fatalf("terminal event count %d, stored %d", got.Terminal.EventCount, len(got.Events))
	}
}
