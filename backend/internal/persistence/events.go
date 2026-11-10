package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"smt-digital-twin/backend/internal/contract"
)

type AppendResult struct{ Duplicate bool }

// Projection contains changed, already-validated W5 projections for this event.
type Projection struct {
	BoardTraces map[string]json.RawMessage
	WorkOrders  map[string]json.RawMessage
	Checkpoint  json.RawMessage
}

// CreateRun registers the immutable station ordering for a new run.
func CreateRun(ctx context.Context, db *pgxpool.Pool, runID string, stations []string) error {
	b, err := json.Marshal(stations)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `INSERT INTO runs (run_id,stations,integrity) VALUES ($1,$2,'uncertain')`, runID, b)
	return err
}

// EnsureRun creates a run once and refuses to change its station order.
func EnsureRun(ctx context.Context, db *pgxpool.Pool, runID string, stations []string) (bool, error) {
	if runID == "" || len(stations) == 0 {
		return false, errors.New("run and stations are required")
	}
	b, err := json.Marshal(stations)
	if err != nil {
		return false, err
	}
	result, err := db.Exec(ctx, `INSERT INTO runs (run_id,stations,integrity) VALUES ($1,$2,'uncertain') ON CONFLICT (run_id) DO NOTHING`, runID, b)
	if err != nil {
		return false, err
	}
	if result.RowsAffected() == 1 {
		return true, nil
	}
	var matches bool
	if err := db.QueryRow(ctx, `SELECT stations=$2::jsonb FROM runs WHERE run_id=$1`, runID, b).Scan(&matches); err != nil {
		return false, err
	}
	if !matches {
		return false, errors.New("stored topology mismatch")
	}
	return false, nil
}

// Append records a contiguous envelope without projections. It does not run the
// W5 reducer; Journal is the validated live-writer path.
func Append(ctx context.Context, db *pgxpool.Pool, event contract.Event, ingestedAt time.Time) (AppendResult, error) {
	return AppendProjection(ctx, db, event, ingestedAt, Projection{})
}

// AppendProjection atomically commits an event and its changed projections.
// Reducer validation remains the caller's responsibility.
func AppendProjection(ctx context.Context, db *pgxpool.Pool, event contract.Event, ingestedAt time.Time, projection Projection) (AppendResult, error) {
	return appendProjection(ctx, db, event, ingestedAt, projection, false)
}

// AppendProjectionRecovered extends a run previously frozen on gap; only the
// verified recovery coordinator may use it.
func AppendProjectionRecovered(ctx context.Context, db *pgxpool.Pool, event contract.Event, ingestedAt time.Time, projection Projection) (AppendResult, error) {
	return appendProjection(ctx, db, event, ingestedAt, projection, true)
}

type transactionBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

func appendProjection(ctx context.Context, db transactionBeginner, event contract.Event, ingestedAt time.Time, projection Projection, allowGap bool) (AppendResult, error) {
	if event.SchemaVersion != 1 {
		return AppendResult{}, fmt.Errorf("unsupported event schema version %d", event.SchemaVersion)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return AppendResult{}, err
	}
	emitted, err := time.Parse(time.RFC3339Nano, event.EmittedAt)
	if err != nil {
		return AppendResult{}, err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return AppendResult{}, err
	}
	defer tx.Rollback(ctx)
	var last int
	var integrity string
	if err := tx.QueryRow(ctx, `SELECT last_sequence, integrity FROM runs WHERE run_id=$1 FOR UPDATE`, event.RunID).Scan(&last, &integrity); err != nil {
		return AppendResult{}, err
	}
	freeze := func(status, reason string) (AppendResult, error) {
		if _, err := tx.Exec(ctx, `UPDATE runs SET integrity=$2,reason=$3 WHERE run_id=$1`, event.RunID, status, reason); err != nil {
			return AppendResult{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return AppendResult{}, err
		}
		return AppendResult{}, errors.New(reason)
	}
	var equal bool
	err = tx.QueryRow(ctx, `SELECT envelope = $3::jsonb FROM events WHERE run_id=$1 AND event_id=$2`, event.RunID, event.EventID, data).Scan(&equal)
	if err == nil {
		if equal {
			return AppendResult{Duplicate: true}, nil
		}
		return freeze("conflict", "event ID conflict")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return AppendResult{}, err
	}
	if integrity == "conflict" || (integrity == "gap" && !allowGap) {
		return AppendResult{}, errors.New("run frozen")
	}
	if event.Sequence <= last {
		return freeze("conflict", "sequence conflict")
	}
	if event.Sequence != last+1 {
		return freeze("gap", fmt.Sprintf("expected sequence %d, got %d", last+1, event.Sequence))
	}
	if last > 0 {
		var lastTime int64
		if err := tx.QueryRow(ctx, `SELECT sim_time_ms FROM events WHERE run_id=$1 AND sequence=$2`, event.RunID, last).Scan(&lastTime); err != nil {
			return AppendResult{}, err
		}
		if event.SimTimeMs < lastTime {
			return freeze("conflict", "simulated time regression")
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO events(run_id,event_id,sequence,schema_version,sim_time_ms,emitted_at,ingested_at,envelope) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, event.RunID, event.EventID, event.Sequence, event.SchemaVersion, event.SimTimeMs, emitted, ingestedAt.UTC(), data)
	if err != nil {
		return AppendResult{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE runs SET last_sequence=$2 WHERE run_id=$1`, event.RunID, event.Sequence); err != nil {
		return AppendResult{}, err
	}
	for id, value := range projection.BoardTraces {
		_, err = tx.Exec(ctx, `INSERT INTO board_trace (run_id,board_id,revision,projection) VALUES ($1,$2,$3,$4::jsonb) ON CONFLICT (run_id,board_id) DO UPDATE SET revision=EXCLUDED.revision,projection=EXCLUDED.projection`, event.RunID, id, event.Sequence, []byte(value))
		if err != nil {
			return AppendResult{}, err
		}
	}
	for id, value := range projection.WorkOrders {
		_, err = tx.Exec(ctx, `INSERT INTO work_order_projection (run_id,work_order_id,revision,projection) VALUES ($1,$2,$3,$4::jsonb) ON CONFLICT (run_id,work_order_id) DO UPDATE SET revision=EXCLUDED.revision,projection=EXCLUDED.projection`, event.RunID, id, event.Sequence, []byte(value))
		if err != nil {
			return AppendResult{}, err
		}
	}
	if projection.Checkpoint != nil {
		_, err = tx.Exec(ctx, `INSERT INTO snapshots(run_id,sequence,sim_time_ms,format_version,reducer_version,checkpoint) VALUES($1,$2,$3,1,1,$4::jsonb)`, event.RunID, event.Sequence, event.SimTimeMs, []byte(projection.Checkpoint))
		if err != nil {
			return AppendResult{}, err
		}
		if err := pruneCheckpoints(ctx, tx, event.RunID); err != nil {
			return AppendResult{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return AppendResult{}, err
	}
	return AppendResult{}, nil
}
