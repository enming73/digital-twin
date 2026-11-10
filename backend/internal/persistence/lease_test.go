package persistence

import (
	"context"
	"testing"
	"time"

	"smt-digital-twin/backend/internal/contract"
	"smt-digital-twin/backend/internal/twin"
)

func TestWriterLeaseExcludesSameRun(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	one, err := AcquireRunWriter(ctx, db, "r")
	if err != nil {
		t.Fatal(err)
	}
	defer one.Close(ctx)
	if _, err := AcquireRunWriter(ctx, db, "r"); err == nil {
		t.Fatal("second run writer accepted")
	}
	other, err := AcquireRunWriter(ctx, db, "other")
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := one.Close(ctx); err != nil {
		t.Fatal(err)
	}
	again, err := AcquireRunWriter(ctx, db, "r")
	if err != nil {
		t.Fatal(err)
	}
	if err := again.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestWriterLeaseLossStopsOldWriter(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "r", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireRunWriter(ctx, db, "r")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close(ctx)
	store := twin.NewStoreWithJournal("epoch", map[string][]string{"r": {"loader"}}, NewLeasedJournal(lease))
	var pid int
	if err := lease.conn.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `SELECT pg_terminate_backend($1)`, pid); err != nil {
		t.Fatal(err)
	}
	if err := lease.Ping(ctx); err == nil {
		t.Fatal("lost lease reported ready")
	}
	e := contract.Event{SchemaVersion: 1, RunID: "r", EventID: "one", Sequence: 1, EventType: "workorder.started", EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1)}}
	if _, err := store.Apply(e, time.Now()); err == nil {
		t.Fatal("lost-lock writer accepted event")
	}
	snapshot, _ := store.Snapshot("r")
	if snapshot.LastSequence != 0 {
		t.Fatal("lost-lock writer changed state")
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("lost-lock writer persisted event")
	}
}
