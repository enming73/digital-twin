package engine

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"smt-digital-twin/simulator/internal/scenario"
)

func checkpointScenario() scenario.Scenario {
	return scenario.Scenario{
		Quantity:   12,
		DefectRate: .35,
		Stations: []scenario.Station{
			{ID: "loader", ProcessTimeMs: 7, InputCapacity: 2},
			{ID: "spi", ProcessTimeMs: 11, InputCapacity: 1},
			{ID: "mounter", ProcessTimeMs: 5, InputCapacity: 2},
			{ID: "unloader", ProcessTimeMs: 9, InputCapacity: 1},
		},
		Faults: []scenario.Fault{
			{SimTimeMs: 23, MachineID: "spi", Action: "raise", Reason: "test"},
			{SimTimeMs: 61, MachineID: "mounter", Action: "raise", Reason: "test"},
			{SimTimeMs: 97, MachineID: "spi", Action: "clear", Reason: "recovered"},
			{SimTimeMs: 151, MachineID: "mounter", Action: "clear", Reason: "recovered"},
		},
	}
}

func runAll(t *testing.T, e *Engine) []string {
	t.Helper()
	var trace []string
	for i := 0; i < 2000; i++ {
		batch, done, err := e.Next()
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range batch {
			raw, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			trace = append(trace, string(raw))
		}
		if done {
			return trace
		}
	}
	t.Fatal("simulation did not terminate")
	return nil
}

func TestCheckpointContinuationMatchesUninterruptedRun(t *testing.T) {
	s := checkpointScenario()
	for _, split := range []int{0, 1, 2, 7, 40} {
		t.Run(fmt.Sprintf("after-%d-boundaries", split), func(t *testing.T) {
			baseline, err := New(s, 2026, "run-checkpoint")
			if err != nil {
				t.Fatal(err)
			}
			want := runAll(t, baseline)

			part, err := New(s, 2026, "run-checkpoint")
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for i := 0; i < split; i++ {
				batch, done, err := part.Next()
				if err != nil {
					t.Fatal(err)
				}
				for _, event := range batch {
					raw, err := json.Marshal(event)
					if err != nil {
						t.Fatal(err)
					}
					got = append(got, string(raw))
				}
				if done {
					t.Fatalf("scenario finished before split %d", split)
				}
			}
			state, err := part.ExportState()
			if err != nil {
				t.Fatal(err)
			}
			restored, err := RestoreState(s, 2026, "run-checkpoint", state)
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, runAll(t, restored)...)
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("resumed run differs at split %d:\nfirst diff: %s", split, firstDifference(want, got))
			}
		})
	}
}

func firstDifference(want, got []string) string {
	for i := 0; i < len(want) && i < len(got); i++ {
		if want[i] != got[i] {
			return fmt.Sprintf("index %d\nwant %s\ngot  %s", i, want[i], got[i])
		}
	}
	return fmt.Sprintf("length want %d got %d", len(want), len(got))
}

func TestCheckpointRoundTripPreservesFullState(t *testing.T) {
	s := checkpointScenario()
	e, err := New(s, 7, "run-state")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 9; i++ {
		if _, done, err := e.Next(); err != nil || done {
			t.Fatalf("boundary %d: done=%v err=%v", i, done, err)
		}
	}
	state, err := e.ExportState()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreState(s, 7, "run-state", state)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(e.Snapshot(), restored.Snapshot()) {
		t.Fatal("snapshot not preserved")
	}
	nextA, doneA, errA := e.Next()
	nextB, doneB, errB := restored.Next()
	if errA != nil || errB != nil || doneA != doneB || !reflect.DeepEqual(nextA, nextB) {
		t.Fatalf("continuation differs: done %v/%v err %v/%v", doneA, doneB, errA, errB)
	}
}

func TestCheckpointRejectsIdentityMismatch(t *testing.T) {
	s := checkpointScenario()
	e, err := New(s, 2026, "run-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.Next(); err != nil {
		t.Fatal(err)
	}
	state, err := e.ExportState()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		config scenario.Scenario
		seed   uint32
		runID  string
	}{
		{"run id", s, 2026, "run-b"},
		{"seed", s, 2027, "run-a"},
		{"topology", func() scenario.Scenario { c := checkpointScenario(); c.Stations[1].ProcessTimeMs++; return c }(), 2026, "run-a"},
		{"quantity", func() scenario.Scenario { c := checkpointScenario(); c.Quantity++; return c }(), 2026, "run-a"},
		{"faults", func() scenario.Scenario { c := checkpointScenario(); c.Faults = c.Faults[:2]; return c }(), 2026, "run-a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := RestoreState(tc.config, tc.seed, tc.runID, state); err == nil {
				t.Fatal("mismatched checkpoint accepted")
			}
		})
	}
}

func TestCheckpointRejectsCorruption(t *testing.T) {
	s := checkpointScenario()
	e, err := New(s, 2026, "run-a")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if _, _, err := e.Next(); err != nil {
			t.Fatal(err)
		}
	}
	state, err := e.ExportState()
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"truncated": state[:len(state)/2],
		"empty":     {},
		"garbage":   []byte("not json"),
		"tampered":  []byte(strings.Replace(string(state), `"sequence":`, `"sequence":1`, 1)),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := RestoreState(s, 2026, "run-a", data); err == nil {
				t.Fatal("corrupt checkpoint accepted")
			}
		})
	}
}

func TestCheckpointRejectsImpossibleState(t *testing.T) {
	s := checkpointScenario()
	valid := func(t *testing.T) map[string]any {
		e, err := New(s, 2026, "run-a")
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 5; i++ {
			if _, _, err := e.Next(); err != nil {
				t.Fatal(err)
			}
		}
		state, err := e.ExportState()
		if err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Checkpoint map[string]any `json:"checkpoint"`
			SHA256     string         `json:"sha256"`
		}
		if err := json.Unmarshal(state, &envelope); err != nil {
			t.Fatal(err)
		}
		return envelope.Checkpoint
	}
	encode := func(t *testing.T, checkpoint map[string]any) []byte {
		t.Helper()
		payload, err := json.Marshal(checkpoint)
		if err != nil {
			t.Fatal(err)
		}
		envelope := map[string]any{"checkpoint": json.RawMessage(payload), "sha256": digestBytes(payload)}
		data, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	mutations := map[string]func(map[string]any){
		"version":                func(c map[string]any) { c["version"] = 99 },
		"fault index":            func(c map[string]any) { c["faultIndex"] = 99 },
		"negative time":          func(c map[string]any) { c["snapshot"].(map[string]any)["SimTimeMs"] = -1 },
		"created above quantity": func(c map[string]any) { c["snapshot"].(map[string]any)["Created"] = float64(s.Quantity + 1) },
		"queue over capacity": func(c map[string]any) {
			stations := c["snapshot"].(map[string]any)["Stations"].([]any)
			station := stations[0].(map[string]any)
			station["Queue"] = []any{"board-1", "board-2", "board-3"}
		},
		"unknown failed board": func(c map[string]any) { c["failed"] = []any{"board-9999"} },
		"station order": func(c map[string]any) {
			stations := c["snapshot"].(map[string]any)["Stations"].([]any)
			stations[0], stations[1] = stations[1], stations[0]
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			checkpoint := valid(t)
			mutate(checkpoint)
			if _, err := RestoreState(s, 2026, "run-a", encode(t, checkpoint)); err == nil {
				t.Fatal("impossible checkpoint accepted")
			}
		})
	}
}
