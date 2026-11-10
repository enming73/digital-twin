package recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// HTTPClient talks to the simulator's read-only durable source. Every request
// has a 5 second deadline and bounded retries; MQTT is never consulted.
type HTTPClient struct {
	base       string
	client     *http.Client
	retryDelay func(attempt int) time.Duration
}

func NewHTTPClient(base string) *HTTPClient {
	return &HTTPClient{
		base:   base,
		client: &http.Client{Timeout: 5 * time.Second},
		retryDelay: func(attempt int) time.Duration {
			delay := 250 * time.Millisecond
			for i := 0; i < attempt && delay < 5*time.Second; i++ {
				delay *= 2
			}
			if delay > 5*time.Second {
				delay = 5 * time.Second
			}
			return delay
		},
	}
}

func (c *HTTPClient) Manifest(ctx context.Context, runID string) (Manifest, error) {
	var manifest Manifest
	err := c.request(ctx, http.MethodGet, "/v1/runs/"+url.PathEscape(runID)+"/manifest", nil, &manifest)
	return manifest, err
}

func (c *HTTPClient) ReadPage(ctx context.Context, runID string, after int64, limit int) (Page, error) {
	var page Page
	endpoint := fmt.Sprintf("/v1/runs/%s/events?after=%d&limit=%d", url.PathEscape(runID), after, limit)
	err := c.request(ctx, http.MethodGet, endpoint, nil, &page)
	return page, err
}

func (c *HTTPClient) Ack(ctx context.Context, runID, generation string, sequence int64) error {
	body, err := json.Marshal(struct {
		ProtocolVersion int    `json:"protocolVersion"`
		Generation      string `json:"generation"`
		Sequence        int64  `json:"sequence"`
	}{1, generation, sequence})
	if err != nil {
		return err
	}
	return c.request(ctx, http.MethodPost, "/v1/runs/"+url.PathEscape(runID)+"/ack", body, nil)
}

func (c *HTTPClient) request(ctx context.Context, method, path string, body []byte, target any) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(c.retryDelay(attempt)):
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		payload, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			lastErr = fmt.Errorf("source %s %s: status %d", method, path, resp.StatusCode)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("source %s %s: status %d: %s", method, path, resp.StatusCode, bytes.TrimSpace(payload))
		}
		if target != nil {
			if err := json.Unmarshal(payload, target); err != nil {
				return fmt.Errorf("decode source response: %w", err)
			}
		}
		return nil
	}
	return fmt.Errorf("source unavailable: %w", lastErr)
}

var _ = errors.Is
