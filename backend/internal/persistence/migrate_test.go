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
	"smt-digital-twin/backend/migrations"
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
	var registered, schemaVersion int
	if err = db.QueryRow(ctx, `SELECT COUNT(*) FROM assets WHERE representation = 'glb' AND model_url = '/models/' || machine_id || '.glb'`).Scan(&registered); err != nil {
		t.Fatal(err)
	}
	if registered != 7 {
		t.Fatalf("asset registry rows: %d", registered)
	}
	if err = db.QueryRow(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&schemaVersion); err != nil {
		t.Fatal(err)
	}
	if schemaVersion != 2 {
		t.Fatalf("schema version: %d", schemaVersion)
	}
	if _, err = db.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES (999)`); err != nil {
		t.Fatal(err)
	}
	if err = Migrate(ctx, db); err == nil {
		t.Fatal("newer migration version accepted")
	}
}

func TestMigrationUpgradesVersionOne(t *testing.T) {
	ctx := context.Background()
	db := testDatabase(t)
	if _, err := db.Exec(ctx, migrations.Initial); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version integer PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	var version, registered int
	if err := db.QueryRow(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 2 {
		t.Fatalf("upgraded version: %d", version)
	}
	if err := db.QueryRow(ctx, `SELECT COUNT(*) FROM assets WHERE representation = 'glb'`).Scan(&registered); err != nil {
		t.Fatal(err)
	}
	if registered != 7 {
		t.Fatalf("upgraded asset rows: %d", registered)
	}
}
