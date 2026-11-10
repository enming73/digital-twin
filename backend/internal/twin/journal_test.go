package twin

import (
	"errors"
	"testing"
	"time"

	"smt-digital-twin/backend/internal/contract"
)

type failingJournal struct{}

func (failingJournal) SetIntegrity(_, _, _ string) error { return errors.New("storage unavailable") }

func (failingJournal) Commit(_ contract.Event, _ time.Time, _ Snapshot, _ []TraceEntry, _ *Checkpoint) error {
	return errors.New("storage unavailable")
}

type blockingJournal struct {
	entered chan struct{}
	release chan struct{}
}

func (blockingJournal) SetIntegrity(_, _, _ string) error { return nil }

func (j blockingJournal) Commit(_ contract.Event, _ time.Time, _ Snapshot, _ []TraceEntry, _ *Checkpoint) error {
	close(j.entered)
	<-j.release
	return nil
}

func TestJournalCommitPrecedesDelta(t *testing.T) {
	j := blockingJournal{entered: make(chan struct{}), release: make(chan struct{})}
	s := NewStoreWithJournal("epoch", map[string][]string{"r": {"loader"}}, j)
	_, updates, cancel, ok := s.Subscribe("r", 2)
	if !ok {
		t.Fatal("missing run")
	}
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := s.Apply(ev(1, "e1", "workorder.started", nil, map[string]any{"workOrderId": "wo", "quantity": float64(1)}), time.Now())
		done <- err
	}()
	<-j.entered
	select {
	case update := <-updates:
		t.Fatalf("delta before commit: %+v", update)
	default:
	}
	close(j.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	select {
	case update := <-updates:
		if update.Type != "delta" || update.Revision != 1 {
			t.Fatalf("unexpected update: %+v", update)
		}
	default:
		t.Fatal("missing committed delta")
	}
}

func TestJournalFailureDoesNotPublishOrApply(t *testing.T) {
	s := NewStoreWithJournal("epoch", map[string][]string{"r": {"loader"}}, failingJournal{})
	_, updates, cancel, ok := s.Subscribe("r", 2)
	if !ok {
		t.Fatal("missing run")
	}
	defer cancel()
	e := ev(1, "e1", "workorder.started", nil, map[string]any{"workOrderId": "wo", "quantity": float64(1)})
	if _, err := s.Apply(e, time.Now()); err == nil {
		t.Fatal("journal failure was ignored")
	}
	snapshot, _ := s.Snapshot("r")
	if snapshot.LastSequence != 0 || len(snapshot.WorkOrders) != 0 {
		t.Fatalf("uncommitted state exposed: %+v", snapshot)
	}
	select {
	case update := <-updates:
		t.Fatalf("uncommitted update published: %+v", update)
	default:
	}
}
