package outbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

// maxResponseBytes bounds a single events page, including envelopes and the
// enclosing JSON object, so a slow client cannot force unbounded buffering.
const maxResponseBytes = 1 << 20
const pageOverheadBytes = 64 << 10

// NewSourceHandler serves the versioned, read-only source protocol. It never
// exposes SQLite or accepts equipment commands.
func NewSourceHandler(store *Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/runs/{runId}/manifest", func(w http.ResponseWriter, r *http.Request) {
		if !supportedVersion(w, r) {
			return
		}
		manifest, err := store.Manifest(r.Context(), r.PathValue("runId"))
		if err != nil {
			writeSourceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, manifest)
	})
	mux.HandleFunc("GET /v1/runs/{runId}/events", func(w http.ResponseWriter, r *http.Request) {
		if !supportedVersion(w, r) {
			return
		}
		runID := r.PathValue("runId")
		after, afterErr := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
		limit, limitErr := strconv.Atoi(r.URL.Query().Get("limit"))
		if r.URL.Query().Get("after") == "" || r.URL.Query().Get("limit") == "" || afterErr != nil || limitErr != nil || after < 0 || limit < 1 || limit > 500 {
			writeSourceError(w, fmt.Errorf("%w: invalid page bounds", errInvalidRequest))
			return
		}
		if _, err := store.Manifest(r.Context(), runID); err != nil {
			writeSourceError(w, err)
			return
		}
		page, err := store.ReadPage(r.Context(), runID, after, limit)
		if err != nil {
			writeSourceError(w, err)
			return
		}
		envelopes := make([]json.RawMessage, 0, len(page.Events))
		used := pageOverheadBytes
		for _, raw := range page.Events {
			if used+len(raw)+1 > maxResponseBytes {
				if len(envelopes) == 0 {
					writeSourceError(w, errEventTooLarge)
					return
				}
				break
			}
			used += len(raw) + 1
			envelopes = append(envelopes, json.RawMessage(raw))
		}
		writeJSON(w, http.StatusOK, struct {
			ProtocolVersion int               `json:"protocolVersion"`
			RunID           string            `json:"runId"`
			Head            int64             `json:"head"`
			Terminal        *Terminal         `json:"terminal"`
			Events          []json.RawMessage `json:"events"`
		}{SourceProtocolVersion, runID, page.Head, page.Terminal, envelopes})
	})
	mux.HandleFunc("POST /v1/runs/{runId}/ack", func(w http.ResponseWriter, r *http.Request) {
		if !supportedVersion(w, r) {
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		var payload struct {
			ProtocolVersion int    `json:"protocolVersion"`
			Generation      string `json:"generation"`
			Sequence        int64  `json:"sequence"`
		}
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&payload); err != nil {
			writeSourceError(w, fmt.Errorf("%w: malformed acknowledgement", errInvalidRequest))
			return
		}
		if payload.ProtocolVersion != SourceProtocolVersion || payload.Generation == "" || payload.Sequence < 0 {
			writeSourceError(w, fmt.Errorf("%w: invalid acknowledgement", errInvalidRequest))
			return
		}
		acknowledged, err := store.Ack(r.Context(), r.PathValue("runId"), payload.Generation, payload.Sequence)
		if err != nil {
			writeSourceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Acknowledged int64 `json:"acknowledged"`
		}{acknowledged})
	})
	return mux
}

var errInvalidRequest = errors.New("invalid source request")
var errEventTooLarge = errors.New("event exceeds page byte limit")

func supportedVersion(w http.ResponseWriter, r *http.Request) bool {
	version := r.URL.Query().Get("protocolVersion")
	if version == "" {
		return true
	}
	parsed, err := strconv.Atoi(version)
	if err != nil || parsed != SourceProtocolVersion {
		writeSourceError(w, fmt.Errorf("%w: unsupported protocol version", errInvalidRequest))
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeSourceError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, errInvalidRequest):
		status = http.StatusBadRequest
	case errors.Is(err, ErrNoRun):
		status = http.StatusNotFound
	case errors.Is(err, ErrGenerationMismatch), errors.Is(err, ErrAckOrder), errors.Is(err, ErrAckOutOfRange):
		status = http.StatusConflict
	case errors.Is(err, errEventTooLarge):
		status = http.StatusRequestEntityTooLarge
	case errors.Is(err, ErrStorageUnavailable):
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{err.Error()})
}
