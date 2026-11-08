package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"smt-digital-twin/simulator/internal/engine"
	"smt-digital-twin/simulator/internal/publisher"
	"smt-digital-twin/simulator/internal/runner"
	"smt-digital-twin/simulator/internal/scenario"
)

func run(ctx context.Context, args []string, out io.Writer) error {
	return runWithControl(ctx, args, out, nil)
}
func runWithControl(ctx context.Context, args []string, out io.Writer, pause <-chan struct{}) error {
	if len(args) == 0 || args[0] != "run" {
		return errors.New("usage: simulator run --scenario PATH [--seed N] [--speed N] [--mqtt-url URL] [--run-id ID] [--dry-run]")
	}
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	path := fs.String("scenario", "", "scenario JSON")
	seed := fs.Uint("seed", 2026, "seed")
	speed := fs.Float64("speed", 1, "speed")
	url := fs.String("mqtt-url", "tcp://127.0.0.1:1883", "MQTT broker")
	id := fs.String("run-id", "demo-run", "run ID")
	dry := fs.Bool("dry-run", false, "NDJSON output")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *path == "" || fs.NArg() != 0 || uint64(*seed) > uint64(^uint32(0)) {
		return errors.New("invalid scenario, seed or arguments")
	}
	s, err := scenario.Load(*path)
	if err != nil {
		return err
	}
	e, err := engine.New(s, uint32(*seed), *id)
	if err != nil {
		return err
	}
	var sink publisher.Sink
	if *dry {
		sink = publisher.NewNDJSON(out)
	} else {
		sink, err = publisher.NewMQTT(publisher.Config{URL: *url, ClientID: "simulator-" + *id})
		if err != nil {
			return err
		}
	}
	err = runner.RunControlled(ctx, e, sink, *speed, pause)
	closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	closeErr := sink.Close(closeCtx)
	if err != nil {
		return err
	}
	return closeErr
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGUSR1)
	defer signal.Stop(signals)
	pause := make(chan struct{}, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-signals:
				select {
				case pause <- struct{}{}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	if err := runWithControl(ctx, os.Args[1:], os.Stdout, pause); err != nil {
		fmt.Fprintln(os.Stderr, "SIMULATED simulator:", err)
		os.Exit(1)
	}
}
