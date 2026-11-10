package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"smt-digital-twin/simulator/internal/engine"
	"smt-digital-twin/simulator/internal/outbox"
	"smt-digital-twin/simulator/internal/publisher"
	"smt-digital-twin/simulator/internal/runner"
	"smt-digital-twin/simulator/internal/scenario"
)

// sourceGeneration derives a stable source identity from the run and canonical
// scenario digest, so a restart or backend recovery cannot mix generations.
func sourceGeneration(runID, scenarioDigest string) string {
	sum := sha256.Sum256([]byte("source-v1|" + runID + "|" + scenarioDigest))
	return hex.EncodeToString(sum[:16])
}

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
	outboxPath := fs.String("outbox-path", "", "durable outbox SQLite path (enables commit-before-publish)")
	outboxMaxBytes := fs.Int64("outbox-max-bytes", 0, "retained-data quota in bytes (0 = 256 MiB default)")
	resume := fs.Bool("resume", false, "resume an existing run from the outbox")
	sourceAddr := fs.String("source-addr", "", "read-only source API listen address (empty disables)")
	sourceHold := fs.Bool("source-hold", false, "keep serving the source API after the terminal boundary")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *path == "" || fs.NArg() != 0 || uint64(*seed) > uint64(^uint32(0)) || *outboxMaxBytes < 0 {
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
	var store *outbox.Store
	if *outboxPath != "" {
		store, err = outbox.Open(*outboxPath, *outboxMaxBytes)
		if err != nil {
			return err
		}
		defer store.Close()
		digest, err := engine.ScenarioDigest(s)
		if err != nil {
			return err
		}
		if err := store.RegisterRun(ctx, *id, sourceGeneration(*id, digest), digest); err != nil {
			return fmt.Errorf("register source identity for %q: %w", *id, err)
		}
		state, _, stateErr := store.ReadState(ctx, *id)
		switch {
		case stateErr == nil:
			if !*resume {
				return fmt.Errorf("run %q already exists in outbox %q; pass --resume to continue it", *id, *outboxPath)
			}
			e, err = engine.RestoreState(s, uint32(*seed), *id, state)
			if err != nil {
				return fmt.Errorf("cannot resume run %q: %w", *id, err)
			}
		case errors.Is(stateErr, outbox.ErrNoRun):
			if *resume {
				return fmt.Errorf("run %q not found in outbox %q", *id, *outboxPath)
			}
		default:
			return stateErr
		}
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
	var sourceServer *http.Server
	if *sourceAddr != "" {
		if store == nil {
			return errors.New("--source-addr requires --outbox-path")
		}
		sourceServer = outbox.NewSourceServer(*sourceAddr, store)
		listener, err := net.Listen("tcp", sourceServer.Addr)
		if err != nil {
			return err
		}
		go func() {
			if serveErr := sourceServer.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
				fmt.Fprintf(out, "SIMULATED simulator: source API stopped: %v\n", serveErr)
			}
		}()
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = sourceServer.Shutdown(shutdownCtx)
		}()
	}

	if store != nil {
		err = runner.RunDurableWithStatus(ctx, e, store, sink, *speed, pause, func(status runner.DurableStatus) {
			if status.Paused {
				fmt.Fprintf(out, "SIMULATED simulator: paused: %s\n", status.Reason)
			}
		})
	} else {
		err = runner.RunControlled(ctx, e, sink, *speed, pause)
	}
	if err == nil && sourceServer != nil && *sourceHold {
		manifest, manifestErr := store.Manifest(context.Background(), *id)
		if manifestErr == nil && manifest.Terminal != nil {
			<-ctx.Done()
		}
	}
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
