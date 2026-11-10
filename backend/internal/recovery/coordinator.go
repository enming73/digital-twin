// Package recovery reconciles the backend projection with the durable source
// log after lost or delayed MQTT notifications. MQTT is a best-effort wake-up
// path; only the verified source terminal boundary can establish completeness.
package recovery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"smt-digital-twin/backend/internal/contract"
	"smt-digital-twin/backend/internal/twin"
)

type Terminal struct {
	RunID         string `json:"runId"`
	FinalSequence int64  `json:"finalSequence"`
	EventCount    int64  `json:"eventCount"`
	SHA256        string `json:"sha256"`
}

type Manifest struct {
	ProtocolVersion int       `json:"protocolVersion"`
	RunID           string    `json:"runId"`
	Generation      string    `json:"sourceGenerationId"`
	ScenarioDigest  string    `json:"scenarioDigest"`
	Head            int64     `json:"head"`
	Terminal        *Terminal `json:"terminal"`
}

type Page struct {
	Events   []json.RawMessage `json:"events"`
	Head     int64             `json:"head"`
	Terminal *Terminal         `json:"terminal"`
}

// Source is the read-only, versioned durable log served by the simulator.
type Source interface {
	Manifest(ctx context.Context, runID string) (Manifest, error)
	ReadPage(ctx context.Context, runID string, after int64, limit int) (Page, error)
	Ack(ctx context.Context, runID, generation string, sequence int64) error
}

// Store is the subset of twin.Store needed for verified catch-up.
type Store interface {
	Snapshot(runID string) (twin.Snapshot, bool)
	SequenceFingerprint(runID string, sequence int) (string, bool)
	Apply(event contract.Event, at time.Time) (twin.ApplyResult, error)
	ApplyRecovered(event contract.Event, at time.Time) (twin.ApplyResult, error)
	BeginRecovery(runID string) error
	EndRecovery(runID string)
	PromoteComplete(runID string, finalSequence int) error
	RecoveryFailed(reason string)
}

var ErrIncomplete = errors.New("source run is not terminal")
var ErrIdentity = errors.New("source identity does not match the backend run")
var ErrProtocol = errors.New("unsupported source protocol")
var ErrOrder = errors.New("source page is not contiguous with the committed prefix")

type Config struct {
	RunID          string
	Generation     string
	ScenarioDigest string
	RetryDelay     func(attempt int) time.Duration
}

type Coordinator struct {
	store  Store
	source Source
	config Config
	mu     sync.Mutex
}

func New(store Store, source Source, config Config) *Coordinator {
	if config.RetryDelay == nil {
		config.RetryDelay = func(attempt int) time.Duration {
			delay := 250 * time.Millisecond
			for i := 0; i < attempt && delay < 5*time.Second; i++ {
				delay *= 2
			}
			if delay > 5*time.Second {
				delay = 5 * time.Second
			}
			return delay
		}
	}
	return &Coordinator{store: store, source: source, config: config}
}

