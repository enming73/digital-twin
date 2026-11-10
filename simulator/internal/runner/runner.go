package runner

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	"smt-digital-twin/simulator/internal/contract"
	"smt-digital-twin/simulator/internal/engine"
	"smt-digital-twin/simulator/internal/outbox"
	"smt-digital-twin/simulator/internal/publisher"
)

// Run paces virtual boundaries against wall time; the engine itself never reads wall time.
func Run(ctx context.Context, e *engine.Engine, sink publisher.Sink, speed float64) error {
	return RunControlled(ctx, e, sink, speed, nil)
}

// RunControlled toggles pause for each notification. Pause freezes pacing and event emission.
func RunControlled(ctx context.Context, e *engine.Engine, sink publisher.Sink, speed float64, toggle <-chan struct{}) error {
	if speed <= 0 || math.IsNaN(speed) || math.IsInf(speed, 0) {
		return errors.New("invalid speed")
	}
	var previous int64
	paused := false
	for {
		now, err := e.NextTime()
		if err != nil {
			return err
		}
		delay := time.Duration(float64(now-previous) * float64(time.Millisecond) / speed)
		previous = now
		for paused || delay > 0 {
			if paused {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case _, ok := <-toggle:
					if !ok {
						toggle = nil
					} else {
						paused = false
					}
				}
				continue
			}
			started := time.Now()
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case _, ok := <-toggle:
				timer.Stop()
				delay -= time.Since(started)
				if delay < 0 {
					delay = 0
				}
				if !ok {
					toggle = nil
				} else {
					paused = true
				}
			case <-timer.C:
				delay = 0
			}
		}
		events, done, err := e.Next()
		if err != nil {
			return err
		}
		for _, event := range events {
			if err := publishWithDrain(ctx, sink, event); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if done {
			return nil
		}
	}
}

// BoundaryStore persists a whole virtual boundary before it may be published.
type BoundaryStore interface {
	CommitBoundary(ctx context.Context, runID string, events [][]byte, state []byte, terminal *outbox.Terminal) error
}

// DurableStatus reports why generation is paused, if it is.
type DurableStatus struct {
	Paused bool
	Reason string
}

var durableRetryDelay = func(attempt int) time.Duration {
	delay := 250 * time.Millisecond
	for i := 0; i < attempt && delay < 5*time.Second; i++ {
		delay *= 2
	}
	if delay > 5*time.Second {
		delay = 5 * time.Second
	}
	return delay
}

// RunDurable commits each virtual boundary, including the post-boundary engine
// state, before publishing it. Capacity or storage failures restore the last
// committed checkpoint, report a pause and retry without discarding a batch.
func RunDurable(ctx context.Context, e *engine.Engine, store BoundaryStore, sink publisher.Sink, speed float64, toggle <-chan struct{}) error {
	return RunDurableWithStatus(ctx, e, store, sink, speed, toggle, nil)
}

// RunDurableWithStatus is RunDurable with pause/availability reporting.
func RunDurableWithStatus(ctx context.Context, e *engine.Engine, store BoundaryStore, sink publisher.Sink, speed float64, toggle <-chan struct{}, report func(DurableStatus)) error {
	if speed <= 0 || math.IsNaN(speed) || math.IsInf(speed, 0) {
		return errors.New("invalid speed")
	}
	var previous int64
	paused := false
	attempt := 0
	for {
		now, err := e.NextTime()
		if err != nil {
			return err
		}
		delay := time.Duration(float64(now-previous) * float64(time.Millisecond) / speed)
		previous = now
		for paused || delay > 0 {
			if paused {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case _, ok := <-toggle:
					if !ok {
						toggle = nil
					} else {
						paused = false
					}
				}
				continue
			}
			started := time.Now()
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case _, ok := <-toggle:
				timer.Stop()
				delay -= time.Since(started)
				if delay < 0 {
					delay = 0
				}
				if !ok {
					toggle = nil
				} else {
					paused = true
				}
			case <-timer.C:
				delay = 0
			}
		}
		pre, err := e.ExportState()
		if err != nil {
			return err
		}
		events, done, err := e.Next()
		if err != nil {
			if restoreErr := e.ImportState(pre); restoreErr != nil {
				return restoreErr
			}
			return err
		}
		if done && len(events) == 0 {
			return nil
		}
		raws := make([][]byte, 0, len(events))
		for i := range events {
			if events[i].EmittedAt == "" {
				events[i].EmittedAt = time.Now().UTC().Format(time.RFC3339Nano)
			}
			raw, err := json.Marshal(events[i])
			if err != nil {
				return err
			}
			raws = append(raws, raw)
		}
		post, err := e.ExportState()
		if err != nil {
			return err
		}
		var terminal *outbox.Terminal
		if done {
			last := events[len(events)-1]
			terminal = &outbox.Terminal{RunID: e.RunID(), FinalSequence: int64(last.Sequence), EventCount: int64(last.Sequence)}
		}
		if err := store.CommitBoundary(ctx, e.RunID(), raws, post, terminal); err != nil {
			if errors.Is(err, outbox.ErrCapacity) || errors.Is(err, outbox.ErrStorageUnavailable) {
				if restoreErr := e.ImportState(pre); restoreErr != nil {
					return restoreErr
				}
				attempt++
				if report != nil {
					report(DurableStatus{Paused: true, Reason: err.Error()})
				}
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(durableRetryDelay(attempt)):
				}
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				continue
			}
			return err
		}
		attempt = 0
		if report != nil {
			report(DurableStatus{})
		}
		for _, event := range events {
			if err := publishWithDrain(ctx, sink, event); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if done {
			return nil
		}
	}
}

func publishWithDrain(ctx context.Context, sink publisher.Sink, event contract.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	publication, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			timer := time.NewTimer(3 * time.Second)
			defer timer.Stop()
			select {
			case <-timer.C:
				cancel()
			case <-done:
			}
		case <-done:
		}
	}()
	err := sink.Publish(publication, event)
	close(done)
	cancel()
	return err
}
