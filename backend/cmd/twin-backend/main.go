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

	"smt-digital-twin/backend/internal/api"
	"smt-digital-twin/backend/internal/ingest"
	"smt-digital-twin/backend/internal/replay"
	"smt-digital-twin/backend/internal/twin"
)

func validID(id string) bool { return id != "" && !strings.ContainsAny(id, "/+#\x00") }
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *importPath == "" {
		go func() {
			if err := consumer.Run(ctx, *broker); err != nil {
				log.Printf("MQTT error: %v", err)
				stop()
			}
		}()
	}
	server := &http.Server{Addr: *listen, Handler: api.NewHandler(store, consumer), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
