package persistence

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"smt-digital-twin/backend/internal/twin"
)

type HistoricalSource struct {
	Stations   []string
	Integrity  string
	Reason     string
	Events     []StoredEvent
	Checkpoint *twin.Store
}

// LoadHistorical reads a consistent event prefix, including all events at the
// requested simulated timestamp. It does not infer coverage beyond the log.
func LoadHistorical(ctx context.Context, db *pgxpool.Pool, runID string, simTimeMs int64) (HistoricalSource, error) {
	if simTimeMs < 0 {
		return HistoricalSource{}, errors.New("negative simulated time")
	}
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return HistoricalSource{}, err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	var last int
	var out HistoricalSource
	err = tx.QueryRow(ctx, `SELECT stations,integrity,reason,last_sequence FROM runs WHERE run_id=$1`, runID).Scan(&raw, &out.Integrity, &out.Reason, &last)
	if errors.Is(err, pgx.ErrNoRows) {
		return HistoricalSource{}, ErrRunNotFound
	}
	if err != nil {
		return HistoricalSource{}, err
	}
	if err := json.Unmarshal(raw, &out.Stations); err != nil {
		return HistoricalSource{}, err
	}
	if out.Integrity != "complete" && out.Integrity != "uncertain" && out.Integrity != "gap" && out.Integrity != "conflict" {
		return HistoricalSource{}, errors.New("unknown stored integrity")
	}
	out.Events = make([]StoredEvent, 0)
	checkpoint, sequence, err := loadCheckpoint(ctx, tx, runID, out.Stations, "historical", last, simTimeMs)
	if err != nil {
		return HistoricalSource{}, err
	}
	out.Checkpoint = checkpoint
	var lastTime int64
	if checkpoint != nil {
		snapshot, _ := checkpoint.Snapshot(runID)
		lastTime = snapshot.SimTimeMs
	}
	for sequence < last {
		rows, err := readEvents(ctx, tx, runID, sequence, 1000)
		if err != nil {
			return HistoricalSource{}, err
		}
		if len(rows) == 0 {
			return HistoricalSource{}, errors.New("incomplete committed prefix")
		}
		for _, row := range rows {
			if row.Event.Sequence != sequence+1 || row.Event.Sequence > last || row.Event.SimTimeMs < 0 || (sequence > 0 && row.Event.SimTimeMs < lastTime) {
				return HistoricalSource{}, errors.New("invalid committed prefix")
			}
			if row.Event.SimTimeMs > simTimeMs {
				if err := tx.Commit(ctx); err != nil {
					return HistoricalSource{}, err
				}
				return out, nil
			}
			sequence++
			lastTime = row.Event.SimTimeMs
			out.Events = append(out.Events, row)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return HistoricalSource{}, err
	}
	return out, nil
}
