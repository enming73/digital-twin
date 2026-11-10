package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"smt-digital-twin/backend/internal/api"
	"smt-digital-twin/backend/internal/ingest"
	"smt-digital-twin/backend/internal/persistence"
	"smt-digital-twin/backend/internal/recovery"
	"smt-digital-twin/backend/internal/replay"
	"smt-digital-twin/backend/internal/twin"
)

func validID(id string) bool { return id != "" && !strings.ContainsAny(id, "/+#\x00") }
func finishShutdown(ctx context.Context, consumerDone <-chan struct{}, save func() error) error {
	select {
	case <-consumerDone:
		return save()
	case <-ctx.Done():
		return ctx.Err()
	}
}
func parseStations(value string) ([]string, error) {
	ids := strings.Split(value, ",")
	seen := map[string]bool{}
	for _, id := range ids {
		if !validID(id) || strings.TrimSpace(id) != id || seen[id] {
			return nil, errors.New("invalid or duplicate station ID")
		}
		seen[id] = true
	}
	return ids, nil
}
func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "HTTP listen address")
	broker := flag.String("mqtt-url", "tcp://127.0.0.1:1883", "MQTT broker URL")
	run := flag.String("run-id", "demo-run", "registered run ID")
	topology := flag.String("stations", "loader,printer,spi,mounter,reflow,aoi,unloader", "ordered station IDs")
	importPath := flag.String("import", "", "load a complete NDJSON run in offline mode")
	fresh := flag.Bool("expect-fresh-run", false, "assert publisher will start only after subscription is ready")
	flag.Parse()
	stations, err := parseStations(*topology)
	if err != nil || !validID(*run) {
		log.Fatal("invalid run or station IDs")
	}
	epoch := make([]byte, 16)
	if _, err = rand.Read(epoch); err != nil {
		log.Fatal(err)
	}
	store := twin.NewStore(hex.EncodeToString(epoch), map[string][]string{*run: stations})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var readiness func(context.Context) error
	var history api.History
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL != "" {
		if *importPath != "" {
			log.Fatal("DATABASE_URL cannot be combined with offline import")
		}
		pool, err := pgxpool.New(ctx, databaseURL)
		if err != nil {
			log.Fatal(err)
		}
		defer pool.Close()
		if err := pool.Ping(ctx); err != nil {
			log.Fatal(err)
		}
		if err := persistence.Migrate(ctx, pool); err != nil {
			log.Fatal(err)
		}
		lease, err := persistence.AcquireRunWriter(ctx, pool, *run)
		if err != nil {
			log.Fatal(err)
		}
		defer func() {
			if err := lease.Close(context.Background()); err != nil {
				log.Printf("close writer lease: %v", err)
			}
		}()
		if _, err := persistence.EnsureRun(ctx, pool, *run, stations); err != nil {
			log.Fatal(err)
		}
		store, err = persistence.RestoreWithLease(ctx, pool, *run, stations, hex.EncodeToString(epoch), lease)
		if err != nil {
			log.Fatal(err)
		}
		readiness = lease.Ping
		history = replay.HistoricalService{DB: pool}
		if sourceURL := os.Getenv("W6_SOURCE_URL"); sourceURL != "" {
			store.SetRequireTerminal()
			coordinator := recovery.New(store, recovery.NewHTTPClient(sourceURL), recovery.Config{
				RunID:          *run,
				Generation:     os.Getenv("W6_SOURCE_GENERATION"),
				ScenarioDigest: os.Getenv("W6_SCENARIO_DIGEST"),
			})
			go func() {
				if err := coordinator.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
					log.Printf("durable source recovery: %v", err)
				}
			}()
		}
	}
	if !*fresh && *importPath == "" {
		store.MarkUncertain("publisher may have started before subscription")
	}
	if *importPath != "" {
		file, err := os.Open(*importPath)
		if err != nil {
			log.Fatal(err)
		}
		store, err = replay.Import(file, hex.EncodeToString(epoch), *run, stations)
		_ = file.Close()
		if err != nil {
			log.Fatal(err)
		}
	}
	consumer := ingest.NewConsumer(store)
	if databaseURL != "" {
		if os.Getenv("W6_SOURCE_URL") != "" {
			consumer = ingest.NewDurableConsumerWithRecovery(store)
		} else {
			consumer = ingest.NewDurableConsumer(store)
		}
	}
	consumerDone := make(chan struct{})
	consumerFailure := make(chan error, 1)
	if *importPath == "" {
		go func() {
			defer close(consumerDone)
			if err := consumer.Run(ctx, *broker); err != nil {
				log.Printf("MQTT error: %v", err)
				select {
				case consumerFailure <- err:
				default:
				}
				stop()
			}
		}()
	} else {
		close(consumerDone)
	}
	server := &http.Server{Addr: *listen, Handler: api.NewHandlerWithHistory(store, consumer, readiness, history), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	serveErr := server.ListenAndServe()
	stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := finishShutdown(shutdownCtx, consumerDone, func() error {
		if databaseURL == "" {
			return nil
		}
		return store.SaveCheckpoint(*run)
	}); err != nil {
		log.Printf("checkpoint on shutdown: %v", err)
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		log.Print(serveErr)
		os.Exit(1)
	}
	select {
	case err := <-consumerFailure:
		log.Printf("durable ingestion failed: %v", err)
		os.Exit(1)
	default:
	}
}
