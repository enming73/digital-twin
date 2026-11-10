package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// SourceProtocolVersion is the versioned read/acknowledgement protocol served
// to the backend recovery coordinator. Bump it for any incompatible change.
const SourceProtocolVersion = 1

// Manifest identifies one source run so a backend can verify it is catching up
// from the right generation before it trusts any page.
type Manifest struct {
	ProtocolVersion int       `json:"protocolVersion"`
	RunID           string    `json:"runId"`
	Generation      string    `json:"sourceGenerationId"`
	ScenarioDigest  string    `json:"scenarioDigest"`
	Head            int64     `json:"head"`
	Terminal        *Terminal `json:"terminal"`
}

// RegisterRun binds a run to one source generation and scenario digest. The
// pair must stay stable across restarts; conflicting re-registration fails.
func (s *Store) RegisterRun(ctx context.Context, runID, generation, scenarioDigest string) error {
	if runID == "" || generation == "" || scenarioDigest == "" {
		return errors.New("invalid source registration")
	}
	var existingGeneration, existingDigest string
	err := s.db.QueryRowContext(ctx, `SELECT generation,scenario_digest FROM meta WHERE run_id=?`, runID).Scan(&existingGeneration, &existingDigest)
	switch {
	case err == nil:
		if existingGeneration != generation || existingDigest != scenarioDigest {
			return ErrGenerationMismatch
		}
		return nil
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO meta(run_id,generation,scenario_digest,acknowledged) VALUES(?,?,?,0)`, runID, generation, scenarioDigest); err != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	return nil
}

// Manifest reports the durable head and terminal boundary for a run.
func (s *Store) Manifest(ctx context.Context, runID string) (Manifest, error) {
	if runID == "" {
		return Manifest{}, errors.New("invalid run ID")
	}
	var manifest Manifest
	manifest.ProtocolVersion = SourceProtocolVersion
	manifest.RunID = runID
	var terminal []byte
	err := s.db.QueryRowContext(ctx, `SELECT r.head, r.terminal, COALESCE(m.generation,''), COALESCE(m.scenario_digest,'') FROM runs r LEFT JOIN meta m ON m.run_id=r.run_id WHERE r.run_id=?`, runID).Scan(&manifest.Head, &terminal, &manifest.Generation, &manifest.ScenarioDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return Manifest{}, ErrNoRun
	}
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	if len(terminal) > 0 {
		var value Terminal
		if err := json.Unmarshal(terminal, &value); err != nil {
			return Manifest{}, err
		}
		manifest.Terminal = &value
	}
	return manifest, nil
}

// Ack records the highest contiguous sequence the backend has committed. It
// never deletes events; this phase only persists the cursor.
func (s *Store) Ack(ctx context.Context, runID, generation string, sequence int64) (int64, error) {
	if runID == "" || generation == "" || sequence < 0 {
		return 0, errors.New("invalid acknowledgement")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	defer tx.Rollback()
	var head, acknowledged int64
	var storedGeneration string
	err = tx.QueryRowContext(ctx, `SELECT r.head, m.generation, m.acknowledged FROM runs r JOIN meta m ON m.run_id=r.run_id WHERE r.run_id=?`, runID).Scan(&head, &storedGeneration, &acknowledged)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNoRun
	}
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	if storedGeneration != generation {
		return 0, ErrGenerationMismatch
	}
	if sequence > head {
		return 0, ErrAckOutOfRange
	}
	if sequence < acknowledged {
		return 0, ErrAckOrder
	}
	if _, err := tx.ExecContext(ctx, `UPDATE meta SET acknowledged=? WHERE run_id=?`, sequence, runID); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	return sequence, nil
}

// NewSourceServer builds the bounded read-only source API used by recovery.
// An empty address binds loopback only.
func NewSourceServer(addr string, store *Store) *http.Server {
	if addr == "" {
		addr = "127.0.0.1:8081"
	}
	return &http.Server{
		Addr:              addr,
		Handler:           NewSourceHandler(store),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}
