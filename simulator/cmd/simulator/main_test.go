package main

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
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
