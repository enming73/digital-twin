package scenario

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidate(t *testing.T) {
	valid := Scenario{Quantity: 100, DefectRate: .03, Stations: []Station{{ID: "loader", ProcessTimeMs: 3000, InputCapacity: 2}, {ID: "unloader", ProcessTimeMs: 3000, InputCapacity: 2}}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	valid.Stations[1].ID = "loader"
	if err := valid.Validate(); err == nil {
		t.Fatal("duplicate station accepted")
	}
}

func TestLoadRejectsTrailingJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scenario.json")
	data := `{"quantity":1,"stations":[{"id":"loader","processTimeMs":1,"inputCapacity":1}]} {}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("accepted trailing JSON")
	}
}
