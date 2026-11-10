package persistence

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"smt-digital-twin/backend/internal/contract"
)

func TestReadEventsPreservesOrderAndIngestionTime(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "run", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 9, 0, 1, 0, 0, time.UTC)
	for i, id := range []string{"one", "two", "three"} {
		e := contract.Event{SchemaVersion: 1, EventID: id, RunID: "run", Sequence: i + 1, SimTimeMs: int64(i / 2), EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{}}
		if _, err := Append(ctx, db, e, at.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := ReadEvents(ctx, db, "run", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Event.EventID != "two" || rows[1].Event.EventID != "three" || !rows[0].IngestedAt.Equal(at.Add(time.Second)) || !rows[1].IngestedAt.Equal(at.Add(2*time.Second)) {
		t.Fatalf("wrong ordered events or ingest time: %+v", rows)
	}
}

func TestReadEventsRejectsInvalidBounds(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ after, limit int }{{-1, 1}, {0, 0}, {0, 1001}} {
		if _, err := ReadEvents(ctx, db, "run", test.after, test.limit); err == nil {
			t.Fatalf("accepted invalid bounds: %+v", test)
		}
	}
}

func TestReadEventsRejectsEnvelopeSequenceMismatch(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "run", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	e := contract.Event{SchemaVersion: 1, EventID: "one", RunID: "run", Sequence: 1, EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{}}
	if _, err := Append(ctx, db, e, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE events SET envelope=jsonb_set(envelope,'{sequence}','99') WHERE run_id='run'`); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadEvents(ctx, db, "run", 0, 10); err == nil {
		t.Fatal("corrupt envelope accepted")
	}
}

func TestReadTimeRangeInclusiveAndPaginated(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "run", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	for i, simTime := range []int64{0, 5, 5, 9} {
		e := contract.Event{SchemaVersion: 1, EventID: fmt.Sprint(i + 1), RunID: "run", Sequence: i + 1, SimTimeMs: simTime, EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{}}
		if _, err := Append(ctx, db, e, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := ReadTimeRange(ctx, db, "run", 5, 5, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Event.Sequence != 2 {
		t.Fatalf("first page: %+v", rows)
	}
	rows, err = ReadTimeRange(ctx, db, "run", 5, 5, 2, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Event.Sequence != 3 {
		t.Fatalf("second page: %+v", rows)
	}
	if _, err := ReadTimeRange(ctx, db, "run", 9, 5, 0, 1); err == nil {
		t.Fatal("reversed range accepted")
	}
	if _, err := ReadTimeRange(ctx, db, "missing", 0, 10, 0, 1); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("unknown run: %v", err)
	}
}

func TestLoadHistoricalIncludesAllEventsAtTarget(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "run", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	for i, simTime := range []int64{0, 5, 5, 9} {
		e := contract.Event{SchemaVersion: 1, EventID: fmt.Sprint(i + 1), RunID: "run", Sequence: i + 1, SimTimeMs: simTime, EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{}}
		if _, err := Append(ctx, db, e, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	source, err := LoadHistorical(ctx, db, "run", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(source.Events) != 3 || source.Events[2].Event.Sequence != 3 || len(source.Stations) != 1 || source.Stations[0] != "loader" || source.Integrity != "uncertain" {
		t.Fatalf("historical source: %+v", source)
	}
	if _, err := LoadHistorical(ctx, db, "missing", 5); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("missing run: %v", err)
	}
}
