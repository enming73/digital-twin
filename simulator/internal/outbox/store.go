// Package outbox stores committed simulator boundaries before publication.
package outbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"modernc.org/sqlite"
)

const DefaultMaxBytes int64 = 256 << 20

// schemaReserve is fixed space the database may use for schema, indexes and
// page slack beyond the retained-data quota. WAL growth is separately limited.
const schemaReserve int64 = 1 << 20
const outboxPageSize int64 = 4096

// capacityLimits derives a hard SQLite database bound and a WAL bound from the
// retained-data quota so the on-disk footprint cannot grow without limit.
func capacityLimits(maxBytes int64) (dbLimit, walLimit int64) {
	walLimit = maxBytes / 8
	if walLimit < 64<<10 {
		walLimit = 64 << 10
	}
	return maxBytes + schemaReserve, walLimit
}

var ErrCapacity = errors.New("outbox capacity exceeded")
var ErrStorageUnavailable = errors.New("outbox storage unavailable")
var ErrNoRun = errors.New("run not found in outbox")
var ErrGenerationMismatch = errors.New("source generation mismatch")
var ErrAckOrder = errors.New("acknowledgement would move backwards")
var ErrAckOutOfRange = errors.New("acknowledgement beyond durable head")

type Terminal struct {
	RunID         string `json:"runId"`
	FinalSequence int64  `json:"finalSequence"`
	EventCount    int64  `json:"eventCount"`
	SHA256        string `json:"sha256"`
}
type Page struct {
	Events   [][]byte  `json:"events"`
	Head     int64     `json:"head"`
	Terminal *Terminal `json:"terminal,omitempty"`
}
type Store struct {
	db       *sql.DB
	maxBytes int64
	path     string
	walLimit int64
}

func Open(path string, maxBytes int64) (*Store, error) {
	if path == "" || maxBytes < 0 {
		return nil, errors.New("invalid outbox configuration")
	}
	if maxBytes == 0 {
		maxBytes = DefaultMaxBytes
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*Store, error) {
		_ = db.Close()
		return nil, fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		return fail(err)
	}
	if _, err := db.Exec(`PRAGMA synchronous=FULL`); err != nil {
		return fail(err)
	}
	dbLimit, walLimit := capacityLimits(maxBytes)
	if _, err := db.Exec(fmt.Sprintf(`PRAGMA max_page_count=%d`, dbLimit/outboxPageSize)); err != nil {
		return fail(err)
	}
	if _, err := db.Exec(fmt.Sprintf(`PRAGMA journal_size_limit=%d`, walLimit)); err != nil {
		return fail(err)
	}
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fail(err)
	}
	if version > 2 {
		return fail(fmt.Errorf("unsupported outbox version %d", version))
	}
	if version < 2 {
		tx, err := db.Begin()
		if err != nil {
			return fail(err)
		}
		if version == 0 {
			_, err = tx.Exec(`CREATE TABLE runs (run_id TEXT PRIMARY KEY, head INTEGER NOT NULL, boundary_start INTEGER NOT NULL, state BLOB NOT NULL, terminal BLOB); CREATE TABLE events (run_id TEXT NOT NULL, sequence INTEGER NOT NULL, event_id TEXT NOT NULL, envelope BLOB NOT NULL, PRIMARY KEY(run_id, sequence), UNIQUE(run_id, event_id));`)
		}
		if err == nil {
			_, err = tx.Exec(`CREATE TABLE meta (run_id TEXT PRIMARY KEY, generation TEXT NOT NULL, scenario_digest TEXT NOT NULL, acknowledged INTEGER NOT NULL DEFAULT 0); PRAGMA user_version=2`)
		}
		if err != nil {
			_ = tx.Rollback()
			return fail(err)
		}
		if err := tx.Commit(); err != nil {
			return fail(err)
		}
	}
	return &Store{db: db, maxBytes: maxBytes, path: path, walLimit: walLimit}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Digest returns the SHA-256 of the ordered envelopes, each prefixed with its
