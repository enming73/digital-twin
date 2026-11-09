package replay

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"smt-digital-twin/backend/internal/contract"
	"smt-digital-twin/backend/internal/twin"
	"testing"
	"time"
)

func TestImportCompleteStreamAndRejectTruncation(t *testing.T) {
	schema, err := filepath.Abs("../../../contracts/event.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TWIN_SCHEMA_PATH", schema)
	cmd := exec.Command("go", "run", "./cmd/simulator", "run", "--scenario", "scenarios/normal.json", "--run-id", "r", "--speed", "100000", "--dry-run")
	cmd.Dir = "../../../simulator"
	data, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	stations := []string{"loader", "printer", "spi", "mounter", "reflow", "aoi", "unloader"}
	store, err := Import(bytes.NewReader(data), "epoch", "r", stations)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := store.Snapshot("r")
	if len(snapshot.Boards) != 100 || snapshot.WorkOrders["work-order-1"].Status != "completed" {
		t.Fatalf("snapshot: %d", len(snapshot.Boards))
	}
	live := twin.NewStore("different", map[string][]string{"r": stations})
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		var e contract.Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		if _, err := live.Apply(e, time.Unix(0, 0)); err != nil {
			t.Fatal(err)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	actual, _ := live.Snapshot("r")
	snapshot.Epoch = ""
	actual.Epoch = ""
	snapshot.LastIngestedAt = time.Time{}
	actual.LastIngestedAt = time.Time{}
	a, _ := json.Marshal(snapshot)
	b, _ := json.Marshal(actual)
	if !bytes.Equal(a, b) {
		t.Fatal("offline projection differs from live projection")
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	truncated := bytes.Join(lines[:len(lines)-1], []byte("\n"))
	if _, err := Import(bytes.NewReader(truncated), "epoch", "r", stations); err == nil {
		t.Fatal("truncated stream accepted")
	}
	missing := append(append([][]byte{}, lines[:1]...), lines[2:]...)
	if _, err := Import(bytes.NewReader(bytes.Join(missing, []byte("\n"))), "epoch", "r", stations); err == nil {
		t.Fatal("missing event accepted")
	}
}
