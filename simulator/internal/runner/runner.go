package runner

import (
	"context"
	"errors"
	"math"
	"time"

	"smt-digital-twin/simulator/internal/contract"
	"smt-digital-twin/simulator/internal/engine"
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
