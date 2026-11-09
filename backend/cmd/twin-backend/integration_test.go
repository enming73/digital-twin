package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestOfflineImportAfterProcessRestart(t *testing.T) {
	if os.Getenv("W3_PROCESS_INTEGRATION") != "1" {
		t.Skip("set W3_PROCESS_INTEGRATION=1")
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "backend")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %s %v", out, err)
	}
	sim := exec.Command("go", "run", "./cmd/simulator", "run", "--scenario", "scenarios/normal.json", "--run-id", "r", "--speed", "100000", "--dry-run")
	sim.Dir = "../../../simulator"
	data, err := sim.Output()
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "run.ndjson")
	if err := os.WriteFile(logPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	schema, _ := filepath.Abs("../../../contracts/event.schema.json")
	run := func() string {
		t.Helper()
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := l.Addr().String()
		_ = l.Close()
		cmd := exec.Command(binary, "--listen", addr, "--run-id", "r", "--import", logPath)
		cmd.Env = append(os.Environ(), "TWIN_SCHEMA_PATH="+schema)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
		deadline := time.Now().Add(20 * time.Second)
		for {
			resp, err := http.Get("http://" + addr + "/api/runs/r/snapshot")
			if err == nil {
				var snapshot struct {
					Epoch     string         `json:"epoch"`
					Integrity string         `json:"integrity"`
					Boards    map[string]any `json:"boards"`
				}
				decode := json.NewDecoder(resp.Body).Decode(&snapshot)
				_ = resp.Body.Close()
				if decode != nil {
					t.Fatal(decode)
				}
				if snapshot.Integrity != "complete" || len(snapshot.Boards) != 100 {
					t.Fatalf("snapshot: %+v", snapshot)
				}
				return snapshot.Epoch
			}
			if time.Now().After(deadline) {
				t.Fatalf("server did not start: %v", err)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	first, second := run(), run()
	if first == "" || first == second {
		t.Fatalf("epochs: %q %q", first, second)
	}
}