// 8-byte big-endian length so that boundaries between events are significant.
func Digest(events [][]byte) string {
	h := sha256.New()
	var length [8]byte
	for _, raw := range events {
		binary.BigEndian.PutUint64(length[:], uint64(len(raw)))
		_, _ = h.Write(length[:])
		_, _ = h.Write(raw)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ReadState returns the last committed engine checkpoint and its sequence.
func (s *Store) ReadState(ctx context.Context, runID string) ([]byte, int64, error) {
	if runID == "" {
		return nil, 0, errors.New("invalid run ID")
	}
	var state []byte
	var head int64
	if err := s.db.QueryRowContext(ctx, `SELECT state,head FROM runs WHERE run_id=?`, runID).Scan(&state, &head); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, ErrNoRun
		}
		return nil, 0, fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	return state, head, nil
}

func (s *Store) CommitBoundary(ctx context.Context, runID string, events [][]byte, state []byte, terminal *Terminal) error {
	if runID == "" || len(events) == 0 || len(state) == 0 {
		return errors.New("invalid boundary")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	defer tx.Rollback()
	var head, boundaryStart int64
	var savedState, savedTerminal []byte
	err = tx.QueryRowContext(ctx, `SELECT head,boundary_start,state,terminal FROM runs WHERE run_id=?`, runID).Scan(&head, &boundaryStart, &savedState, &savedTerminal)
	if errors.Is(err, sql.ErrNoRows) {
		head = 0
	} else if err != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	var first struct {
		Sequence int64 `json:"sequence"`
	}
	if err := json.Unmarshal(events[0], &first); err != nil {
		return errors.New("invalid event")
	}
	if first.Sequence <= head {
		if first.Sequence < 1 || first.Sequence != boundaryStart || first.Sequence+int64(len(events))-1 != head || !bytes.Equal(state, savedState) {
			return errors.New("conflicting boundary retry")
		}
		encodedTerminal, err := json.Marshal(terminal)
		if err != nil {
			return err
		}
		if terminal == nil {
			encodedTerminal = nil
		}
		if !bytes.Equal(encodedTerminal, savedTerminal) {
			var stored Terminal
			if terminal == nil || json.Unmarshal(savedTerminal, &stored) != nil {
				return errors.New("conflicting boundary terminal")
			}
			if stored.RunID != terminal.RunID || stored.FinalSequence != terminal.FinalSequence || stored.EventCount != terminal.EventCount {
				return errors.New("conflicting boundary terminal")
			}
			if terminal.SHA256 != "" && terminal.SHA256 != stored.SHA256 {
				return errors.New("conflicting boundary terminal")
			}
		}
		for i, raw := range events {
			var stored []byte
			if err := tx.QueryRowContext(ctx, `SELECT envelope FROM events WHERE run_id=? AND sequence=?`, runID, first.Sequence+int64(i)).Scan(&stored); err != nil {
				return fmt.Errorf("%w: reading committed boundary: %v", ErrStorageUnavailable, err)
			}
			if !bytes.Equal(raw, stored) {
				return errors.New("conflicting boundary event")
			}
		}
		return nil
	}
	if len(savedTerminal) > 0 {
		return errors.New("run already completed")
	}
	var used int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT SUM(length(envelope)+length(event_id)+length(run_id)+32) FROM events),0)+COALESCE((SELECT SUM(length(state)+length(run_id)+COALESCE(length(terminal),0)+32) FROM runs),0)`).Scan(&used); err != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	var previousState int64
	if head > 0 {
		if err := tx.QueryRowContext(ctx, `SELECT length(state)+COALESCE(length(terminal),0) FROM runs WHERE run_id=?`, runID).Scan(&previousState); err != nil {
			return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
		}
	}
	used -= previousState
	used += int64(len(state) + len(runID) + 32)
	var terminalBytes []byte
	if terminal != nil {
		terminalBytes, err = json.Marshal(terminal)
		if err != nil {
			return err
		}
		used += int64(len(terminalBytes))
	}
	for _, raw := range events {
		var e struct {
			RunID    string `json:"runId"`
			EventID  string `json:"eventId"`
			Sequence int64  `json:"sequence"`
		}
		if err := json.Unmarshal(raw, &e); err != nil || e.RunID != runID || e.EventID == "" || e.Sequence != head+1 {
			return errors.New("invalid or non-contiguous event")
		}
		used += int64(len(raw) + len(e.EventID) + len(runID) + 32)
		if used > s.maxBytes {
			return ErrCapacity
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO events(run_id,sequence,event_id,envelope) VALUES(?,?,?,?)`, runID, e.Sequence, e.EventID, raw); err != nil {
			var sqliteErr *sqlite.Error
			if errors.As(err, &sqliteErr) && sqliteErr.Code()&0xff == 19 { // SQLITE_CONSTRAINT
				return fmt.Errorf("outbox event conflict: %w", err)
			}
			return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
		}
		head = e.Sequence
	}
	if terminal != nil && (terminal.RunID != runID || terminal.FinalSequence != head || terminal.EventCount != head) {
		return errors.New("invalid terminal boundary")
	}
	if terminal != nil {
		rows, err := tx.QueryContext(ctx, `SELECT envelope FROM events WHERE run_id=? ORDER BY sequence`, runID)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
		}
		var envelopes [][]byte
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				rows.Close()
				return err
			}
			envelopes = append(envelopes, raw)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		digest := Digest(envelopes)
		if terminal.SHA256 == "" {
			terminal.SHA256 = digest
			if terminalBytes, err = json.Marshal(terminal); err != nil {
				return err
			}
		} else if terminal.SHA256 != digest {
			return errors.New("terminal digest mismatch")
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO runs(run_id,head,boundary_start,state,terminal) VALUES(?,?,?,?,?) ON CONFLICT(run_id) DO UPDATE SET head=excluded.head,boundary_start=excluded.boundary_start,state=excluded.state,terminal=excluded.terminal`, runID, head, first.Sequence, state, terminalBytes); err != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	// The batch is durable at this point. Truncating the WAL keeps the physical
	// footprint inside the documented bound; a failed truncation cannot undo the
	// commit and SQLite recycles frames until the next successful checkpoint.
	if s.walNeedsTruncate() {
		_, _ = s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	}
	return nil
}

func (s *Store) walNeedsTruncate() bool {
	info, err := os.Stat(s.path + "-wal")
	if err != nil {
		return false
	}
	return info.Size() > s.walLimit/2
}

func (s *Store) ReadPage(ctx context.Context, runID string, after int64, limit int) (Page, error) {
	if runID == "" || after < 0 || limit < 1 || limit > 500 {
		return Page{}, errors.New("invalid page bounds")
	}
	var page Page
	var terminal []byte
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Page{}, fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `SELECT head,terminal FROM runs WHERE run_id=?`, runID).Scan(&page.Head, &terminal)
	if errors.Is(err, sql.ErrNoRows) {
		return Page{Events: [][]byte{}}, nil
	}
	if err != nil {
		return Page{}, fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	if len(terminal) > 0 {
		var value Terminal
		if err := json.Unmarshal(terminal, &value); err != nil {
			return Page{}, err
		}
		page.Terminal = &value
	}
	rows, err := tx.QueryContext(ctx, `SELECT envelope FROM events WHERE run_id=? AND sequence>? ORDER BY sequence LIMIT ?`, runID, after, limit)
	if err != nil {
		return Page{}, fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	defer rows.Close()
	page.Events = [][]byte{}
	for rows.Next() {
		var event []byte
		if err := rows.Scan(&event); err != nil {
			return Page{}, err
		}
		page.Events = append(page.Events, event)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	return page, nil
}
