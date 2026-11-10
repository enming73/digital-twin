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

type StoredEvent struct {
	Event      contract.Event `json:"event"`
	IngestedAt time.Time      `json:"ingestedAt"`
}

var ErrRunNotFound = errors.New("run not found")
var ErrUnsupportedVersion = errors.New("unsupported event schema version")

// ReadTimeRange returns a bounded inclusive simulated-time range in sequence order.
func ReadTimeRange(ctx context.Context, db *pgxpool.Pool, runID string, from, to int64, afterSequence, limit int) ([]StoredEvent, error) {
	if from < 0 || to < from || afterSequence < 0 || limit < 1 || limit > 1000 {
		return nil, errors.New("invalid event range")
	}
	var exists bool
	if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runs WHERE run_id=$1)`, runID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrRunNotFound
	}
	rows, err := db.Query(ctx, `SELECT envelope,ingested_at,sequence,event_id,schema_version,sim_time_ms FROM events WHERE run_id=$1 AND sim_time_ms >= $2 AND sim_time_ms <= $3 AND sequence > $4 ORDER BY sequence LIMIT $5`, runID, from, to, afterSequence, limit)
	if err != nil {
		return nil, err
	}
	return scanStoredEvents(rows, runID)
}

// ReadEvents returns at most 1000 events strictly after sequence in run order.
func ReadEvents(ctx context.Context, db *pgxpool.Pool, runID string, afterSequence, limit int) ([]StoredEvent, error) {
	return readEvents(ctx, db, runID, afterSequence, limit)
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readEvents(ctx context.Context, db queryer, runID string, afterSequence, limit int) ([]StoredEvent, error) {
	if afterSequence < 0 || limit < 1 || limit > 1000 {
		return nil, errors.New("invalid event range")
	}
	rows, err := db.Query(ctx, `SELECT envelope,ingested_at,sequence,event_id,schema_version,sim_time_ms FROM events WHERE run_id=$1 AND sequence>$2 ORDER BY sequence LIMIT $3`, runID, afterSequence, limit)
	if err != nil {
		return nil, err
	}
	return scanStoredEvents(rows, runID)
}

func scanStoredEvents(rows pgx.Rows, runID string) ([]StoredEvent, error) {
	defer rows.Close()
	result := make([]StoredEvent, 0)
	for rows.Next() {
		var body []byte
		var event StoredEvent
		var sequence, version int
		var id string
		var simTime int64
		if err := rows.Scan(&body, &event.IngestedAt, &sequence, &id, &version, &simTime); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(body, &event.Event); err != nil {
			return nil, err
		}
		if event.Event.SchemaVersion != 1 {
			return nil, fmt.Errorf("%w: %d", ErrUnsupportedVersion, event.Event.SchemaVersion)
		}
		if event.Event.RunID != runID || event.Event.Sequence != sequence || event.Event.EventID != id || event.Event.SchemaVersion != version || event.Event.SimTimeMs != simTime {
			return nil, fmt.Errorf("stored event columns disagree with envelope at sequence %d", sequence)
		}
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
