package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"smt-digital-twin/simulator/internal/scenario"
)

// checkpointVersion identifies the persisted engine-state format. Unknown
// versions fail closed rather than being silently reinterpreted.
const checkpointVersion = 1

type checkpointState struct {
	Version        int      `json:"version"`
	RunID          string   `json:"runId"`
	Seed           uint32   `json:"seed"`
	ScenarioDigest string   `json:"scenarioDigest"`
	Sequence       int      `json:"sequence"`
	Snapshot       Snapshot `json:"snapshot"`
	Started        bool     `json:"started"`
	Random         uint32   `json:"random"`
	FaultIndex     int      `json:"faultIndex"`
	Failed         []string `json:"failed"`
}

type checkpointEnvelope struct {
	Checkpoint json.RawMessage `json:"checkpoint"`
	SHA256     string          `json:"sha256"`
}

// ScenarioDigest is the canonical scenario identity persisted in checkpoints
// and advertised by the source manifest.
func ScenarioDigest(s scenario.Scenario) (string, error) {
	return scenarioDigest(s)
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func scenarioDigest(s scenario.Scenario) (string, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return digestBytes(data), nil
}

func cloneSnapshot(s Snapshot) Snapshot {
	out := s
	out.Stations = append([]Station(nil), s.Stations...)
	for i := range out.Stations {
		out.Stations[i].Queue = append([]string(nil), s.Stations[i].Queue...)
	}
	return out
}

// ExportState serializes every mutable engine field plus the identity needed to
// reject a checkpoint from another run, seed or scenario.
func (e *Engine) ExportState() ([]byte, error) {
	digest, err := scenarioDigest(e.config)
	if err != nil {
		return nil, err
	}
	failed := make([]string, 0, len(e.failed))
	for id := range e.failed {
		failed = append(failed, id)
	}
	sort.Strings(failed)
	payload, err := json.Marshal(checkpointState{
		Version:        checkpointVersion,
		RunID:          e.runID,
		Seed:           e.seed,
		ScenarioDigest: digest,
		Sequence:       e.sequence,
		Snapshot:       cloneSnapshot(e.snapshot),
		Started:        e.started,
		Random:         e.random,
		FaultIndex:     e.faultIndex,
		Failed:         failed,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(checkpointEnvelope{Checkpoint: payload, SHA256: digestBytes(payload)})
}

// RestoreState rebuilds an engine from a checkpoint produced by ExportState.
func RestoreState(config scenario.Scenario, seed uint32, runID string, data []byte) (*Engine, error) {
	e, err := New(config, seed, runID)
	if err != nil {
		return nil, err
	}
	if err := e.ImportState(data); err != nil {
		return nil, err
	}
	return e, nil
}

// ImportState replaces the mutable state of an existing engine after verifying
// that the checkpoint belongs to it and describes a reachable simulation state.
func (e *Engine) ImportState(data []byte) error {
	var envelope checkpointEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil || len(envelope.Checkpoint) == 0 {
		return errors.New("invalid checkpoint envelope")
	}
	if digestBytes(envelope.Checkpoint) != envelope.SHA256 {
		return errors.New("checkpoint checksum mismatch")
	}
	var state checkpointState
	decoder := json.NewDecoder(bytes.NewReader(envelope.Checkpoint))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return fmt.Errorf("invalid checkpoint state: %w", err)
	}
	if state.Version != checkpointVersion {
		return fmt.Errorf("unsupported checkpoint version %d", state.Version)
	}
	if state.RunID != e.runID || state.Seed != e.seed {
		return errors.New("checkpoint identity mismatch")
	}
	digest, err := scenarioDigest(e.config)
	if err != nil {
		return err
	}
	if state.ScenarioDigest != digest {
		return errors.New("checkpoint scenario mismatch")
	}
	if state.Sequence < 0 || state.FaultIndex < 0 || state.FaultIndex > len(e.config.Faults) {
		return errors.New("invalid checkpoint counters")
	}
	if !state.Started && state.Sequence != 0 {
		return errors.New("checkpoint has events before start")
	}
	snapshot := state.Snapshot
	if snapshot.SimTimeMs < 0 || snapshot.Created < 0 || snapshot.Completed < 0 ||
		snapshot.Completed > snapshot.Created || snapshot.Created > e.config.Quantity {
		return errors.New("invalid checkpoint progress")
	}
	if len(snapshot.Stations) != len(e.config.Stations) {
		return errors.New("checkpoint topology mismatch")
	}
	for i := range snapshot.Stations {
		station := snapshot.Stations[i]
		if !reflect.DeepEqual(station.Station, e.config.Stations[i]) {
			return fmt.Errorf("checkpoint station %d does not match scenario", i)
		}
		if len(station.Queue) > station.InputCapacity {
			return fmt.Errorf("checkpoint station %d queue exceeds capacity", i)
		}
		if station.BoardID != "" {
			if n, ok := boardNumber(station.BoardID); !ok || n < 1 || n > snapshot.Created {
				return fmt.Errorf("checkpoint station %d holds unknown board", i)
			}
		}
		for _, id := range station.Queue {
			if n, ok := boardNumber(id); !ok || n < 1 || n > snapshot.Created {
				return fmt.Errorf("checkpoint station %d queues unknown board", i)
			}
		}
	}
	failed := make(map[string]bool, len(state.Failed))
	for _, id := range state.Failed {
		n, ok := boardNumber(id)
		if !ok || n < 1 || n > snapshot.Created || failed[id] {
			return errors.New("checkpoint references unknown failed board")
		}
		failed[id] = true
	}
	e.sequence = state.Sequence
	e.snapshot = cloneSnapshot(snapshot)
	e.started = state.Started
	e.random = state.Random
	e.faultIndex = state.FaultIndex
	e.failed = failed
	return nil
}

func boardNumber(id string) (int, bool) {
	if !strings.HasPrefix(id, "board-") {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(id, "board-"))
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}
