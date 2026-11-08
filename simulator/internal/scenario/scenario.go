package scenario

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"strings"
)

type Station struct {
	ID            string `json:"id"`
	ProcessTimeMs int64  `json:"processTimeMs"`
	InputCapacity int    `json:"inputCapacity"`
}
type Fault struct {
	SimTimeMs int64  `json:"simTimeMs"`
	MachineID string `json:"machineId"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
}
type Scenario struct {
	Quantity   int       `json:"quantity"`
	DefectRate float64   `json:"defectRate"`
	Stations   []Station `json:"stations"`
	Faults     []Fault   `json:"faults"`
}

func Load(path string) (Scenario, error) {
	var s Scenario
	f, err := os.Open(path)
	if err != nil {
		return s, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return s, err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return s, errors.New("unexpected trailing JSON")
		}
		return s, err
	}
	return s, s.Validate()
}
func (s Scenario) Validate() error {
	if s.Quantity < 1 || math.IsNaN(s.DefectRate) || s.DefectRate < 0 || s.DefectRate > 1 || len(s.Stations) == 0 {
		return errors.New("invalid scenario")
	}
	ids := map[string]bool{}
	for _, st := range s.Stations {
		if st.ID == "" || strings.ContainsAny(st.ID, "/+#") || ids[st.ID] || st.ProcessTimeMs <= 0 || st.InputCapacity < 1 {
			return errors.New("invalid station")
		}
		ids[st.ID] = true
	}
	faulted := map[string]bool{}
	var last int64
	for _, f := range s.Faults {
		if !ids[f.MachineID] || f.Reason == "" || f.SimTimeMs < last || f.SimTimeMs < 0 || (f.Action != "raise" && f.Action != "clear") || (f.Action == "raise" && faulted[f.MachineID]) || (f.Action == "clear" && !faulted[f.MachineID]) {
			return errors.New("invalid fault command")
		}
		faulted[f.MachineID] = f.Action == "raise"
		last = f.SimTimeMs
	}
	return nil
}
