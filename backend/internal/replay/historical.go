package replay

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"smt-digital-twin/backend/internal/persistence"
	"smt-digital-twin/backend/internal/twin"
)

type HistoricalSnapshot struct {
	Mode               string        `json:"mode"`
	Synthetic          bool          `json:"synthetic"`
	RequestedSimTimeMs int64         `json:"requestedSimTimeMs"`
	Snapshot           twin.Snapshot `json:"snapshot"`
}

type HistoricalService struct{ DB *pgxpool.Pool }

func (s HistoricalService) Replay(ctx context.Context, runID string, simTimeMs int64) (HistoricalSnapshot, error) {
	source, err := persistence.LoadHistorical(ctx, s.DB, runID, simTimeMs)
	if err != nil {
		return HistoricalSnapshot{}, err
	}
	store := source.Checkpoint
	if store == nil {
		store, err = ReduceHistorical(source.Events, runID, source.Stations, simTimeMs)
		if err != nil {
			return HistoricalSnapshot{}, err
		}
	} else {
		for _, event := range source.Events {
			result, err := store.Apply(event.Event, event.IngestedAt)
			if err != nil {
				return HistoricalSnapshot{}, err
			}
			if result.Duplicate {
				return HistoricalSnapshot{}, errors.New("duplicate historical event")
			}
		}
	}
	snapshot, _ := store.Snapshot(runID)
	snapshot.Integrity, snapshot.Reason = source.Integrity, source.Reason
	return HistoricalSnapshot{Mode: "historical", Synthetic: true, RequestedSimTimeMs: simTimeMs, Snapshot: snapshot}, nil
}

func (s HistoricalService) Events(ctx context.Context, runID string, from, to int64, after, limit int) ([]persistence.StoredEvent, error) {
	return persistence.ReadTimeRange(ctx, s.DB, runID, from, to, after, limit)
}

// ReduceHistorical builds an isolated W5 projection from a complete ordered
// prefix. It never alters the live twin or claims completeness of the source.
func ReduceHistorical(events []persistence.StoredEvent, runID string, stations []string, simTimeMs int64) (*twin.Store, error) {
	if simTimeMs < 0 {
		return nil, errors.New("negative simulated time")
	}
	store := twin.NewStore("historical", map[string][]string{runID: stations})
	var lastTime int64
	for i, stored := range events {
		e := stored.Event
		if e.RunID != runID || e.SchemaVersion != 1 || e.Sequence != i+1 || e.SimTimeMs < 0 || (i > 0 && e.SimTimeMs < lastTime) {
			return nil, fmt.Errorf("invalid event prefix at index %d", i)
		}
		lastTime = e.SimTimeMs
		if e.SimTimeMs > simTimeMs {
			break
		}
		result, err := store.Apply(e, stored.IngestedAt)
		if err != nil {
			return nil, fmt.Errorf("sequence %d: %w", e.Sequence, err)
		}
		if result.Duplicate {
			return nil, fmt.Errorf("duplicate sequence %d", e.Sequence)
		}
	}
	store.MarkUncertain("historical source coverage is not established")
	return store, nil
}
