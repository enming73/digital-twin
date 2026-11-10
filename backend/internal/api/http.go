package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"smt-digital-twin/backend/internal/ingest"
	"smt-digital-twin/backend/internal/persistence"
	"smt-digital-twin/backend/internal/replay"
	"smt-digital-twin/backend/internal/twin"
)

type asset struct {
	MachineID      string  `json:"machineId"`
	StationType    string  `json:"stationType"`
	ModelURL       *string `json:"modelURL"`
	Representation string  `json:"representation"`
}

func assets() []asset {
	ids := []string{"loader", "printer", "spi", "mounter", "reflow", "aoi", "unloader"}
	out := make([]asset, 0, len(ids))
	for _, id := range ids {
		a := asset{MachineID: id, StationType: id, Representation: "procedural"}
		if id == "printer" || id == "mounter" {
			url := "/models/" + id + ".glb"
			a.ModelURL = &url
			a.Representation = "glb"
		}
		out = append(out, a)
	}
	return out
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func NewHandler(store *twin.Store, consumer *ingest.Consumer) http.Handler {
	return NewHandlerWithReadiness(store, consumer, nil)
}

func NewHandlerWithReadiness(store *twin.Store, consumer *ingest.Consumer, readiness func(context.Context) error) http.Handler {
	return NewHandlerWithHistory(store, consumer, readiness, nil)
}

type History interface {
	Replay(context.Context, string, int64) (replay.HistoricalSnapshot, error)
	Events(context.Context, string, int64, int64, int, int) ([]persistence.StoredEvent, error)
}

func NewHandlerWithHistory(store *twin.Store, consumer *ingest.Consumer, readiness func(context.Context) error, history History) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/metrics", func(w http.ResponseWriter, r *http.Request) {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		writeJSON(w, struct {
			twin.Metrics
			GoMemoryBytes uint64 `json:"goMemoryBytes"`
		}{store.Metrics(), mem.Alloc})
	})
	if history != nil {
		writeHistoryError := func(w http.ResponseWriter, err error) {
			if errors.Is(err, persistence.ErrRunNotFound) {
				http.Error(w, "run not found", http.StatusNotFound)
				return
			}
			if errors.Is(err, persistence.ErrUnsupportedVersion) {
				http.Error(w, "unsupported event version", http.StatusUnprocessableEntity)
				return
			}
			http.Error(w, "historical data unavailable", http.StatusServiceUnavailable)
		}
		mux.HandleFunc("GET /api/runs/{id}/replay", func(w http.ResponseWriter, r *http.Request) {
			at, err := strconv.ParseInt(r.URL.Query().Get("simTimeMs"), 10, 64)
			if err != nil || at < 0 {
				http.Error(w, "invalid simulated time", http.StatusBadRequest)
				return
			}
			result, err := history.Replay(r.Context(), r.PathValue("id"), at)
			if err != nil {
				writeHistoryError(w, err)
				return
			}
			writeJSON(w, result)
		})
		mux.HandleFunc("GET /api/runs/{id}/events", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			from, e1 := strconv.ParseInt(q.Get("fromSimTimeMs"), 10, 64)
			to, e2 := strconv.ParseInt(q.Get("toSimTimeMs"), 10, 64)
			after, limit := int64(0), int64(100)
			var e3, e4 error
			if q.Has("afterSequence") {
				after, e3 = strconv.ParseInt(q.Get("afterSequence"), 10, 32)
			}
			if q.Has("limit") {
				limit, e4 = strconv.ParseInt(q.Get("limit"), 10, 32)
			}
			if e1 != nil || e2 != nil || e3 != nil || e4 != nil || from < 0 || to < from || after < 0 || limit < 1 || limit > 1000 {
				http.Error(w, "invalid event range", http.StatusBadRequest)
				return
			}
			result, err := history.Events(r.Context(), r.PathValue("id"), from, to, int(after), int(limit))
			if err != nil {
				writeHistoryError(w, err)
				return
			}
			writeJSON(w, result)
		})
	}
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		h := consumer.Health()
		w.Header().Set("Content-Type", "application/json")
		ready := h.Connected && h.Subscribed
		if readiness != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			if err := readiness(ctx); err != nil {
				ready = false
			}
			cancel()
		}
		if !ready {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		writeJSON(w, h)
	})
	mux.HandleFunc("GET /api/assets", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, assets()) })
	mux.HandleFunc("GET /api/runs/{id}/workorders/{order}", func(w http.ResponseWriter, r *http.Request) {
		detail, ok := store.WorkOrderDetail(r.PathValue("id"), r.PathValue("order"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, detail)
	})
	mux.HandleFunc("GET /api/runs/{id}/boards/{board}/trace", func(w http.ResponseWriter, r *http.Request) {
		trace, ok := store.BoardTrace(r.PathValue("id"), r.PathValue("board"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, trace)
	})
	mux.HandleFunc("GET /api/runs/{id}/snapshot", func(w http.ResponseWriter, r *http.Request) {
		s, ok := store.Snapshot(r.PathValue("id"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, s)
	})
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		return err == nil && (u.Scheme == "http" || u.Scheme == "https") && strings.EqualFold(u.Host, r.Host)
	}}
	mux.HandleFunc("GET /ws/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		snapshot, ch, cancel, ok := store.Subscribe(r.PathValue("id"), 256)
		if !ok {
			http.NotFound(w, r)
			return
		}
		defer cancel()
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetReadLimit(1024)
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()
		send := func(v any) error {
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			return conn.WriteJSON(v)
		}
		if err := send(map[string]any{"type": "snapshot", "epoch": snapshot.Epoch, "revision": snapshot.Revision, "snapshot": snapshot}); err != nil {
			return
		}
		for {
			select {
			case <-r.Context().Done():
				return
			case <-done:
				return
			case update, ok := <-ch:
				if !ok {
					_ = send(map[string]any{"type": "resync_required", "reason": "slow_consumer"})
					return
				}
				if send(update) != nil {
					return
				}
			}
		}
	})
	return mux
}
