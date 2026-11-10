package persistence

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"smt-digital-twin/backend/internal/contract"
	"smt-digital-twin/backend/internal/twin"
)

func checkpointFixture(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
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
	for i := 1; i <= 501; i++ {
		e := contract.Event{SchemaVersion: 1, RunID: "r", EventID: fmt.Sprint(i), Sequence: i, SimTimeMs: int64(i), EventType: "machine.state.changed", MachineID: &station, EmittedAt: "2026-11-09T00:00:00.000000987Z", Payload: map[string]any{"observedState": "RUNNING", "flowState": "", "reason": ""}}
		if i == 1 {
			e.EventType = "workorder.started"
			e.MachineID = nil
			e.Payload = map[string]any{"workOrderId": "wo", "quantity": float64(1)}
		}
		if i == 2 {
			e.EventType = "pcb.entered"
			e.Payload = map[string]any{"workOrderId": "wo", "boardId": "b"}
		}
		if _, err := s.Apply(e, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	return db, ctx
}

func TestCheckpointToleratesSubMicrosecondProjectionDrift(t *testing.T) {
	db, ctx := traceCheckpointFixture(t)
	if _, err := db.Exec(ctx, `UPDATE board_trace SET projection=jsonb_set(projection,'{entries,0,ingestedAt}',to_jsonb((projection->'entries'->0->>'ingestedAt')::timestamptz + interval '0.0000005 seconds')) WHERE run_id='r' AND board_id='b'`); err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(ctx, db, "r", []string{"loader"}, "new")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := restored.Snapshot("r")
	if snapshot.LastSequence != 501 || restored.Metrics().Accepted != 1 {
		t.Fatalf("checkpoint did not accept sub-microsecond drift: seq=%+v accepted=%d", snapshot, restored.Metrics().Accepted)
	}
}

func TestCheckpointRejectsMicrosecondProjectionDrift(t *testing.T) {
	db, ctx := traceCheckpointFixture(t)
	if _, err := db.Exec(ctx, `UPDATE board_trace SET projection=jsonb_set(projection,'{entries,0,ingestedAt}',to_jsonb((projection->'entries'->0->>'ingestedAt')::timestamptz + interval '0.001 seconds')) WHERE run_id='r' AND board_id='b'`); err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(ctx, db, "r", []string{"loader"}, "new")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := restored.Snapshot("r")
	// Fallback replays all 501 events; the checkpoint path would apply only the suffix.
	if snapshot.LastSequence != 501 || restored.Metrics().Accepted != 501 {
		t.Fatalf("drift not rejected: seq=%+v accepted=%d", snapshot, restored.Metrics().Accepted)
	}
}

func traceCheckpointFixture(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
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
	for i := 1; i <= 501; i++ {
		e := contract.Event{SchemaVersion: 1, RunID: "r", EventID: fmt.Sprint(i), Sequence: i, SimTimeMs: int64(i), EventType: "machine.state.changed", MachineID: &station, EmittedAt: "2026-11-09T00:00:00.000000987Z", Payload: map[string]any{"observedState": "RUNNING", "flowState": "", "reason": ""}}
		switch i {
		case 1:
			e.EventType = "workorder.started"
			e.MachineID = nil
			e.Payload = map[string]any{"workOrderId": "wo", "quantity": float64(1)}
		case 2:
			e.EventType = "pcb.entered"
			e.Payload = map[string]any{"workOrderId": "wo", "boardId": "b"}
		}
		if _, err := s.Apply(e, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	return db, ctx
}

func TestHistoricalCheckpointMatchesFullReplay(t *testing.T) {
	db, ctx := checkpointFixture(t)
	all, err := ReadEvents(ctx, db, "r", 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []int64{1, 2, 499, 500, 501, 999} {
		full := twin.NewStore("historical", map[string][]string{"r": {"loader"}})
		for _, row := range all {
			if row.Event.SimTimeMs > target {
				break
			}
			if _, err := full.Apply(row.Event, row.IngestedAt); err != nil {
				t.Fatal(err)
			}
		}
		source, err := LoadHistorical(ctx, db, "r", target)
		if err != nil {
			t.Fatal(err)
		}
		actual := source.Checkpoint
		if actual == nil {
			actual = twin.NewStore("historical", map[string][]string{"r": {"loader"}})
		}
		for _, row := range source.Events {
			if _, err := actual.Apply(row.Event, row.IngestedAt); err != nil {
				t.Fatal(err)
			}
		}
		want, _ := full.Snapshot("r")
		got, _ := actual.Snapshot("r")
		want.Integrity, want.Reason = "", ""
		got.Integrity, got.Reason = "", ""
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("time %d: snapshots differ", target)
		}
		if target >= 2 {
			wantTrace, _ := full.BoardTrace("r", "b")
			gotTrace, _ := actual.BoardTrace("r", "b")
			wantTrace.Integrity, wantTrace.Reason = "", ""
			gotTrace.Integrity, gotTrace.Reason = "", ""
			if !reflect.DeepEqual(wantTrace, gotTrace) {
				t.Fatalf("time %d: traces differ", target)
			}
		}
	}
}

func TestRestoreFromCheckpointAndSuffix(t *testing.T) {
	db, ctx := checkpointFixture(t)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	checkpointStore, seq, err := loadCheckpoint(ctx, tx, "r", []string{"loader"}, "new", 501, 501)
	if err != nil || checkpointStore == nil || seq != 500 {
		t.Fatalf("checkpoint seq=%d err=%v", seq, err)
	}
	_ = tx.Rollback(ctx)
	restored, err := Restore(ctx, db, "r", []string{"loader"}, "new")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := restored.Snapshot("r")
	if snapshot.LastSequence != 501 || snapshot.SimTimeMs != 501 || snapshot.Epoch != "new" {
		t.Fatalf("snapshot %+v", snapshot)
	}
}

func TestHistoricalUsesCheckpointBeforeRequestedTime(t *testing.T) {
	db, ctx := checkpointFixture(t)
	before, err := LoadHistorical(ctx, db, "r", 499)
	if err != nil {
		t.Fatal(err)
	}
	if before.Checkpoint != nil || len(before.Events) != 499 {
		t.Fatalf("before checkpoint: %d", len(before.Events))
	}
	after, err := LoadHistorical(ctx, db, "r", 501)
	if err != nil {
		t.Fatal(err)
	}
	if after.Checkpoint == nil || len(after.Events) != 1 || after.Events[0].Event.Sequence != 501 {
		t.Fatalf("after checkpoint: %+v", after)
	}
}

func TestRestoreFallsBackFromCorruptCheckpoint(t *testing.T) {
	db, ctx := checkpointFixture(t)
	if _, err := db.Exec(ctx, `UPDATE snapshots SET checkpoint=jsonb_set(checkpoint,'{sha256}','"bad"') WHERE run_id='r'`); err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(ctx, db, "r", []string{"loader"}, "new")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := restored.Snapshot("r")
	if snapshot.LastSequence != 501 {
		t.Fatalf("snapshot %+v", snapshot)
	}
}

func TestRestoreRejectsCheckpointWhenStoredEventChanges(t *testing.T) {
	db, ctx := checkpointFixture(t)
	if _, err := db.Exec(ctx, `UPDATE events SET envelope=jsonb_set(envelope,'{runId}','"wrong"') WHERE run_id='r' AND sequence=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(ctx, db, "r", []string{"loader"}, "new"); err == nil {
		t.Fatal("checkpoint hid corrupted source event")
	}
}

func TestRestoreRejectsCheckpointWhenEventColumnChanges(t *testing.T) {
	db, ctx := checkpointFixture(t)
	if _, err := db.Exec(ctx, `UPDATE events SET sim_time_ms=999 WHERE run_id='r' AND sequence=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(ctx, db, "r", []string{"loader"}, "new"); err == nil {
		t.Fatal("checkpoint hid corrupted event time column")
	}
}

func TestHistoricalFallsBackWhenTraceIngestionTimeChanges(t *testing.T) {
	db, ctx := checkpointFixture(t)
	if _, err := db.Exec(ctx, `UPDATE events SET ingested_at=ingested_at+interval '1 second' WHERE run_id='r' AND sequence=2`); err != nil {
		t.Fatal(err)
	}
	source, err := LoadHistorical(ctx, db, "r", 501)
	if err != nil {
		t.Fatal(err)
	}
	if source.Checkpoint != nil || len(source.Events) != 501 {
		t.Fatal("checkpoint hid changed trace ingestion time")
	}
}

func TestRestoreCommittedPrefix(t *testing.T) {
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
	at := time.Date(2026, 11, 9, 0, 1, 0, 0, time.UTC)
	events := []contract.Event{
		{SchemaVersion: 1, RunID: "r", EventID: "one", Sequence: 1, EventType: "workorder.started", EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1)}},
		{SchemaVersion: 1, RunID: "r", EventID: "two", Sequence: 2, SimTimeMs: 1, EventType: "pcb.entered", MachineID: &station, EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{"workOrderId": "wo", "boardId": "b"}},
	}
	for i, e := range events {
		if _, err := s.Apply(e, at.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	restored, err := Restore(ctx, db, "r", []string{"loader"}, "new")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, ok := restored.Snapshot("r")
	if !ok || snapshot.Epoch != "new" || snapshot.LastSequence != 2 || snapshot.Integrity != "uncertain" || !snapshot.LastIngestedAt.Equal(at.Add(time.Second)) {
		t.Fatalf("restored snapshot: %+v", snapshot)
	}
	trace, ok := restored.BoardTrace("r", "b")
	if !ok || len(trace.Entries) != 1 || !trace.Entries[0].IngestedAt.Equal(at.Add(time.Second)) {
		t.Fatalf("restored trace: %+v", trace)
	}
	result, err := restored.Apply(events[1], at.Add(time.Hour))
	if err != nil || !result.Duplicate {
		t.Fatalf("restored duplicate: %+v %v", result, err)
	}
	if _, err := Restore(ctx, db, "r", []string{"different"}, "new"); err == nil {
		t.Fatal("topology mismatch accepted")
	}
}

func TestRestoreRejectsRegressingTime(t *testing.T) {
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
	events := []contract.Event{
		{SchemaVersion: 1, RunID: "r", EventID: "one", Sequence: 1, SimTimeMs: 100, EventType: "workorder.started", EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1)}},
		{SchemaVersion: 1, RunID: "r", EventID: "two", Sequence: 2, SimTimeMs: 100, EventType: "pcb.entered", MachineID: &station, EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{"workOrderId": "wo", "boardId": "b"}},
	}
	for _, e := range events {
		if _, err := s.Apply(e, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `UPDATE events SET sim_time_ms=99,envelope=jsonb_set(envelope,'{simTimeMs}','99') WHERE run_id='r' AND sequence=2`); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(ctx, db, "r", []string{"loader"}, "new"); err == nil {
		t.Fatal("regressing event time accepted")
	}
}

func TestRestoreRequiresRunLease(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "r", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	if _, err := RestoreWithLease(ctx, db, "r", []string{"loader"}, "new", nil); err == nil {
		t.Fatal("restore without lease accepted")
	}
	lease, err := AcquireRunWriter(ctx, db, "other")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close(ctx)
	if _, err := RestoreWithLease(ctx, db, "r", []string{"loader"}, "new", lease); err == nil {
		t.Fatal("wrong run lease accepted")
	}
}

func TestRestoreRejectsExtraCommittedEvent(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "r", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(ctx, `INSERT INTO events (run_id,event_id,sequence,schema_version,sim_time_ms,emitted_at,ingested_at,envelope) VALUES ('r','orphan',1,1,0,now(),now(),'{}')`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(ctx, db, "r", []string{"loader"}, "new"); err == nil {
		t.Fatal("extra event beyond run cursor accepted")
	}
}

func TestRestoreLeasePersistsRestartUncertainty(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "r", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE runs SET integrity='complete' WHERE run_id='r'`); err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireRunWriter(ctx, db, "r")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close(ctx)
	if _, err := RestoreWithLease(ctx, db, "r", []string{"loader"}, "new", lease); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := db.QueryRow(ctx, `SELECT integrity FROM runs WHERE run_id='r'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "uncertain" {
		t.Fatalf("restart uncertainty not persisted: %s", status)
	}
}
