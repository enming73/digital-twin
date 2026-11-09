package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Event struct {
	SchemaVersion int            `json:"schemaVersion"`
	EventID       string         `json:"eventId"`
	RunID         string         `json:"runId"`
	Source        string         `json:"source"`
	MachineID     *string        `json:"machineId"`
	EventType     string         `json:"eventType"`
	Sequence      int            `json:"sequence"`
	SimTimeMs     int64          `json:"simTimeMs"`
	EmittedAt     string         `json:"emittedAt"`
	Payload       map[string]any `json:"payload"`
}

var schemaOnce sync.Once
var schema *jsonschema.Schema
var schemaErr error

func loadSchema() {
	path := os.Getenv("TWIN_SCHEMA_PATH")
	if path == "" {
		path = "../contracts/event.schema.json"
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	schema, schemaErr = compiler.Compile(path)
}

// checkValue rejects duplicate object keys at every depth and malformed JSON.
func checkValue(dec *json.Decoder) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	d, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch d {
	case '{':
		seen := map[string]bool{}
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return err
			}
			s, ok := key.(string)
			if !ok || seen[s] {
				return errors.New("duplicate or invalid JSON key")
			}
			seen[s] = true
			if err := checkValue(dec); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("invalid object")
		}
	case '[':
		for dec.More() {
			if err := checkValue(dec); err != nil {
				return err
			}
		}
		end, err := dec.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("invalid array")
		}
	default:
		return errors.New("unexpected delimiter")
	}
	return nil
}

func DecodeAndValidate(topic string, body []byte) (Event, error) {
	var event Event
	if len(body) > 1<<20 {
		return event, errors.New("message too large")
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := checkValue(dec); err != nil {
		return event, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return event, errors.New("trailing JSON")
	}
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return event, err
	}
	schemaOnce.Do(loadSchema)
	if schemaErr != nil {
		return event, fmt.Errorf("load schema: %w", schemaErr)
	}
	if err := schema.Validate(raw); err != nil {
		return event, err
	}
	if err := json.Unmarshal(body, &event); err != nil {
		return event, err
	}
	parts := strings.Split(topic, "/")
	valid := len(parts) == 5 && parts[0] == "demo" && parts[1] == "smt" && parts[2] == event.RunID && parts[3] == "line" && parts[4] == "events" && event.MachineID == nil
	if len(parts) == 6 && event.MachineID != nil {
		valid = parts[0] == "demo" && parts[1] == "smt" && parts[2] == event.RunID && parts[3] == "machines" && parts[4] == *event.MachineID && parts[5] == "events"
	}
	if !valid {
		return event, errors.New("topic does not match envelope")
	}
	if event.EventType == "workorder.completed" {
		q := event.Payload["quantity"].(float64)
		p := event.Payload["passed"].(float64)
		f := event.Payload["failed"].(float64)
		if p+f != q {
			return event, errors.New("completion counts do not reconcile")
		}
	}
	return event, nil
}