// Recover catches the projection up to the source head, verifies the committed
// prefix and the terminal boundary, and only then promotes completeness.
// Concurrent calls serialize.
func (c *Coordinator) Recover(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	snapshot, ok := c.store.Snapshot(c.config.RunID)
	if !ok {
		return errors.New("unknown run")
	}
	if snapshot.Integrity == "conflict" {
		return errors.New("run is frozen in conflict")
	}
	manifest, err := c.source.Manifest(ctx, c.config.RunID)
	if err != nil {
		return c.fail(err)
	}
	if manifest.ProtocolVersion != 1 {
		return c.fail(fmt.Errorf("%w: version %d", ErrProtocol, manifest.ProtocolVersion))
	}
	if manifest.RunID != c.config.RunID {
		return c.fail(fmt.Errorf("%w: run %q", ErrIdentity, manifest.RunID))
	}
	if c.config.Generation != "" && manifest.Generation != c.config.Generation {
		return c.fail(fmt.Errorf("%w: generation %q", ErrIdentity, manifest.Generation))
	}
	if c.config.ScenarioDigest != "" && manifest.ScenarioDigest != c.config.ScenarioDigest {
		return c.fail(fmt.Errorf("%w: scenario digest", ErrIdentity))
	}
	if manifest.Head < int64(snapshot.LastSequence) {
		return c.fail(fmt.Errorf("%w: source head %d behind committed %d", ErrIdentity, manifest.Head, snapshot.LastSequence))
	}

	if err := c.store.BeginRecovery(c.config.RunID); err != nil {
		return err
	}
	defer c.store.EndRecovery(c.config.RunID)

	// Verify the already-committed prefix before any tail event is applied, so a
	// different/re-created source cannot pollute the projection first.
	envelopes, err := c.verifiedPrefix(ctx, snapshot.LastSequence)
	if err != nil {
		return c.fail(err)
	}

	head := snapshot.LastSequence
	for int64(head) < manifest.Head {
		page, err := c.source.ReadPage(ctx, c.config.RunID, int64(head), 500)
		if err != nil {
			return c.fail(fmt.Errorf("source page after %d: %w", head, err))
		}
		if len(page.Events) == 0 {
			return c.fail(fmt.Errorf("%w: empty page before head %d", ErrOrder, manifest.Head))
		}
		for _, raw := range page.Events {
			event, err := decodeEnvelope(raw)
			if err != nil {
				return c.fail(err)
			}
			if event.Sequence != head+1 {
				return c.fail(fmt.Errorf("%w: got sequence %d after %d", ErrOrder, event.Sequence, head))
			}
			if _, err := c.store.ApplyRecovered(event, time.Now()); err != nil {
				return c.fail(fmt.Errorf("apply recovered event %d: %w", event.Sequence, err))
			}
			envelopes = append(envelopes, []byte(raw))
			head = event.Sequence
		}
	}

	if manifest.Terminal == nil {
		return ErrIncomplete
	}
	terminal := *manifest.Terminal
	if err := c.verifyTerminal(manifest, terminal, envelopes); err != nil {
		return c.fail(err)
	}
	if snapshot.Integrity != "complete" || !snapshot.TerminalVerified || snapshot.LastSequence != int(terminal.FinalSequence) {
		if err := c.store.PromoteComplete(c.config.RunID, int(terminal.FinalSequence)); err != nil {
			return err
		}
	}
	if err := c.source.Ack(ctx, c.config.RunID, manifest.Generation, terminal.FinalSequence); err != nil {
		return fmt.Errorf("committed but not acknowledged: %w", err)
	}
	return nil
}

func (c *Coordinator) fail(err error) error {
	c.store.RecoveryFailed(err.Error())
	return err
}

// verifiedPrefix re-reads and fingerprints the committed prefix. PostgreSQL
// JSONB is not byte-preserving, so comparisons use re-encoded fingerprints.
func (c *Coordinator) verifiedPrefix(ctx context.Context, committed int) ([][]byte, error) {
	var envelopes [][]byte
	after := int64(0)
	for after < int64(committed) {
		limit := 500
		if remaining := int64(committed) - after; remaining < 500 {
			limit = int(remaining)
		}
		page, err := c.source.ReadPage(ctx, c.config.RunID, after, limit)
		if err != nil {
			return nil, fmt.Errorf("verify page after %d: %w", after, err)
		}
		if len(page.Events) == 0 {
			return nil, fmt.Errorf("%w: verification page empty at %d", ErrOrder, after)
		}
		for _, raw := range page.Events {
			event, err := decodeEnvelope(raw)
			if err != nil {
				return nil, err
			}
			if event.Sequence != int(after)+1 {
				return nil, fmt.Errorf("%w: verification order at %d", ErrOrder, event.Sequence)
			}
			if event.Sequence > committed {
				break
			}
			fingerprint, ok := c.store.SequenceFingerprint(c.config.RunID, event.Sequence)
			if !ok {
				return nil, fmt.Errorf("%w: backend is missing committed sequence %d", ErrIdentity, event.Sequence)
			}
			if fingerprint != envelopeFingerprint(event) {
				return nil, fmt.Errorf("%w: source event %d differs from the committed prefix", ErrIdentity, event.Sequence)
			}
			envelopes = append(envelopes, []byte(raw))
			after = int64(event.Sequence)
		}
	}
	return envelopes, nil
}

