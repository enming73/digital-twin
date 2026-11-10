package twin

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Checkpoint contains all state required to continue reducer validation.
type Checkpoint struct {
	Snapshot  Snapshot                `json:"snapshot"`
	Stations  []string                `json:"stations"`
	Traces    map[string][]TraceEntry `json:"traces"`
	IDs       map[string]string       `json:"ids"`
	Sequences map[int]string          `json:"sequences"`
}

func copyCheckpoint(c Checkpoint) (Checkpoint, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return Checkpoint{}, err
	}
	var result Checkpoint
	err = json.Unmarshal(b, &result)
	return result, err
}

func (s *Store) Checkpoint(runID string) (Checkpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runID]
	if !ok {
		return Checkpoint{}, errors.New("unknown run")
	}
	return copyCheckpoint(Checkpoint{Snapshot: r.snapshot, Stations: r.stations, Traces: r.traces, IDs: r.ids, Sequences: r.sequences})
}

// SaveCheckpoint serializes a committed run while preventing concurrent Apply.
// In-memory-only stores intentionally have no persistent checkpoint.
func (s *Store) SaveCheckpoint(runID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[runID]
	if !ok {
		return errors.New("unknown run")
	}
	if s.journal == nil || r.snapshot.LastSequence == 0 {
		return nil
	}
	writer, ok := s.journal.(interface{ SaveCheckpoint(Checkpoint) error })
	if !ok {
		return errors.New("journal does not support checkpoints")
	}
	checkpoint, err := copyCheckpoint(Checkpoint{Snapshot: r.snapshot, Stations: r.stations, Traces: r.traces, IDs: r.ids, Sequences: r.sequences})
	if err != nil {
		return err
	}
	if err := writer.SaveCheckpoint(checkpoint); err != nil {
		s.metrics.StorageErrors++
		return err
	}
	return nil
}

// RestoreCheckpoint reconstructs a private store and validates the event index.
// Its caller must authenticate the persisted checkpoint before using it.
func RestoreCheckpoint(input Checkpoint, epoch string) (*Store, error) {
	c, err := copyCheckpoint(input)
	if err != nil {
		return nil, err
	}
	snapshot := c.Snapshot
	if epoch == "" || snapshot.RunID == "" || snapshot.LastSequence < 0 || snapshot.Revision != snapshot.LastSequence || len(c.Stations) == 0 || len(c.IDs) != snapshot.LastSequence || len(c.Sequences) != snapshot.LastSequence {
		return nil, errors.New("invalid checkpoint metadata")
	}
	seen := map[string]bool{}
	for _, id := range c.Stations {
		if id == "" || seen[id] {
			return nil, errors.New("invalid checkpoint topology")
		}
		seen[id] = true
	}
	if c.Traces == nil || c.IDs == nil || c.Sequences == nil || snapshot.Boards == nil || snapshot.WorkOrders == nil || snapshot.Machines == nil {
		return nil, errors.New("checkpoint maps are missing")
	}
	for seq := 1; seq <= snapshot.LastSequence; seq++ {
		id, ok := c.Sequences[seq]
		if !ok || id == "" || c.IDs[id] == "" {
			return nil, fmt.Errorf("checkpoint index missing sequence %d", seq)
		}
	}
	if len(c.Sequences) != snapshot.LastSequence {
		return nil, errors.New("checkpoint sequence index is inconsistent")
	}
	s := NewStore(epoch, map[string][]string{snapshot.RunID: c.Stations})
	r := s.runs[snapshot.RunID]
	snapshot.Epoch = epoch
	if snapshot.Integrity != "gap" && snapshot.Integrity != "conflict" {
		snapshot.Integrity = "uncertain"
		snapshot.Reason = "source coverage after restart is unverified"
	}
	r.snapshot = snapshot
	r.traces = c.Traces
	r.ids = c.IDs
	r.sequences = c.Sequences
	r.canEstablish = false
	return s, nil
}
