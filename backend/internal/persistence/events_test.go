package persistence

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"smt-digital-twin/backend/internal/contract"
)

func TestAppendDurableDeduplication(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "run", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	e := contract.Event{SchemaVersion: 1, EventID: "one", RunID: "run", Source: "simulator", EventType: "workorder.started", Sequence: 1, SimTimeMs: 0, EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1)}}
	at := time.Date(2026, 11, 9, 0, 1, 0, 0, time.UTC)
	if result, err := Append(ctx, db, e, at); err != nil || result.Duplicate {
		t.Fatalf("first append: %+v %v", result, err)
	}
	if result, err := Append(ctx, db, e, at.Add(time.Second)); err != nil || !result.Duplicate {
		t.Fatalf("duplicate: %+v %v", result, err)
	}
	var count int
	var actual time.Time
	if err := db.QueryRow(ctx, `SELECT count(*), min(ingested_at) FROM events WHERE run_id='run'`).Scan(&count, &actual); err != nil {
		t.Fatal(err)
	}
	if count != 1 || !actual.Equal(at) {
		t.Fatalf("count=%d ingested=%s", count, actual)
	}
	e.Payload["quantity"] = float64(2)
	if _, err := Append(ctx, db, e, at); err == nil {
		t.Fatal("conflicting event accepted")
	}
	var integrity string
	if err := db.QueryRow(ctx, `SELECT integrity FROM runs WHERE run_id='run'`).Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "conflict" {
		t.Fatalf("integrity=%s", integrity)
	}
}

func TestEnsureRunKeepsOriginalTopology(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	created, err := EnsureRun(ctx, db, "run", []string{"loader"})
	if err != nil || !created {
		t.Fatalf("create: %v %v", created, err)
	}
	created, err = EnsureRun(ctx, db, "run", []string{"loader"})
	if err != nil || created {
		t.Fatalf("existing: %v %v", created, err)
	}
	if _, err := EnsureRun(ctx, db, "run", []string{"other"}); err == nil {
		t.Fatal("mismatched topology accepted")
	}
}