func (c *Coordinator) verifyTerminal(manifest Manifest, terminal Terminal, envelopes [][]byte) error {
	if terminal.RunID != c.config.RunID || terminal.FinalSequence != manifest.Head || terminal.EventCount != terminal.FinalSequence {
		return fmt.Errorf("%w: inconsistent terminal record", ErrIdentity)
	}
	if int64(len(envelopes)) != manifest.Head {
		return fmt.Errorf("%w: collected %d of %d envelopes", ErrOrder, len(envelopes), manifest.Head)
	}
	if digest(envelopes) != terminal.SHA256 {
		return fmt.Errorf("%w: terminal digest mismatch", ErrIdentity)
	}
	var last contract.Event
	if err := json.Unmarshal(envelopes[len(envelopes)-1], &last); err != nil {
		return fmt.Errorf("%w: final envelope: %v", ErrIdentity, err)
	}
	if last.EventType != "workorder.completed" {
		return fmt.Errorf("%w: final event is %q", ErrIdentity, last.EventType)
	}
	return nil
}

// Run retries recovery with bounded backoff until cancellation or a verified
// terminal boundary. A verified complete run needs no further polling.
func (c *Coordinator) Run(ctx context.Context) error {
	attempt := 0
	for {
		if snapshot, ok := c.store.Snapshot(c.config.RunID); ok && snapshot.TerminalVerified && snapshot.Integrity == "complete" {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(30 * time.Second):
			}
			continue
		}
		err := c.Recover(ctx)
		if err == nil {
			attempt = 0
		} else {
			attempt++
			log.Printf("recovery attempt for %s failed: %v", c.config.RunID, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.config.RetryDelay(attempt)):
		}
	}
}

func (c *Coordinator) verify(ctx context.Context, manifest Manifest, terminal Terminal, committed int) error {
	if terminal.RunID != c.config.RunID || terminal.FinalSequence != manifest.Head || terminal.EventCount != terminal.FinalSequence {
		return fmt.Errorf("%w: inconsistent terminal record", ErrIdentity)
	}
	var envelopes [][]byte
	var last contract.Event
	after := int64(0)
	for after < manifest.Head {
		page, err := c.source.ReadPage(ctx, c.config.RunID, after, 500)
		if err != nil {
			return fmt.Errorf("verify page after %d: %w", after, err)
		}
		if len(page.Events) == 0 {
			return fmt.Errorf("%w: verification page empty at %d", ErrOrder, after)
		}
		for _, raw := range page.Events {
			event, err := decodeEnvelope(raw)
			if err != nil {
				return err
			}
			if event.Sequence != int(after)+1 {
				return fmt.Errorf("%w: verification order at %d", ErrOrder, event.Sequence)
			}
			if event.Sequence <= committed {
				fingerprint, ok := c.store.SequenceFingerprint(c.config.RunID, event.Sequence)
				if !ok {
					return fmt.Errorf("%w: backend is missing committed sequence %d", ErrIdentity, event.Sequence)
				}
				if fingerprint != envelopeFingerprint(event) {
					return fmt.Errorf("%w: source event %d differs from the committed prefix", ErrIdentity, event.Sequence)
				}
			}
			envelopes = append(envelopes, []byte(raw))
			after = int64(event.Sequence)
			last = event
		}
	}
	if digest(envelopes) != terminal.SHA256 {
		return fmt.Errorf("%w: terminal digest mismatch", ErrIdentity)
	}
	if last.EventType != "workorder.completed" {
		return fmt.Errorf("%w: final event is %q", ErrIdentity, last.EventType)
	}
	return nil
}

func decodeEnvelope(raw []byte) (contract.Event, error) {
	var event contract.Event
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return contract.Event{}, fmt.Errorf("malformed source envelope: %w", err)
	}
	if event.SchemaVersion != 1 || event.EventID == "" || event.RunID == "" || event.Sequence < 1 || event.Source != "simulator" || event.EmittedAt == "" {
		return contract.Event{}, errors.New("invalid source envelope")
	}
	switch event.EventType {
	case "workorder.started", "workorder.completed", "machine.state.changed", "machine.alarm.raised", "machine.alarm.cleared",
		"pcb.entered", "pcb.exited", "pcb.process.completed", "pcb.inspection.completed":
	default:
		return contract.Event{}, fmt.Errorf("unsupported source event type %q", event.EventType)
	}
	return event, nil
}

func envelopeFingerprint(event contract.Event) string {
	data, err := json.Marshal(event)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func digest(envelopes [][]byte) string {
	hash := sha256.New()
	var length [8]byte
	for _, raw := range envelopes {
		binary.BigEndian.PutUint64(length[:], uint64(len(raw)))
		hash.Write(length[:])
		hash.Write(raw)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
