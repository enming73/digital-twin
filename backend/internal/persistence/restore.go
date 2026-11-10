package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"smt-digital-twin/backend/internal/twin"
)

const checkpointRetention = 3

// pruneCheckpoints bounds derived checkpoint storage; the event log is retained.
func pruneCheckpoints(ctx context.Context, tx pgx.Tx, runID string) error {
	_, err := tx.Exec(ctx, `DELETE FROM snapshots WHERE run_id=$1 AND sequence NOT IN (SELECT sequence FROM snapshots WHERE run_id=$1 ORDER BY sequence DESC LIMIT $2)`, runID, checkpointRetention)
	return err
}

// Restore reconstructs a private store from one consistent committed prefix.
// It must not be exposed until the caller has established a single-writer lock.
func Restore(ctx context.Context, db *pgxpool.Pool, runID string, stations []string, epoch string) (*twin.Store, error) {
	return restore(ctx, db, runID, stations, epoch, NewJournal(db))
}

// RestoreWithLease must be used by a live writer; it binds further writes to
// the same PostgreSQL session that owns the run lock.
func RestoreWithLease(ctx context.Context, db *pgxpool.Pool, runID string, stations []string, epoch string, lease *WriterLease) (*twin.Store, error) {
	if lease == nil {
		return nil, errors.New("writer lease required")
	}
	lease.mu.Lock()
	valid := lease.conn != nil && lease.runID == runID
	lease.mu.Unlock()
	if !valid {
		return nil, errors.New("writer lease does not match run")
	}
	return restore(ctx, db, runID, stations, epoch, NewLeasedJournal(lease))
}

func restore(ctx context.Context, db *pgxpool.Pool, runID string, stations []string, epoch string, journal twin.Journal) (*twin.Store, error) {
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var raw []byte
	var integrity, reason string
	var last int
	if err := tx.QueryRow(ctx, `SELECT stations,integrity,reason,last_sequence FROM runs WHERE run_id=$1`, runID).Scan(&raw, &integrity, &reason, &last); err != nil {
		return nil, err
	}
	var saved []string
	if err := json.Unmarshal(raw, &saved); err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(saved, stations) {
		return nil, errors.New("stored topology mismatch")
	}
	if integrity != "complete" && integrity != "uncertain" && integrity != "gap" && integrity != "conflict" {
		return nil, errors.New("unknown stored integrity")
	}
	s, sequence, err := loadCheckpoint(ctx, tx, runID, stations, epoch, last, math.MaxInt64)
	if err != nil {
		return nil, err
	}
	if s == nil {
		s = twin.NewStore(epoch, map[string][]string{runID: stations})
	}
	var lastSimTime int64
	if sequence > 0 {
		snapshot, _ := s.Snapshot(runID)
		lastSimTime = snapshot.SimTimeMs
	}
	for sequence < last {
		rows, err := readEvents(ctx, tx, runID, sequence, 1000)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return nil, errors.New("committed event prefix is incomplete")
		}
		for _, row := range rows {
			if row.Event.Sequence != sequence+1 || row.Event.Sequence > last {
				return nil, errors.New("invalid committed event sequence")
			}
			if row.Event.SimTimeMs < 0 || (sequence > 0 && row.Event.SimTimeMs < lastSimTime) {
				return nil, errors.New("invalid committed simulated time")
			}
			result, err := s.Apply(row.Event, row.IngestedAt)
			if err != nil || result.Duplicate {
				return nil, fmt.Errorf("restore sequence %d: %v", row.Event.Sequence, err)
			}
			sequence++
			lastSimTime = row.Event.SimTimeMs
		}
	}
	var extra bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM events WHERE run_id=$1 AND sequence>$2)`, runID, last).Scan(&extra); err != nil {
		return nil, err
	}
	if extra {
		return nil, errors.New("events exist beyond committed run cursor")
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if integrity != "gap" && integrity != "conflict" {
		if err := journal.SetIntegrity(runID, "uncertain", "source coverage after restart is unverified"); err != nil {
			return nil, err
		}
	}
	if err := s.FinishRestore(runID, integrity, reason, journal); err != nil {
		return nil, err
	}
	return s, nil
}
