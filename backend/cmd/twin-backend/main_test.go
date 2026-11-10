package main

import (
	"context"
	"testing"
)

func TestStationsRejectInvalidOrDuplicateIDs(t *testing.T) {
	for _, value := range []string{"", "a,a", "a,", "a/b", "a,+"} {
		if _, err := parseStations(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	got, err := parseStations("loader,printer")
	if err != nil || len(got) != 2 || got[1] != "printer" {
		t.Fatalf("%v %v", got, err)
	}
}

func TestShutdownWaitsForConsumerBeforeCheckpoint(t *testing.T) {
	done := make(chan struct{})
	saved := make(chan struct{}, 1)
	result := make(chan error, 1)
	go func() {
		result <- finishShutdown(context.Background(), done, func() error { saved <- struct{}{}; return nil })
	}()
	select {
	case <-saved:
		t.Fatal("saved before consumer stopped")
	default:
	}
	close(done)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	select {
	case <-saved:
	default:
		t.Fatal("checkpoint not saved")
	}
}

func TestShutdownTimeoutDoesNotCheckpoint(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	saved := false
	if err := finishShutdown(ctx, make(chan struct{}), func() error { saved = true; return nil }); err == nil {
		t.Fatal("timeout accepted")
	}
	if saved {
		t.Fatal("checkpoint while consumer active")
	}
}
