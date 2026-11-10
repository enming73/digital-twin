package persistence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"time"

	"github.com/jackc/pgx/v5"
	"smt-digital-twin/backend/internal/contract"
	"smt-digital-twin/backend/internal/twin"
)

// loadCheckpoint fails back to the event log for missing, incompatible or
// corrupt checkpoint data; database errors themselves are not ignored.
func loadCheckpoint(ctx context.Context, tx pgx.Tx, runID string, stations []string, epoch string, last int, maxSimTime int64) (*twin.Store, int, error) {
	var raw []byte
	var sequence, format, reducer int
	var simTime int64
	err := tx.QueryRow(ctx, `SELECT sequence,sim_time_ms,format_version,reducer_version,checkpoint FROM snapshots WHERE run_id=$1 AND sequence <= $2 AND sim_time_ms <= $3 ORDER BY sequence DESC LIMIT 1`, runID, last, maxSimTime).Scan(&sequence, &simTime, &format, &reducer, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	if format != 1 || reducer != 1 {
		return nil, 0, nil
	}
	var envelope struct {
		Payload json.RawMessage `json:"payload"`
		SHA256  string          `json:"sha256"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, 0, nil
	}
	var checkpoint twin.Checkpoint
	if err := json.Unmarshal(envelope.Payload, &checkpoint); err != nil {
		return nil, 0, nil
	}
	canonical, err := json.Marshal(checkpoint)
	if err != nil {
		return nil, 0, nil
	}
	sum := sha256.Sum256(canonical)
	if hex.EncodeToString(sum[:]) != envelope.SHA256 || checkpoint.Snapshot.RunID != runID || checkpoint.Snapshot.LastSequence != sequence || checkpoint.Snapshot.SimTimeMs != simTime || !reflect.DeepEqual(checkpoint.Stations, stations) {
		return nil, 0, nil
	}
	store, err := twin.RestoreCheckpoint(checkpoint, epoch)
	if err != nil {
		return nil, 0, nil
	}
	if err := verifyCheckpointPrefix(ctx, tx, runID, sequence, &checkpoint); err != nil {
		if errors.Is(err, errCheckpointMismatch) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	return store, sequence, nil
}

var errCheckpointMismatch = errors.New("checkpoint does not match stored prefix")

type traceReference struct {
	boardID string
	entry   twin.TraceEntry
}

// verifyCheckpointPrefix validates the checkpoint's event index, traces and
// reduced projections against the committed log in one ordered scan. A
// mismatch is reported as errCheckpointMismatch so callers fall back to replay.
func verifyCheckpointPrefix(ctx context.Context, tx pgx.Tx, runID string, sequence int, checkpoint *twin.Checkpoint) error {
	entries := make(map[int]traceReference)
	for boardID, trace := range checkpoint.Traces {
		for _, entry := range trace {
			if _, exists := entries[entry.Sequence]; exists {
				return errCheckpointMismatch
			}
			entries[entry.Sequence] = traceReference{boardID: boardID, entry: entry}
		}
	}
	rows, err := tx.Query(ctx, `SELECT sequence,event_id,schema_version,sim_time_ms,emitted_at,ingested_at,envelope FROM events WHERE run_id=$1 AND sequence <= $2 ORDER BY sequence`, runID, sequence)
	if err != nil {
		return err
	}
	defer rows.Close()
	expected := 1
	var previousTime int64
	var lastIngested time.Time
	traceCount := 0
	for rows.Next() {
		var seq, version int
		var id string
		var eventSimTime int64
		var emittedAt, ingestedAt time.Time
		var body []byte
		if err := rows.Scan(&seq, &id, &version, &eventSimTime, &emittedAt, &ingestedAt, &body); err != nil {
			return err
		}
		if seq != expected || version != 1 || checkpoint.Sequences[seq] != id {
			return errCheckpointMismatch
		}
		var event contract.Event
		if err := json.Unmarshal(body, &event); err != nil {
			return errCheckpointMismatch
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			return errCheckpointMismatch
		}
		sum := sha256.Sum256(encoded)
		if checkpoint.IDs[event.EventID] != hex.EncodeToString(sum[:]) || event.RunID != runID || event.Sequence != seq || event.SchemaVersion != version || event.SimTimeMs != eventSimTime || event.SimTimeMs < previousTime || event.SimTimeMs < 0 {
			return errCheckpointMismatch
		}
		if !timesClose(event.EmittedAt, emittedAt) {
			return errCheckpointMismatch
		}
		previousTime = event.SimTimeMs
		lastIngested = ingestedAt
		if boardID, ok := event.Payload["boardId"].(string); ok {
			ref, ok := entries[seq]
			if !ok || ref.boardID != boardID || ref.entry.EventID != id || ref.entry.EventType != event.EventType || ref.entry.SimTimeMs != event.SimTimeMs || ref.entry.EmittedAt != event.EmittedAt || !ref.entry.IngestedAt.Equal(ingestedAt) || !sameMachine(ref.entry.MachineID, event.MachineID) {
				return errCheckpointMismatch
			}
			passed, hasPassed := event.Payload["passed"].(bool)
			if (hasPassed && (ref.entry.Passed == nil || *ref.entry.Passed != passed)) || (!hasPassed && ref.entry.Passed != nil) {
				return errCheckpointMismatch
			}
			traceCount++
		}
		expected++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if expected != sequence+1 || traceCount != len(entries) {
		return errCheckpointMismatch
	}
	if sequence > 0 && !checkpoint.Snapshot.LastIngestedAt.Equal(lastIngested) {
		return errCheckpointMismatch
	}
	if err := verifyProjections(ctx, tx, runID, sequence, checkpoint); err != nil {
		return err
	}
	return nil
}

func timesClose(emitted string, stored time.Time) bool {
	parsed, err := time.Parse(time.RFC3339Nano, emitted)
	if err != nil {
		return false
	}
	delta := parsed.Sub(stored).Seconds()
	return math.Abs(delta) <= 0.0000015
}

func sameMachine(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// traceEntriesEqual compares persisted trace projections. Ingestion times are
// compared within one microsecond because PostgreSQL stores microseconds while
// legacy projection rows may retain nanoseconds from earlier ingestion.
func traceEntriesEqual(a, b []twin.TraceEntry) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x.Sequence != y.Sequence || x.EventID != y.EventID || x.EventType != y.EventType || x.SimTimeMs != y.SimTimeMs || x.EmittedAt != y.EmittedAt || !sameMachine(x.MachineID, y.MachineID) || !timeClose(x.IngestedAt, y.IngestedAt) {
			return false
		}
		if (x.Passed == nil) != (y.Passed == nil) || (x.Passed != nil && *x.Passed != *y.Passed) {
			return false
		}
	}
	return true
}

func timeClose(a, b time.Time) bool {
	if a.IsZero() || b.IsZero() {
		return a.IsZero() && b.IsZero()
	}
	delta := a.Sub(b).Seconds()
	return math.Abs(delta) <= 0.0000015
}

// verifyProjections compares persisted W5 projections with the checkpoint state.
func verifyProjections(ctx context.Context, tx pgx.Tx, runID string, sequence int, checkpoint *twin.Checkpoint) error {
	rows, err := tx.Query(ctx, `SELECT board_id,revision,projection FROM board_trace WHERE run_id=$1`, runID)
	if err != nil {
		return err
	}
	boards := 0
	for rows.Next() {
		var id string
		var revision int
		var raw []byte
		if err := rows.Scan(&id, &revision, &raw); err != nil {
			rows.Close()
			return err
		}
		var stored struct {
			Board   twin.Board        `json:"board"`
			Entries []twin.TraceEntry `json:"entries"`
		}
		if err := json.Unmarshal(raw, &stored); err != nil {
			rows.Close()
			return errCheckpointMismatch
		}
		board, ok := checkpoint.Snapshot.Boards[id]
		if !ok || revision > sequence || !reflect.DeepEqual(board, stored.Board) || !traceEntriesEqual(checkpoint.Traces[id], stored.Entries) {
			rows.Close()
			return errCheckpointMismatch
		}
		boards++
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if boards != len(checkpoint.Snapshot.Boards) {
		return errCheckpointMismatch
	}
	orders, err := tx.Query(ctx, `SELECT work_order_id,revision,projection FROM work_order_projection WHERE run_id=$1`, runID)
	if err != nil {
		return err
	}
	defer orders.Close()
	ordersRead := 0
	for orders.Next() {
		var id string
		var revision int
		var raw []byte
		if err := orders.Scan(&id, &revision, &raw); err != nil {
			return err
		}
		var stored struct {
			Order  twin.WorkOrder        `json:"order"`
			Boards map[string]twin.Board `json:"boards"`
		}
		if err := json.Unmarshal(raw, &stored); err != nil {
			return errCheckpointMismatch
		}
		order, ok := checkpoint.Snapshot.WorkOrders[id]
		if !ok || revision > sequence || !reflect.DeepEqual(order, stored.Order) {
			return errCheckpointMismatch
		}
		for boardID, board := range stored.Boards {
			current, exists := checkpoint.Snapshot.Boards[boardID]
			if !exists || !reflect.DeepEqual(current, board) {
				return errCheckpointMismatch
			}
		}
		ordersRead++
	}
	if err := orders.Err(); err != nil {
		return err
	}
	if ordersRead != len(checkpoint.Snapshot.WorkOrders) {
		return errCheckpointMismatch
	}
	return nil
}
