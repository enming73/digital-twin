package outbox

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestAtomicCrashHelper(t *testing.T) {
	path := os.Getenv("OUTBOX_CRASH_PATH")
	if path == "" {
		t.Skip("subprocess helper")
	}
	s, err := Open(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO events(run_id,sequence,event_id,envelope) VALUES('run',1,'one',x'7b7d')`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO runs(run_id,head,boundary_start,state) VALUES('run',1,1,x'01')`); err != nil {
		t.Fatal(err)
	}
	fmt.Println("UNCOMMITTED")
	select {}
}

func TestAtomicCrashRollsBackBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.db")
	s, err := Open(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestAtomicCrashHelper$")
	cmd.Env = append(os.Environ(), "OUTBOX_CRASH_PATH="+path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	lines := make(chan string, 1)
	go func() {
		scan := bufio.NewScanner(stdout)
		if scan.Scan() {
			lines <- scan.Text()
		} else {
			lines <- ""
		}
	}()
	select {
	case line := <-lines:
		if line != "UNCOMMITTED" {
			t.Fatalf("helper: %q", line)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("helper timed out")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	s, err = Open(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	page, err := s.ReadPage(context.Background(), "run", 0, 500)
	if err != nil || page.Head != 0 || len(page.Events) != 0 {
		t.Fatalf("partial transaction survived: %+v %v", page, err)
	}
}
