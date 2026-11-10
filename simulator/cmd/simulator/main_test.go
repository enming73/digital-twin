package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"smt-digital-twin/simulator/internal/outbox"
)

func TestDryRunOutputsNDJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scenario.json")
	if err := os.WriteFile(path, []byte(`{"quantity":1,"stations":[{"id":"loader","processTimeMs":1,"inputCapacity":1}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"run", "--scenario", path, "--seed", "1", "--speed", "1000", "--dry-run"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"eventType":"workorder.completed"`) {
		t.Fatalf("missing completion: %s", out.String())
	}
}

func TestDurableRunRequiresExplicitResume(t *testing.T) {
	dir := t.TempDir()
	scenarioPath := filepath.Join(dir, "scenario.json")
	if err := os.WriteFile(scenarioPath, []byte(`{"quantity":1,"stations":[{"id":"loader","processTimeMs":1,"inputCapacity":1}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	outboxPath := filepath.Join(dir, "outbox.db")
	args := []string{"run", "--scenario", scenarioPath, "--seed", "1", "--speed", "1000", "--dry-run", "--outbox-path", outboxPath}
	var first bytes.Buffer
	if err := run(context.Background(), args, &first); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first.String(), `"eventType":"workorder.completed"`) {
		t.Fatalf("missing completion: %s", first.String())
	}

	var duplicate bytes.Buffer
	if err := run(context.Background(), args, &duplicate); err == nil || !strings.Contains(err.Error(), "resume") {
		t.Fatalf("existing run was not rejected: %v", err)
	}

	var resumed bytes.Buffer
	resumedArgs := append(append([]string{}, args...), "--resume")
	if err := run(context.Background(), resumedArgs, &resumed); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(resumed.String(), `"eventType":"workorder.completed"`) {
		t.Fatal("completed run was replayed instead of resuming at its terminal boundary")
	}
}

func TestResumeUnknownRunFails(t *testing.T) {
	dir := t.TempDir()
	scenarioPath := filepath.Join(dir, "scenario.json")
	if err := os.WriteFile(scenarioPath, []byte(`{"quantity":1,"stations":[{"id":"loader","processTimeMs":1,"inputCapacity":1}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	err := run(context.Background(), []string{"run", "--scenario", scenarioPath, "--dry-run", "--outbox-path", filepath.Join(dir, "empty.db"), "--resume"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("resume of unknown run: %v", err)
	}
}

func TestDurableRunRegistersSourceManifest(t *testing.T) {
	dir := t.TempDir()
	scenarioPath := filepath.Join(dir, "scenario.json")
	if err := os.WriteFile(scenarioPath, []byte(`{"quantity":1,"stations":[{"id":"loader","processTimeMs":1,"inputCapacity":1}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	outboxPath := filepath.Join(dir, "outbox.db")
	if err := run(context.Background(), []string{"run", "--scenario", scenarioPath, "--seed", "1", "--speed", "1000", "--dry-run", "--outbox-path", outboxPath}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	store, err := outbox.Open(outboxPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manifest, err := store.Manifest(context.Background(), "demo-run")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Generation == "" || manifest.ScenarioDigest == "" {
		t.Fatalf("source identity not registered: %+v", manifest)
	}
	if manifest.Head == 0 || manifest.Terminal == nil {
		t.Fatalf("terminal boundary not published: %+v", manifest)
	}
}

func TestSourceServerHoldsAfterCompletion(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "simulator")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, output)
	}
	scenarioPath := filepath.Join(dir, "scenario.json")
	if err := os.WriteFile(scenarioPath, []byte(`{"quantity":1,"stations":[{"id":"loader","processTimeMs":1,"inputCapacity":1}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()

	cmd := exec.Command(binary, "run", "--scenario", scenarioPath, "--seed", "1", "--speed", "1000", "--dry-run",
		"--outbox-path", filepath.Join(dir, "outbox.db"), "--source-addr", addr, "--source-hold")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	})

	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(15 * time.Second)
	for {
		resp, err := client.Get("http://" + addr + "/v1/runs/demo-run/manifest")
		if err == nil {
			var manifest struct {
				Head     int64 `json:"head"`
				Terminal *struct {
					EventCount int64 `json:"eventCount"`
				} `json:"terminal"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&manifest)
			resp.Body.Close()
			if decodeErr == nil && resp.StatusCode == 200 && manifest.Terminal != nil && manifest.Terminal.EventCount > 0 {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("source manifest never became terminal: %s", stderr.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("exit after hold: %v: %s", err, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("source server did not shut down")
	}
}

func TestProcessPauseResumeSignal(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "simulator")
	build := exec.Command("go", "build", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	path := filepath.Join(dir, "scenario.json")
	if err := os.WriteFile(path, []byte(`{"quantity":1,"stations":[{"id":"loader","processTimeMs":500,"inputCapacity":1}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "run", "--scenario", path, "--dry-run")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	})
	lines := make(chan string, 32)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case line := <-lines:
			if strings.Contains(line, `"eventType":"machine.state.changed"`) {
				goto started
			}
		case <-deadline:
			t.Fatal("no startup event")
		}
	}
started:
	if err := cmd.Process.Signal(syscall.SIGUSR1); err != nil {
		t.Fatal(err)
	}
	select {
	case line := <-lines:
		t.Fatalf("event while paused: %s", line)
	case <-time.After(650 * time.Millisecond):
	}
	if err := cmd.Process.Signal(syscall.SIGUSR1); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("exit: %v: %s", err, stderr.String())
	}
}

func TestSIGTERMWhileBrokerUnavailableReturnsError(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "simulator")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, output)
	}
	path := filepath.Join(dir, "scenario.json")
	if err := os.WriteFile(path, []byte(`{"quantity":1,"stations":[{"id":"loader","processTimeMs":100,"inputCapacity":1}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "run", "--scenario", path, "--mqtt-url", "tcp://127.0.0.1:1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	})
	time.Sleep(300 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- cmd.Wait() }()
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("expected nonzero exit on unsent event")
		}
		if !strings.Contains(stderr.String(), "context canceled") {
			t.Fatalf("missing cancellation diagnostic: %s", stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not terminate")
	}
}
