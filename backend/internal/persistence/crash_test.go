package persistence

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"smt-digital-twin/backend/internal/contract"
	"smt-digital-twin/backend/internal/twin"
)

type crashAfterCommitJournal struct {
	inner  *Journal
	before bool
}

func (j crashAfterCommitJournal) SetIntegrity(run, status, reason string) error {
	return j.inner.SetIntegrity(run, status, reason)
}
func (j crashAfterCommitJournal) Commit(e contract.Event, at time.Time, s twin.Snapshot, trace []twin.TraceEntry, cp *twin.Checkpoint) error {
	if j.before {
		fmt.Println("BEFORE_COMMIT")
		select {}
	}
	if err := j.inner.Commit(e, at, s, trace, cp); err != nil {
		return err
	}
	fmt.Println("COMMITTED")
	select {}
}

func TestCrashAfterCommitHelper(t *testing.T) {
	url := os.Getenv("W6_CRASH_TEST_URL")
	if url == "" {
		t.Skip("helper only")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lease, err := AcquireRunWriter(ctx, db, "r")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close(ctx)
	if os.Getenv("W6_CRASH_IN_TX") == "1" {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO events(run_id,event_id,sequence,schema_version,sim_time_ms,emitted_at,ingested_at,envelope) VALUES('r','one',1,1,0,now(),now(),'{}')`); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE runs SET last_sequence=1 WHERE run_id='r'`); err != nil {
			t.Fatal(err)
		}
		fmt.Println("IN_TRANSACTION")
		select {}
	}
	s := twin.NewStoreWithJournal("old", map[string][]string{"r": {"loader"}}, crashAfterCommitJournal{inner: NewLeasedJournal(lease), before: os.Getenv("W6_CRASH_BEFORE") == "1"})
	e := contract.Event{SchemaVersion: 1, RunID: "r", EventID: "one", Sequence: 1, EventType: "workorder.started", EmittedAt: "2026-11-09T00:00:00Z", Payload: map[string]any{"workOrderId": "wo", "quantity": float64(1)}}
	if _, err := s.Apply(e, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestCrashDuringTransactionRollsBack(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "r", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashAfterCommitHelper$")
	cmd.Env = append(os.Environ(), "W6_CRASH_TEST_URL="+db.Config().ConnString(), "W6_CRASH_IN_TX=1")
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
	line := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			line <- scanner.Text()
		} else {
			line <- ""
		}
	}()
	select {
	case value := <-line:
		if value != "IN_TRANSACTION" {
			t.Fatalf("helper output %q", value)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("helper did not reach transaction")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	lease, err := AcquireRunWriter(ctx, db, "r")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close(ctx)
	restored, err := RestoreWithLease(ctx, db, "r", []string{"loader"}, "new", lease)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := restored.Snapshot("r")
	if snapshot.LastSequence != 0 {
		t.Fatalf("transaction partially persisted: %+v", snapshot)
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM events WHERE run_id='r'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("uncommitted events persisted: %d", count)
	}
}

func TestCrashBeforeCommitLeavesNoEvent(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "r", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashAfterCommitHelper$")
	cmd.Env = append(os.Environ(), "W6_CRASH_TEST_URL="+db.Config().ConnString(), "W6_CRASH_BEFORE=1")
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
	line := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			line <- scanner.Text()
		} else {
			line <- ""
		}
	}()
	select {
	case value := <-line:
		if value != "BEFORE_COMMIT" {
			t.Fatalf("helper output %q", value)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("helper did not reach commit")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	lease, err := AcquireRunWriter(ctx, db, "r")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close(ctx)
	restored, err := RestoreWithLease(ctx, db, "r", []string{"loader"}, "new", lease)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := restored.Snapshot("r")
	if snapshot.LastSequence != 0 || len(snapshot.WorkOrders) != 0 {
		t.Fatalf("uncommitted snapshot %+v", snapshot)
	}
}

func TestCrashAfterCommitRestoresOnce(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := CreateRun(ctx, db, "r", []string{"loader"}); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashAfterCommitHelper$")
	cmd.Env = append(os.Environ(), "W6_CRASH_TEST_URL="+db.Config().ConnString())
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
	line := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			line <- scanner.Text()
		} else {
			line <- ""
		}
	}()
	select {
	case value := <-line:
		if value != "COMMITTED" {
			t.Fatalf("helper output %q", value)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("helper did not commit")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	lease, err := AcquireRunWriter(ctx, db, "r")
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close(ctx)
	restored, err := RestoreWithLease(ctx, db, "r", []string{"loader"}, "new", lease)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := restored.Snapshot("r")
	if snapshot.LastSequence != 1 || snapshot.WorkOrders["wo"].Quantity != 1 || snapshot.Integrity != "uncertain" {
		t.Fatalf("snapshot %+v", snapshot)
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM events WHERE run_id='r'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("stored %d events", count)
	}
}
