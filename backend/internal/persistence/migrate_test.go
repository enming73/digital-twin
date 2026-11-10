package persistence

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func testDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("W6_PG_INTEGRATION") != "1" {
		t.Skip("set W6_PG_INTEGRATION=1 to run isolated PostgreSQL tests")
	}
	name := fmt.Sprintf("w6-migrate-test-%d", time.Now().UnixNano())
	cmd := exec.Command("docker", "run", "-d", "--rm", "--name", name, "-e", "POSTGRES_PASSWORD=test-only", "-p", "127.0.0.1::5432", "postgres:16")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("start isolated postgres: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "stop", name).Run() })
	out, err := exec.Command("docker", "port", name, "5432/tcp").Output()
	if err != nil {
		t.Fatal(err)
	}
	url := "postgres://postgres:test-only@" + strings.TrimSpace(string(out)) + "/postgres?sslmode=disable"
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	deadline := time.Now().Add(30 * time.Second)
	for {
		if err = pool.Ping(ctx); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("postgres not ready: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return pool
}

func TestMigration(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("second migration: %v", err)
	}
	_, err := db.Exec(ctx, `INSERT INTO runs (run_id, stations, integrity, last_sequence) VALUES ('r', '["loader"]', 'uncertain', 0)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO events (run_id,event_id,sequence,schema_version,sim_time_ms,emitted_at,ingested_at,envelope) VALUES ('r','e',1,1,0,now(),now(),'{}')`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO events (run_id,event_id,sequence,schema_version,sim_time_ms,emitted_at,ingested_at,envelope) VALUES ('r','e',2,1,0,now(),now(),'{}')`); err == nil {
		t.Fatal("duplicate event ID accepted")
	}
	if _, err = db.Exec(ctx, `INSERT INTO events (run_id,event_id,sequence,schema_version,sim_time_ms,emitted_at,ingested_at,envelope) VALUES ('r','other',1,1,0,now(),now(),'{}')`); err == nil {
		t.Fatal("duplicate sequence accepted")
	}
	if _, err = db.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES (999)`); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, db); err == nil {
		t.Fatal("newer migration version accepted")
	}
}
