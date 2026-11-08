package publisher

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"smt-digital-twin/simulator/internal/contract"
)

type Sink interface {
	Publish(context.Context, contract.Event) error
	Close(context.Context) error
}
type ndjson struct{ writer io.Writer }

func NewNDJSON(w io.Writer) Sink { return &ndjson{writer: w} }
func (s *ndjson) Publish(ctx context.Context, event contract.Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if event.EmittedAt == "" {
		event.EmittedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := contract.Validate(event); err != nil {
		return err
	}
	return json.NewEncoder(s.writer).Encode(event)
}
func (s *ndjson) Close(context.Context) error { return nil }
