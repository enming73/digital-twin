package publisher

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"smt-digital-twin/simulator/internal/contract"
)

type attempt struct {
	topic string
	body  string
}
type fakeTransport struct {
	mu       sync.Mutex
	attempts []attempt
	failures int
}

type blockingTransport struct {
	entered chan struct{}
	release chan struct{}
}

func (b *blockingTransport) send(ctx context.Context, _ string, _ []byte) error {
	close(b.entered)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.release:
		return nil
	}
}
func (b *blockingTransport) close() {}
func TestMQTTBackpressureHonorsCancellation(t *testing.T) {
	b := &blockingTransport{entered: make(chan struct{}), release: make(chan struct{})}
	sink := newMQTT(b, time.Millisecond, time.Millisecond)
	e := contract.Event{SchemaVersion: 1, EventID: "event-1", RunID: "run", Source: "simulator", EventType: "workorder.started", Sequence: 1, Payload: map[string]any{"synthetic": true, "workOrderId": "work-order-1", "quantity": 1}}
	first := make(chan error, 1)
	go func() { first <- sink.Publish(context.Background(), e) }()
	<-b.entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := sink.Publish(ctx, e); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected backpressure timeout: %v", err)
	}
	close(b.release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
}

// The first send simulates a broker accepting a message but losing its PUBACK.
func TestLostAcknowledgementRetriesSameEventIdentity(t *testing.T) {
	f := &fakeTransport{failures: 1}
	sink := newMQTT(f, time.Millisecond, time.Millisecond)
	e := contract.Event{SchemaVersion: 1, EventID: "event-1", RunID: "run", Source: "simulator", EventType: "workorder.started", Sequence: 1, Payload: map[string]any{"synthetic": true, "workOrderId": "work-order-1", "quantity": 1}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := sink.Publish(ctx, e); err != nil {
		t.Fatal(err)
	}
	if len(f.attempts) != 2 || f.attempts[0] != f.attempts[1] {
		t.Fatalf("retry changed delivery: %+v", f.attempts)
	}
}

func (f *fakeTransport) send(ctx context.Context, topic string, body []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts = append(f.attempts, attempt{topic, string(body)})
	if f.failures > 0 {
		f.failures--
		return errors.New("offline")
	}
	return nil
}
func (f *fakeTransport) close() {}
func TestMQTTRetriesSameBytesAndTopic(t *testing.T) {
	f := &fakeTransport{failures: 2}
	sink := newMQTT(f, time.Millisecond, time.Millisecond)
	id := "mounter"
	event := contract.Event{SchemaVersion: 1, EventID: "event-00000001", RunID: "demo", Source: "simulator", MachineID: &id, EventType: "pcb.entered", Sequence: 1, Payload: map[string]any{"synthetic": true, "boardId": "board-1", "workOrderId": "work-order-1"}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := sink.Publish(ctx, event); err != nil {
		t.Fatal(err)
	}
	if len(f.attempts) != 3 {
		t.Fatalf("attempts: %d", len(f.attempts))
	}
	for _, a := range f.attempts {
		if a != f.attempts[0] {
			t.Fatal("retry changed bytes or topic")
		}
	}
	if f.attempts[0].topic != "demo/smt/demo/machines/mounter/events" {
		t.Fatal(f.attempts[0].topic)
	}
}
func TestMQTTStopsRetryOnCancellation(t *testing.T) {
	f := &fakeTransport{failures: 100}
	sink := newMQTT(f, time.Millisecond, time.Millisecond)
	e := contract.Event{SchemaVersion: 1, EventID: "event-1", RunID: "run", Source: "simulator", EventType: "workorder.started", Sequence: 1, Payload: map[string]any{"synthetic": true, "workOrderId": "work-order-1", "quantity": 1}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := sink.Publish(ctx, e); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error: %v", err)
	}
}
