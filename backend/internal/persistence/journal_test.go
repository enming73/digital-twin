package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"smt-digital-twin/backend/internal/contract"
	"smt-digital-twin/backend/internal/twin"
)

func TestJournalPersistsValidatedTwinEvents(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "r", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	store := twin.NewStoreWithJournal("epoch", map[string][]string{"r": {"loader"}}, NewJournal(db))
	station := "loader"
	events := []contract.Event{
		{SchemaVersion: 1, RunID: "r", EventID: "one", Sequence: 1, EventType: "workorder.started", EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1)}},
		{SchemaVersion: 1, RunID: "r", EventID: "two", Sequence: 2, SimTimeMs: 1, EventType: "pcb.entered", MachineID: &station, EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{"workOrderId": "wo", "boardId": "b"}},
	}
	for _, e := range events {
		if _, err := store.Apply(e, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	var eventsCount, traceRevision, orderRevision int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM events WHERE run_id='r'), (SELECT revision FROM board_trace WHERE run_id='r' AND board_id='b'), (SELECT revision FROM work_order_projection WHERE run_id='r' AND work_order_id='wo')`).Scan(&eventsCount, &traceRevision, &orderRevision); err != nil {
		t.Fatal(err)
	}
	if eventsCount != 2 || traceRevision != 2 || orderRevision != 1 {
		t.Fatalf("events=%d trace=%d order=%d", eventsCount, traceRevision, orderRevision)
	}
}

func TestJournalWritesCheckpointAtBoundary(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "r", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	s := twin.NewStoreWithJournal("old", map[string][]string{"r": {"loader"}}, NewJournal(db))
	station := "loader"
	for i := 1; i <= 500; i++ {
		e := contract.Event{SchemaVersion: 1, RunID: "r", EventID: fmt.Sprint(i), Sequence: i, SimTimeMs: int64(i), EventType: "machine.state.changed", MachineID: &station, EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{"observedState": "RUNNING", "flowState": "", "reason": ""}}
		if _, err := s.Apply(e, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	var seq, count int
	if err := db.QueryRow(ctx, `SELECT coalesce(max(sequence),0), count(*) FROM snapshots WHERE run_id='r'`).Scan(&seq, &count); err != nil {
		t.Fatal(err)
	}
	if seq != 500 || count != 1 {
		t.Fatalf("checkpoint sequence=%d count=%d", seq, count)
	}
}

func TestCheckpointRetentionKeepsRecentRows(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "r", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	store := twin.NewStoreWithJournal("old", map[string][]string{"r": {"loader"}}, NewJournal(db))
	station := "loader"
	for i := 1; i <= 6; i++ {
		e := contract.Event{SchemaVersion: 1, RunID: "r", EventID: fmt.Sprint(i), Sequence: i, SimTimeMs: int64(i), EventType: "machine.state.changed", MachineID: &station, EmittedAt: "2026-11-09T00:00:00.000000987Z", Payload: map[string]any{"observedState": "RUNNING", "flowState": "", "reason": ""}}
		if i == 1 {
			e.EventType = "workorder.started"
			e.MachineID = nil
			e.Payload = map[string]any{"workOrderId": "wo", "quantity": float64(1)}
		}
		if _, err := store.Apply(e, time.Now()); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveCheckpoint("r"); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := db.Query(ctx, `SELECT sequence FROM snapshots WHERE run_id='r' ORDER BY sequence`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var sequences []int
	for rows.Next() {
		var s int
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		sequences = append(sequences, s)
	}
	if len(sequences) != 3 || sequences[0] != 4 || sequences[1] != 5 || sequences[2] != 6 {
		t.Fatalf("retained %v", sequences)
	}
}

func TestJournalWritesFinalCheckpoint(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "r", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	store := twin.NewStoreWithJournal("old", map[string][]string{"r": {"loader"}}, NewJournal(db))
	e := contract.Event{SchemaVersion: 1, RunID: "r", EventID: "one", Sequence: 1, EventType: "workorder.started", EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1)}}
	if _, err := store.Apply(e, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCheckpoint("r"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCheckpoint("r"); err != nil {
		t.Fatalf("repeated save: %v", err)
	}
	var count, seq int
	if err := db.QueryRow(ctx, `SELECT count(*),coalesce(max(sequence),0) FROM snapshots WHERE run_id='r'`).Scan(&count, &seq); err != nil {
		t.Fatal(err)
	}
	if count != 1 || seq != 1 {
		t.Fatalf("checkpoints count=%d seq=%d", count, seq)
	}
	if _, err := db.Exec(ctx, `UPDATE snapshots SET checkpoint=jsonb_set(checkpoint,'{sha256}','"corrupt"') WHERE run_id='r' AND sequence=1`); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCheckpoint("r"); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	checkpoint, sequence, err := loadCheckpoint(ctx, tx, "r", []string{"loader"}, "new", 1, 1)
	if err != nil || checkpoint == nil || sequence != 1 {
		t.Fatalf("checkpoint not repaired: seq=%d err=%v", sequence, err)
	}
}

func TestJournalPersistsTwinConflict(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "r", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	store := twin.NewStoreWithJournal("old", map[string][]string{"r": {"loader"}}, NewJournal(db))
	e := contract.Event{SchemaVersion: 1, RunID: "r", EventID: "one", Sequence: 1, EventType: "workorder.started", EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1)}}
	if _, err := store.Apply(e, time.Now()); err != nil {
		t.Fatal(err)
	}
	e.Payload["quantity"] = float64(2)
	if _, err := store.Apply(e, time.Now()); err == nil {
		t.Fatal("conflict accepted")
	}
	restored, err := Restore(ctx, db, "r", []string{"loader"}, "new")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := restored.Snapshot("r")
	if snapshot.Integrity != "conflict" || snapshot.LastSequence != 1 {
		t.Fatalf("conflict not retained: %+v", snapshot)
	}
}

func TestJournalPersistsUncertainStatus(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "r", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	store := twin.NewStoreWithJournal("old", map[string][]string{"r": {"loader"}}, NewJournal(db))
	store.MarkUncertain("broker disconnected")
	var reason string
	if err := db.QueryRow(ctx, `SELECT reason FROM runs WHERE run_id='r'`).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	if reason != "broker disconnected" {
		t.Fatalf("reason not persisted: %q", reason)
	}
}
