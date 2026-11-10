package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"smt-digital-twin/backend/internal/contract"
	"smt-digital-twin/backend/internal/twin"
)

// Journal is the PostgreSQL-backed writer used by the optional DATABASE_URL
// mode. It is not used for the default in-memory demo.
type Journal struct {
	db    *pgxpool.Pool
	lease *WriterLease
}

func NewJournal(db *pgxpool.Pool) *Journal { return &Journal{db: db} }

// NewLeasedJournal executes all writes on the lock-owning database session.
func NewLeasedJournal(lease *WriterLease) *Journal { return &Journal{lease: lease} }

func encodeCheckpoint(checkpoint twin.Checkpoint) (json.RawMessage, error) {
	body, err := json.Marshal(checkpoint)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(body)
	return json.Marshal(struct {
		Payload json.RawMessage `json:"payload"`
		SHA256  string          `json:"sha256"`
	}{body, hex.EncodeToString(sum[:])})
}

func (j *Journal) SaveCheckpoint(checkpoint twin.Checkpoint) error {
	data, err := encodeCheckpoint(checkpoint)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var db transactionBeginner = j.db
	if j.lease != nil {
		j.lease.mu.Lock()
		defer j.lease.mu.Unlock()
		if j.lease.conn == nil || j.lease.runID != checkpoint.Snapshot.RunID {
			return errors.New("writer lease unavailable")
		}
		db = j.lease.conn
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var last int
	if err := tx.QueryRow(ctx, `SELECT last_sequence FROM runs WHERE run_id=$1 FOR UPDATE`, checkpoint.Snapshot.RunID).Scan(&last); err != nil {
		return err
	}
	if last != checkpoint.Snapshot.LastSequence {
		return errors.New("checkpoint cursor does not match committed log")
	}
	// Checkpoints are derived data; replacing an unverifiable row is safe because
	// last_sequence was locked and the canonical event log remains authoritative.
	if _, err := tx.Exec(ctx, `INSERT INTO snapshots(run_id,sequence,sim_time_ms,format_version,reducer_version,checkpoint) VALUES($1,$2,$3,1,1,$4::jsonb) ON CONFLICT(run_id,sequence) DO UPDATE SET sim_time_ms=EXCLUDED.sim_time_ms,format_version=EXCLUDED.format_version,reducer_version=EXCLUDED.reducer_version,checkpoint=EXCLUDED.checkpoint WHERE snapshots.checkpoint IS DISTINCT FROM EXCLUDED.checkpoint`, checkpoint.Snapshot.RunID, last, checkpoint.Snapshot.SimTimeMs, []byte(data)); err != nil {
		return err
	}
	if err := pruneCheckpoints(ctx, tx, checkpoint.Snapshot.RunID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (j *Journal) SetIntegrity(runID, integrity, reason string) error {
	if integrity != "gap" && integrity != "conflict" && integrity != "uncertain" && integrity != "complete" {
		return fmt.Errorf("invalid integrity %q", integrity)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var result pgconn.CommandTag
	var err error
	query := `UPDATE runs SET integrity=$2, reason=$3 WHERE run_id=$1 AND (integrity <> 'conflict' OR integrity=$2)`
	if j.lease != nil {
		j.lease.mu.Lock()
		defer j.lease.mu.Unlock()
		if j.lease.conn == nil || j.lease.runID != runID {
			return fmt.Errorf("writer lease unavailable for %q", runID)
		}
		result, err = j.lease.conn.Exec(ctx, query, runID, integrity, reason)
	} else {
		result, err = j.db.Exec(ctx, query, runID, integrity, reason)
	}
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("unknown or frozen run %q", runID)
	}
	return nil
}

func (j *Journal) Commit(event contract.Event, at time.Time, candidate twin.Snapshot, trace []twin.TraceEntry, checkpoint *twin.Checkpoint) error {
	return j.commit(event, at, candidate, trace, checkpoint, false)
}

// CommitRecovered allows a verified source event to extend a run frozen on gap.
func (j *Journal) CommitRecovered(event contract.Event, at time.Time, candidate twin.Snapshot, trace []twin.TraceEntry, checkpoint *twin.Checkpoint) error {
	return j.commit(event, at, candidate, trace, checkpoint, true)
}

func (j *Journal) commit(event contract.Event, at time.Time, candidate twin.Snapshot, trace []twin.TraceEntry, checkpoint *twin.Checkpoint, allowGap bool) error {
	p := Projection{BoardTraces: map[string]json.RawMessage{}, WorkOrders: map[string]json.RawMessage{}}
	if checkpoint != nil {
		if checkpoint.Snapshot.RunID != event.RunID || checkpoint.Snapshot.LastSequence != event.Sequence {
			return fmt.Errorf("checkpoint does not match event")
		}
		var err error
		p.Checkpoint, err = encodeCheckpoint(*checkpoint)
		if err != nil {
			return err
		}
	}
	if id, ok := event.Payload["boardId"].(string); ok {
		board, exists := candidate.Boards[id]
		if !exists {
			return fmt.Errorf("candidate missing board %q", id)
		}
		value, err := json.Marshal(struct {
			Board   twin.Board        `json:"board"`
			Entries []twin.TraceEntry `json:"entries"`
		}{board, trace})
		if err != nil {
			return err
		}
		p.BoardTraces[id] = value
	}
	// Work-order totals change only on order lifecycle events and terminal board
	// exits; rewriting the board map on every PCB event amplified writes.
	if id, ok := event.Payload["workOrderId"].(string); ok && (event.EventType == "workorder.started" || event.EventType == "workorder.completed" || event.EventType == "pcb.exited") {
		order, exists := candidate.WorkOrders[id]
		if !exists {
			return fmt.Errorf("candidate missing work order %q", id)
		}
		boards := make(map[string]twin.Board)
		for boardID, board := range candidate.Boards {
			if board.WorkOrderID == id {
				boards[boardID] = board
			}
		}
		value, err := json.Marshal(struct {
			Order  twin.WorkOrder        `json:"order"`
			Boards map[string]twin.Board `json:"boards"`
		}{order, boards})
		if err != nil {
			return err
		}
		p.WorkOrders[id] = value
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var result AppendResult
	var err error
	if j.lease != nil {
		j.lease.mu.Lock()
		defer j.lease.mu.Unlock()
		if j.lease.conn == nil || j.lease.runID != event.RunID {
			return fmt.Errorf("writer lease unavailable for %q", event.RunID)
		}
		result, err = appendProjection(ctx, j.lease.conn, event, at, p, allowGap)
	} else {
		result, err = appendProjection(ctx, j.db, event, at, p, allowGap)
	}
	if err != nil {
		return err
	}
	if result.Duplicate {
		return fmt.Errorf("database already contains event %s; restore required", event.EventID)
	}
	return nil
}
