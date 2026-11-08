package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"smt-digital-twin/simulator/internal/contract"
)

func TestNDJSONStampsAndValidates(t *testing.T) {
	var output bytes.Buffer
	sink := NewNDJSON(&output)
	machine := "loader"
	event := contract.Event{SchemaVersion: 1, EventID: "event-00000001", RunID: "demo", Source: "simulator", MachineID: &machine, EventType: "pcb.entered", Sequence: 1, Payload: map[string]any{"synthetic": true, "boardId": "board-1", "workOrderId": "work-order-1"}}
	if err := sink.Publish(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	var result contract.Event
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &result); err != nil {
		t.Fatal(err)
	}
	if err := contract.Validate(result); err != nil {
		t.Fatal(err)
	}
	if result.EventID != event.EventID {
		t.Fatal("identity changed")
	}
}
