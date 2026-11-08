package runner

import (
	"context"
	"reflect"
	"testing"
	"time"

	"smt-digital-twin/simulator/internal/contract"
	"smt-digital-twin/simulator/internal/engine"
	"smt-digital-twin/simulator/internal/scenario"
)

type collectingSink struct{ events []contract.Event }

func (s *collectingSink) Publish(_ context.Context, e contract.Event) error {
	s.events = append(s.events, e)
	return nil
}

func TestSpeedDoesNotChangeDomainEvents(t *testing.T) {
	s := scenario.Scenario{Quantity: 3, Stations: []scenario.Station{{ID: "loader", ProcessTimeMs: 2, InputCapacity: 1}, {ID: "unloader", ProcessTimeMs: 3, InputCapacity: 1}}}
	run := func(speed float64) []contract.Event {
		e, err := engine.New(s, 123, "same")
		if err != nil {
			t.Fatal(err)
		}
		sink := &collectingSink{}
		if err := Run(context.Background(), e, sink, speed); err != nil {
			t.Fatal(err)
		}
		return sink.events
	}
	if !reflect.DeepEqual(run(1), run(1000)) {
		t.Fatal("speed changed domain events")
	}
}

type channelSink struct{ events chan contract.Event }

type drainingSink struct {
	entered   chan struct{}
	release   chan struct{}
	delivered chan bool
}

func (s *drainingSink) Publish(ctx context.Context, _ contract.Event) error {
	close(s.entered)
	select {
	case <-ctx.Done():
		s.delivered <- false
		return ctx.Err()
	case <-s.release:
		s.delivered <- true
		return nil
	}
}
func (s *drainingSink) Close(context.Context) error { return nil }
func TestCancellationDrainsInFlightEvent(t *testing.T) {
	e, err := engine.New(scenario.Scenario{Quantity: 1, Stations: []scenario.Station{{ID: "loader", ProcessTimeMs: 10, InputCapacity: 1}}}, 1, "run")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	sink := &drainingSink{entered: make(chan struct{}), release: make(chan struct{}), delivered: make(chan bool, 1)}
	done := make(chan error, 1)
	go func() { done <- Run(ctx, e, sink, 1) }()
	<-sink.entered
	cancel()
	time.Sleep(10 * time.Millisecond)
	close(sink.release)
	if !<-sink.delivered {
		t.Fatal("in-flight event was canceled before drain")
	}
	if err := <-done; err == nil {
		t.Fatal("expected canceled run")
	}
}

func (s *channelSink) Publish(_ context.Context, e contract.Event) error { s.events <- e; return nil }
func (s *channelSink) Close(context.Context) error                       { return nil }
func TestPauseFreezesBoundaryAndResumeContinues(t *testing.T) {
	e, err := engine.New(scenario.Scenario{Quantity: 1, Stations: []scenario.Station{{ID: "loader", ProcessTimeMs: 300, InputCapacity: 1}}}, 1, "run")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pause := make(chan struct{}, 1)
	sink := &channelSink{events: make(chan contract.Event, 32)}
	done := make(chan error, 1)
	go func() { done <- RunControlled(ctx, e, sink, 1, pause) }()
	for {
		event := <-sink.events
		if event.EventType == "machine.state.changed" {
			break
		}
	}
	pause <- struct{}{}
	time.Sleep(450 * time.Millisecond)
	select {
	case event := <-sink.events:
		t.Fatalf("event while paused: %s", event.EventType)
	default:
	}
	pause <- struct{}{}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}
func (s *collectingSink) Close(context.Context) error { return nil }
func TestRunnerEmitsWorkOrder(t *testing.T) {
	e, err := engine.New(scenario.Scenario{Quantity: 1, Stations: []scenario.Station{{ID: "loader", ProcessTimeMs: 1, InputCapacity: 1}}}, 1, "run")
	if err != nil {
		t.Fatal(err)
	}
	sink := &collectingSink{}
	if err := Run(context.Background(), e, sink, 1000); err != nil {
		t.Fatal(err)
	}
	if len(sink.events) < 3 || sink.events[0].EventType != "workorder.started" || sink.events[len(sink.events)-1].EventType != "workorder.completed" {
		t.Fatalf("events: %+v", sink.events)
	}
}
