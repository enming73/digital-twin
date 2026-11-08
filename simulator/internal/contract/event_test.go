package contract

import "testing"

func TestValidateEvent(t *testing.T) {
	machine := "mounter"
	base := Event{SchemaVersion: 1, EventID: "event-00000001", RunID: "demo-run", Source: "simulator", MachineID: &machine, EventType: "pcb.entered", Sequence: 1, EmittedAt: "2026-11-08T12:00:00Z", Payload: map[string]any{"synthetic": true, "boardId": "board-1", "workOrderId": "work-order-1"}}
	if err := Validate(base); err != nil {
		t.Fatalf("valid event: %v", err)
	}
	tests := map[string]func(*Event){
		"missing event id":  func(e *Event) { e.EventID = "" },
		"invalid topic id":  func(e *Event) { e.RunID = "bad/+" },
		"unknown type":      func(e *Event) { e.EventType = "unknown" },
		"missing machine":   func(e *Event) { e.MachineID = nil },
		"invalid timestamp": func(e *Event) { e.EmittedAt = "yesterday" },
		"wrong payload":     func(e *Event) { e.Payload = map[string]any{"synthetic": true} },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			event := base
			mutate(&event)
			if Validate(event) == nil {
				t.Fatal("expected invalid event")
			}
		})
	}
}
