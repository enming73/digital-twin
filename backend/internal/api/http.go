package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"smt-digital-twin/backend/internal/ingest"
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
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		h := consumer.Health()
		w.Header().Set("Content-Type", "application/json")
		if !h.Connected || !h.Subscribed {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		writeJSON(w, h)
	})
	mux.HandleFunc("GET /api/assets", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, assets()) })
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
