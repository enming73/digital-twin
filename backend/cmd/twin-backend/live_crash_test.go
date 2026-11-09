package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func TestLiveCrashAndOfflineRecovery(t *testing.T) {
	if os.Getenv("W3_MQTT_INTEGRATION") != "1" {
		t.Skip("set W3_MQTT_INTEGRATION=1")
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
	config, _ := filepath.Abs("../../../deploy/mosquitto/mosquitto.conf")
	free := func() string {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		a := l.Addr().String()
		_ = l.Close()
		return a
	}
	broker := free()
	name := fmt.Sprintf("w3-crash-%d", time.Now().UnixNano())
	out, err := exec.Command("docker", "run", "-d", "--name", name, "-p", broker+":1883", "-v", config+":/mosquitto/config/mosquitto.conf:ro", "eclipse-mosquitto:2.0.21").CombinedOutput()
	if err != nil {
		t.Fatalf("broker: %s %v", out, err)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	addr := free()
	start := func(args ...string) *exec.Cmd {
		cmd := exec.Command(binary, args...)
		cmd.Env = append(os.Environ(), "TWIN_SCHEMA_PATH="+schema)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if cmd.ProcessState == nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
		})
		return cmd
	}
	wait := func(path string, test func(map[string]any) bool) map[string]any {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			resp, err := http.Get("http://" + addr + path)
			if err == nil {
				var v map[string]any
				err = json.NewDecoder(resp.Body).Decode(&v)
				_ = resp.Body.Close()
				if err == nil && test(v) {
					return v
				}
			}
			time.Sleep(30 * time.Millisecond)
		}
		t.Fatalf("timeout %s", path)
		return nil
	}
	first := start("--listen", addr, "--run-id", "r", "--mqtt-url", "tcp://"+broker, "--expect-fresh-run")
	wait("/api/health", func(v map[string]any) bool { return v["subscribed"] == true })
	pub := mqtt.NewClient(mqtt.NewClientOptions().AddBroker("tcp://" + broker).SetClientID("crash-test"))
	tok := pub.Connect()
	tok.Wait()
	if tok.Error() != nil {
		t.Fatal(tok.Error())
	}
	defer pub.Disconnect(100)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for i := 0; i < 10 && scanner.Scan(); i++ {
		var e struct {
			RunID     string  `json:"runId"`
			MachineID *string `json:"machineId"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		topic := "demo/smt/r/line/events"
		if e.MachineID != nil {
			topic = "demo/smt/r/machines/" + *e.MachineID + "/events"
		}
		tok = pub.Publish(topic, 1, false, append([]byte(nil), scanner.Bytes()...))
		tok.Wait()
		if tok.Error() != nil {
			t.Fatal(tok.Error())
		}
	}
	old := wait("/api/runs/r/snapshot", func(v map[string]any) bool { return v["lastSequence"] == float64(10) })
	if old["integrity"] != "complete" {
		t.Fatal(old["integrity"])
	}
	_ = first.Process.Kill()
	_ = first.Wait()
	restarted := start("--listen", addr, "--run-id", "r", "--mqtt-url", "tcp://"+broker)
	snap := wait("/api/runs/r/snapshot", func(v map[string]any) bool { return v["epoch"] != old["epoch"] })
	if snap["integrity"] != "uncertain" || snap["lastSequence"] != float64(0) {
		t.Fatalf("restart: %+v", snap)
	}
	_ = restarted.Process.Kill()
	_ = restarted.Wait()
	restored := start("--listen", addr, "--run-id", "r", "--import", logPath)
	recovered := wait("/api/runs/r/snapshot", func(v map[string]any) bool { return v["integrity"] == "complete" })
	if len(recovered["boards"].(map[string]any)) != 100 {
		t.Fatal("wrong board count")
	}
	_ = restored.Process.Kill()
	_ = restored.Wait()
}
