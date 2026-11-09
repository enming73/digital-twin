package replay

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"smt-digital-twin/backend/internal/contract"
	"smt-digital-twin/backend/internal/twin"
	"time"
)

// Import builds a private store and returns it only after a complete run.
func Import(input io.Reader, epoch, runID string, stations []string) (*twin.Store, error) {
	store := twin.NewStore(epoch, map[string][]string{runID: stations})
	scan := bufio.NewScanner(input)
	scan.Buffer(make([]byte, 4096), 1<<20)
	count := 0
	for scan.Scan() {
		raw := scan.Bytes()
		var envelope struct {
			RunID     string  `json:"runId"`
			MachineID *string `json:"machineId"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, err
		}
		if envelope.RunID != runID {
			return nil, errors.New("run ID mismatch")
		}
		topic := "demo/smt/" + runID + "/line/events"
		if envelope.MachineID != nil {
			topic = "demo/smt/" + runID + "/machines/" + *envelope.MachineID + "/events"
		}
		event, err := contract.DecodeAndValidate(topic, raw)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", count+1, err)
		}
		result, err := store.Apply(event, time.Now())
		if err != nil || result.Duplicate {
			return nil, fmt.Errorf("line %d: duplicate or invalid event: %v", count+1, err)
		}
		count++
	}
	if err := scan.Err(); err != nil {
		return nil, err
	}
	snapshot, _ := store.Snapshot(runID)
	if count == 0 || len(snapshot.WorkOrders) == 0 {
		return nil, errors.New("empty run")
	}
	for _, order := range snapshot.WorkOrders {
		if order.Status != "completed" {
			return nil, errors.New("incomplete run")
		}
	}
	return store, nil
}
