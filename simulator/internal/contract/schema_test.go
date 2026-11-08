package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestFixturesConformToSchemaAndGoValidation(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	schema, err := compiler.Compile("../../../contracts/event.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob("../../../contracts/fixtures/*.json")
	if err != nil || len(paths) != 9 {
		t.Fatalf("expected nine fixtures: %v %v", paths, err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var value any
			if err := json.Unmarshal(data, &value); err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(value); err != nil {
				t.Fatal(err)
			}
			var event Event
			if err := json.Unmarshal(data, &event); err != nil {
				t.Fatal(err)
			}
			if err := Validate(event); err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			var generated any
			if err := json.Unmarshal(encoded, &generated); err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(generated); err != nil {
				t.Fatalf("generated JSON: %v", err)
			}
			value.(map[string]any)["eventType"] = "unknown"
			if schema.Validate(value) == nil {
				t.Fatal("schema accepted unknown type")
			}
		})
	}
}

func TestBothValidatorsRejectMalformedEnvelopes(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	schema, err := compiler.Compile("../../../contracts/event.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../../contracts/fixtures/pcb.entered.json")
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]func(map[string]any){
		"missing event ID":      func(m map[string]any) { delete(m, "eventId") },
		"invalid topic ID":      func(m map[string]any) { m["runId"] = "bad/+" },
		"missing machine":       func(m map[string]any) { m["machineId"] = nil },
		"invalid time":          func(m map[string]any) { m["emittedAt"] = "yesterday" },
		"unknown payload":       func(m map[string]any) { m["payload"].(map[string]any)["extra"] = true },
		"missing payload field": func(m map[string]any) { delete(m["payload"].(map[string]any), "boardId") },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal(data, &value); err != nil {
				t.Fatal(err)
			}
			mutate(value)
			if schema.Validate(value) == nil {
				t.Fatal("schema accepted invalid event")
			}
			changed, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var event Event
			if err := json.Unmarshal(changed, &event); err != nil {
				t.Fatal(err)
			}
			if Validate(event) == nil {
				t.Fatal("Go validator accepted invalid event")
			}
		})
	}
}
