// Package contract defines synthetic external event envelopes.
package contract

import (
	"errors"
	"fmt"
	"strings"
	"time"
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

func validID(s string) bool { return s != "" && !strings.ContainsAny(s, "/+#") }

func Validate(e Event) error {
	if e.SchemaVersion != 1 || !validID(e.EventID) || !validID(e.RunID) || e.Source != "simulator" || e.Sequence < 1 || e.SimTimeMs < 0 {
		return errors.New("invalid event envelope")
	}
	stamp, err := time.Parse(time.RFC3339Nano, e.EmittedAt)
	if err != nil || !strings.HasSuffix(e.EmittedAt, "Z") || stamp.Location() != time.UTC {
		return errors.New("invalid UTC emittedAt")
	}
	if e.Payload["synthetic"] != true {
		return errors.New("payload must be synthetic")
	}
	fields := map[string]string{"synthetic": "bool"}
	line := false
	switch e.EventType {
	case "pcb.entered", "pcb.exited", "pcb.process.completed":
		fields["boardId"] = "id"
		fields["workOrderId"] = "id"
	case "pcb.inspection.completed":
		fields["boardId"] = "id"
		fields["workOrderId"] = "id"
		fields["passed"] = "bool"
	case "machine.state.changed":
		fields["observedState"] = "state"
		fields["flowState"] = "flow"
		fields["reason"] = "string"
	case "machine.alarm.raised", "machine.alarm.cleared":
		fields["alarmId"] = "id"
		fields["reason"] = "string"
	case "workorder.started":
		line = true
		fields["workOrderId"] = "id"
		fields["quantity"] = "positive"
	case "workorder.completed":
		line = true
		fields["workOrderId"] = "id"
		fields["quantity"] = "positive"
		fields["passed"] = "number"
		fields["failed"] = "number"
	default:
		return errors.New("unknown event type")
	}
	if line && e.MachineID != nil || !line && (e.MachineID == nil || !validID(*e.MachineID)) {
		return errors.New("invalid machineId")
	}
	if e.EventType == "pcb.inspection.completed" && *e.MachineID != "spi" && *e.MachineID != "aoi" {
		return errors.New("inspection must be at spi or aoi")
	}
	if len(e.Payload) != len(fields) {
		return errors.New("invalid payload fields")
	}
	for name, kind := range fields {
		v, ok := e.Payload[name]
		if !ok {
			return fmt.Errorf("missing %s", name)
		}
		s, str := v.(string)
		switch kind {
		case "id":
			if !str || !validID(s) {
				return fmt.Errorf("invalid %s", name)
			}
		case "string":
			if !str || s == "" {
				return fmt.Errorf("invalid %s", name)
			}
		case "state":
			if !str || (s != "IDLE" && s != "RUNNING" && s != "FAULT") {
				return errors.New("invalid observedState")
			}
		case "flow":
			if !str || (s != "NORMAL" && s != "BLOCKED" && s != "STARVED") {
				return errors.New("invalid flowState")
			}
		case "bool":
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("invalid %s", name)
			}
		case "positive", "number":
			n, ok := integer(v)
			if !ok || n < 0 || (kind == "positive" && n == 0) {
				return fmt.Errorf("invalid %s", name)
			}
		}
	}
	if e.EventType == "workorder.completed" {
		n, _ := integer(e.Payload["quantity"])
		p, _ := integer(e.Payload["passed"])
		f, _ := integer(e.Payload["failed"])
		if p > n || f != n-p {
			return errors.New("completion counts do not reconcile")
		}
	}
	return nil
}

func integer(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		if n >= 0 && n <= 1<<53 && n == float64(int64(n)) {
			return int64(n), true
		}
	}
	return 0, false
}