func TestAppendDurableProjectionRollback(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "run", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	e := contract.Event{SchemaVersion: 1, EventID: "one", RunID: "run", Source: "simulator", EventType: "workorder.started", Sequence: 1, SimTimeMs: 0, EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{}}
	p := Projection{WorkOrders: map[string]json.RawMessage{"wo": json.RawMessage(`{"created":1}`)}, BoardTraces: map[string]json.RawMessage{"bad": json.RawMessage(`{`)}}
	if _, err := AppendProjection(ctx, db, e, time.Now(), p); err == nil {
		t.Fatal("invalid projection accepted")
	}
	var count, seq int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM events), last_sequence FROM runs WHERE run_id='run'`).Scan(&count, &seq); err != nil {
		t.Fatal(err)
	}
	if count != 0 || seq != 0 {
		t.Fatalf("transaction committed partially: count=%d sequence=%d", count, seq)
	}
	p.BoardTraces["bad"] = json.RawMessage(`{"entries":[]}`)
	if _, err := AppendProjection(ctx, db, e, time.Now(), p); err != nil {
		t.Fatal(err)
	}
	var orderRevision, boardRevision int
	if err := db.QueryRow(ctx, `SELECT (SELECT revision FROM work_order_projection WHERE run_id='run' AND work_order_id='wo'), (SELECT revision FROM board_trace WHERE run_id='run' AND board_id='bad')`).Scan(&orderRevision, &boardRevision); err != nil {
		t.Fatal(err)
	}
	if orderRevision != 1 || boardRevision != 1 {
		t.Fatalf("projection revisions %d %d", orderRevision, boardRevision)
	}
}

func TestAppendCheckpointFailureRollsBackEvent(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "run", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	e := contract.Event{SchemaVersion: 1, EventID: "one", RunID: "run", Sequence: 1, EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{}}
	if _, err := AppendProjection(ctx, db, e, time.Now(), Projection{Checkpoint: json.RawMessage(`{`)}); err == nil {
		t.Fatal("malformed checkpoint accepted")
	}
	var count, last int
	if err := db.QueryRow(ctx, `SELECT (SELECT count(*) FROM events), last_sequence FROM runs WHERE run_id='run'`).Scan(&count, &last); err != nil {
		t.Fatal(err)
	}
	if count != 0 || last != 0 {
		t.Fatalf("partially committed event: %d %d", count, last)
	}
}

func TestAppendDurableConcurrentDuplicates(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "run", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	e := contract.Event{SchemaVersion: 1, EventID: "one", RunID: "run", Source: "simulator", EventType: "workorder.started", Sequence: 1, SimTimeMs: 0, EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{}}
	var wg sync.WaitGroup
	results := make(chan AppendResult, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); result, err := Append(ctx, db, e, time.Now()); results <- result; errs <- err }()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	duplicates := 0
	for result := range results {
		if result.Duplicate {
			duplicates++
		}
	}
	if duplicates != 1 {
		t.Fatalf("expected one duplicate, got %d", duplicates)
	}
}

func TestAppendDurableGap(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "run", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	e := contract.Event{SchemaVersion: 1, EventID: "two", RunID: "run", Source: "simulator", EventType: "workorder.started", Sequence: 2, SimTimeMs: 1, EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{}}
	if _, err := Append(ctx, db, e, time.Now()); err == nil {
		t.Fatal("sequence gap accepted")
	}
	var integrity string
	var count int
	if err := db.QueryRow(ctx, `SELECT integrity, (SELECT count(*) FROM events) FROM runs WHERE run_id='run'`).Scan(&integrity, &count); err != nil {
		t.Fatal(err)
	}
	if integrity != "gap" || count != 0 {
		t.Fatalf("integrity=%s count=%d", integrity, count)
	}
}

func TestAppendDurableRecoveryExtendsGapButNeverConflict(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "run", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	started := contract.Event{SchemaVersion: 1, EventID: "one", RunID: "run", Source: "simulator", EventType: "workorder.started", Sequence: 1, SimTimeMs: 0, EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1)}}
	if _, err := Append(ctx, db, started, time.Now()); err != nil {
		t.Fatal(err)
	}
	gap := contract.Event{SchemaVersion: 1, EventID: "three", RunID: "run", Source: "simulator", EventType: "workorder.completed", Sequence: 3, SimTimeMs: 2, EmittedAt: "2026-11-09T00:00:02Z", Payload: map[string]any{}}
	if _, err := Append(ctx, db, gap, time.Now()); err == nil {
		t.Fatal("gap accepted by the normal writer")
	}
	fill := contract.Event{SchemaVersion: 1, EventID: "two", RunID: "run", Source: "simulator", EventType: "workorder.completed", Sequence: 2, SimTimeMs: 1, EmittedAt: "2026-11-09T00:00:01Z", Payload: map[string]any{}}
	if _, err := AppendProjectionRecovered(ctx, db, fill, time.Now(), Projection{}); err != nil {
		t.Fatalf("recovery could not extend a gap: %v", err)
	}
	var integrity string
	if err := db.QueryRow(ctx, `SELECT integrity FROM runs WHERE run_id='run'`).Scan(&integrity); err != nil {
		t.Fatal(err)
	}
	if integrity != "gap" {
		t.Fatalf("recovery cleared the gap before verification: %s", integrity)
	}
	if _, err := db.Exec(ctx, `UPDATE runs SET integrity='conflict' WHERE run_id='run'`); err != nil {
		t.Fatal(err)
	}
	next := contract.Event{SchemaVersion: 1, EventID: "four", RunID: "run", Source: "simulator", EventType: "workorder.completed", Sequence: 4, SimTimeMs: 3, EmittedAt: "2026-11-09T00:00:03Z", Payload: map[string]any{}}
	if _, err := AppendProjectionRecovered(ctx, db, next, time.Now(), Projection{}); err == nil {
		t.Fatal("recovery thawed a conflict")
	}
}

func TestAppendDurableRejectsTimeRegression(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "run", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	e := contract.Event{SchemaVersion: 1, EventID: "one", RunID: "run", Source: "simulator", EventType: "workorder.started", Sequence: 1, SimTimeMs: 100, EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{}}
	if _, err := Append(ctx, db, e, time.Now()); err != nil {
		t.Fatal(err)
	}
	e.EventID = "two"
	e.Sequence = 2
	e.SimTimeMs = 99
	if _, err := Append(ctx, db, e, time.Now()); err == nil {
		t.Fatal("simulated time regression accepted")
	}
	var integrity string
	var count int
	if err := db.QueryRow(ctx, `SELECT integrity, (SELECT count(*) FROM events) FROM runs WHERE run_id='run'`).Scan(&integrity, &count); err != nil {
		t.Fatal(err)
	}
	if integrity != "conflict" || count != 1 {
		t.Fatalf("integrity=%s count=%d", integrity, count)
	}
}

func TestAppendDurableRejectsUnknownSchema(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "run", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	e := contract.Event{SchemaVersion: 999, EventID: "one", RunID: "run", Sequence: 1, EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{}}
	if _, err := Append(ctx, db, e, time.Now()); err == nil {
		t.Fatal("unknown schema accepted")
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM events`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("unknown schema persisted: %d", count)
	}
}
