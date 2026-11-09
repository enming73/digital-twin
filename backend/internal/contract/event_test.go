package contract

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixturesAndInvalidMessages(t *testing.T) {
	t.Setenv("TWIN_SCHEMA_PATH", "../../../contracts/event.schema.json")
	files, err := filepath.Glob("../../../contracts/fixtures/*.json")
	if err != nil || len(files) != 9 {
		t.Fatalf("fixtures: %v %d", err, len(files))
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var e struct {
			RunID     string  `json:"runId"`
			MachineID *string `json:"machineId"`
		}
		if err := json.Unmarshal(data, &e); err != nil {
			t.Fatal(err)
		}
		topic := "demo/smt/" + e.RunID + "/line/events"
		if e.MachineID != nil {
			topic = "demo/smt/" + e.RunID + "/machines/" + *e.MachineID + "/events"
		}
		if _, err := DecodeAndValidate(topic, data); err != nil {
			t.Errorf("%s: %v", file, err)
		}
	}
	b, err := os.ReadFile("../../../contracts/fixtures/workorder.completed.json")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	topic := "demo/smt/demo-run/line/events"
	tests := map[string]struct{ topic, body string }{
		"unknown field": {topic, strings.Replace(s, `"schemaVersion": 1`, `"extra": 1, "schemaVersion": 1`, 1)},
		"version":       {topic, strings.Replace(s, `"schemaVersion": 1`, `"schemaVersion": 2`, 1)},
		"synthetic":     {topic, strings.Replace(s, `"synthetic": true`, `"synthetic": false`, 1)},
		"utc":           {topic, strings.Replace(s, `2026-11-08T12:00:00Z`, `2026-11-08T12:00:00+00:00`, 1)},
		"counts":        {topic, strings.Replace(s, `"failed": 2`, `"failed": 1`, 1)},
		"topic run":     {"demo/smt/wrong/line/events", s},
		"topic machine": {"demo/smt/demo-run/machines/printer/events", s},
		"oversize":      {topic, strings.Repeat(" ", 1<<20) + s},
		"invalid json":  {topic, "{"},
		"duplicate key": {topic, strings.Replace(s, `"schemaVersion": 1`, `"schemaVersion": 1, "schemaVersion": 1`, 1)},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeAndValidate(tt.topic, []byte(tt.body)); err == nil {
				t.Fatal("invalid message accepted")
			}
		})
	}
	cmd := exec.Command("go", "run", "./cmd/simulator", "run", "--scenario", "scenarios/normal.json", "--speed", "100000", "--dry-run")
	cmd.Dir = "../../../simulator"
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	count := 0
	for scanner.Scan() {
		data := scanner.Bytes()
		var e struct {
			RunID     string  `json:"runId"`
			MachineID *string `json:"machineId"`
		}
		if err := json.Unmarshal(data, &e); err != nil {
			t.Fatal(err)
		}
		topic := "demo/smt/" + e.RunID + "/line/events"
		if e.MachineID != nil {
			topic = "demo/smt/" + e.RunID + "/machines/" + *e.MachineID + "/events"
		}
		if _, err := DecodeAndValidate(topic, data); err != nil {
			t.Fatalf("event %d: %v", count, err)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count < 100 {
		t.Fatalf("only %d events", count)
	}
}
