package outbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestStorePersistsCommittedBoundary(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "outbox.db")
	s, err := Open(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	one := []byte(`{"runId":"run","eventId":"one","sequence":1}`)
	two := []byte(`{"runId":"run","eventId":"two","sequence":2}`)
	if err := s.CommitBoundary(ctx, "run", [][]byte{one, two}, []byte("state-2"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	page, err := s.ReadPage(ctx, "run", 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	if page.Head != 2 || len(page.Events) != 2 || !bytes.Equal(page.Events[0], one) || !bytes.Equal(page.Events[1], two) {
		t.Fatalf("page: %+v", page)
	}
	state, head, err := s.ReadState(ctx, "run")
	if err != nil || head != 2 || !bytes.Equal(state, []byte("state-2")) {
		t.Fatalf("state=%q head=%d err=%v", state, head, err)
	}
}

func TestAtomicBoundaryRejectsGapWithoutPartialWrite(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "outbox.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	one := []byte(`{"runId":"run","eventId":"one","sequence":1}`)
	three := []byte(`{"runId":"run","eventId":"three","sequence":3}`)
	if err := s.CommitBoundary(ctx, "run", [][]byte{one, three}, []byte("state"), nil); err == nil {
		t.Fatal("gap accepted")
	}
	page, err := s.ReadPage(ctx, "run", 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	if page.Head != 0 || len(page.Events) != 0 {
		t.Fatalf("partial boundary visible: %+v", page)
	}
}

func TestQuotaRejectsEntireBoundary(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "outbox.db"), 64)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	event := []byte(`{"runId":"run","eventId":"one","sequence":1}`)
	if err := s.CommitBoundary(ctx, "run", [][]byte{event}, bytes.Repeat([]byte("x"), 100), nil); !errors.Is(err, ErrCapacity) {
		t.Fatalf("expected capacity error, got %v", err)
	}
	page, err := s.ReadPage(ctx, "run", 0, 500)
	if err != nil || page.Head != 0 {
		t.Fatalf("quota changed head: %+v %v", page, err)
	}
}

func TestStoreRetriesIdenticalBoundaryWithoutDuplication(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "outbox.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	one := []byte(`{"runId":"run","eventId":"one","sequence":1}`)
	if err := s.CommitBoundary(ctx, "run", [][]byte{one}, []byte("state"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitBoundary(ctx, "run", [][]byte{one}, []byte("state"), nil); err != nil {
		t.Fatalf("identical retry: %v", err)
	}
	page, err := s.ReadPage(ctx, "run", 0, 500)
	if err != nil || page.Head != 1 || len(page.Events) != 1 {
		t.Fatalf("duplicate: %+v %v", page, err)
	}
	changed := []byte(`{"runId":"run","eventId":"one","sequence":1,"changed":true}`)
	if err := s.CommitBoundary(ctx, "run", [][]byte{changed}, []byte("state"), nil); err == nil {
		t.Fatal("conflicting retry accepted")
	}
}

func TestStoreRejectsPartialBoundaryRetry(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "outbox.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	one := []byte(`{"runId":"run","eventId":"one","sequence":1}`)
	two := []byte(`{"runId":"run","eventId":"two","sequence":2}`)
	if err := s.CommitBoundary(ctx, "run", [][]byte{one, two}, []byte("state"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitBoundary(ctx, "run", [][]byte{two}, []byte("state"), nil); err == nil {
		t.Fatal("partial boundary retry accepted")
	}
}

func TestStoreRejectsDuplicateEventIDWithoutAdvancingHead(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "outbox.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	one := []byte(`{"runId":"run","eventId":"one","sequence":1}`)
	two := []byte(`{"runId":"run","eventId":"one","sequence":2}`)
	if err := s.CommitBoundary(ctx, "run", [][]byte{one}, []byte("state1"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitBoundary(ctx, "run", [][]byte{two}, []byte("state2"), nil); err == nil || errors.Is(err, ErrStorageUnavailable) {
		t.Fatalf("expected conflict, got %v", err)
	}
	state, head, err := s.ReadState(ctx, "run")
	if err != nil || head != 1 || string(state) != "state1" {
		t.Fatalf("head=%d state=%q err=%v", head, state, err)
	}
}

func TestStoreConfiguresPhysicalCapacityBound(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "outbox.db"), 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var maxPages, pageSize, journalLimit int64
	if err := s.db.QueryRow(`PRAGMA max_page_count`).Scan(&maxPages); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`PRAGMA page_size`).Scan(&pageSize); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`PRAGMA journal_size_limit`).Scan(&journalLimit); err != nil {
		t.Fatal(err)
	}
	if pageSize != 4096 {
		t.Fatalf("unexpected page size %d", pageSize)
	}
	wantPages := (int64(64<<10) + schemaReserve) / pageSize
	if maxPages != wantPages {
		t.Fatalf("max_page_count=%d want %d", maxPages, wantPages)
	}
	if journalLimit != 64<<10 {
		t.Fatalf("journal_size_limit=%d want %d", journalLimit, int64(64<<10))
	}
}

func TestStoreKeepsFilesWithinCapacityBound(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	const quota = 256 << 10
	s, err := Open(filepath.Join(dir, "outbox.db"), quota)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	payload := bytes.Repeat([]byte("x"), 8<<10)
	for i := 1; i <= 1000; i++ {
		event := []byte(fmt.Sprintf(`{"runId":"run","eventId":"e%d","sequence":%d,"data":"%s"}`, i, i, payload))
		err := s.CommitBoundary(ctx, "run", [][]byte{event}, []byte("state"), nil)
		if errors.Is(err, ErrCapacity) || errors.Is(err, ErrStorageUnavailable) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	var total int64
	for _, name := range []string{"outbox.db", "outbox.db-wal"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		total += info.Size()
	}
	if limit := int64(quota) + schemaReserve + 64<<10; total > limit {
		t.Fatalf("durable files total %d bytes, exceed bound %d", total, limit)
	}
	page, err := s.ReadPage(ctx, "run", 0, 500)
	if err != nil || page.Head == 0 || len(page.Events) == 0 {
		t.Fatalf("committed prefix unreadable: head=%d events=%d err=%v", page.Head, len(page.Events), err)
	}
}

func TestStoreDiskFullPreservesPrefix(t *testing.T) {
	dir := os.Getenv("OUTBOX_DISK_FULL_DIR")
	if dir == "" {
		t.Skip("set OUTBOX_DISK_FULL_DIR to a size-limited filesystem to run")
	}
	ctx := context.Background()
	s, err := Open(filepath.Join(dir, "outbox.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	payload := bytes.Repeat([]byte("x"), 32<<10)
	var lastHead int64
	var failure error
	for i := 1; i <= 1000; i++ {
		event := []byte(fmt.Sprintf(`{"runId":"run","eventId":"e%d","sequence":%d,"data":"%s"}`, i, i, payload))
		if err := s.CommitBoundary(ctx, "run", [][]byte{event}, bytes.Repeat([]byte("s"), 8<<10), nil); err != nil {
			failure = err
			break
		}
		lastHead = int64(i)
	}
	if failure == nil {
		t.Skip("filesystem was not filled; increase OUTBOX_DISK_FULL_ITERATIONS or shrink the volume")
	}
	if errors.Is(failure, ErrCapacity) {
		t.Fatalf("expected physical storage failure, got capacity error")
	}
	if !errors.Is(failure, ErrStorageUnavailable) {
		t.Fatalf("expected ErrStorageUnavailable, got %v", failure)
	}
	page, err := s.ReadPage(ctx, "run", 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	if page.Head != lastHead || int64(len(page.Events)) > page.Head {
		t.Fatalf("prefix inconsistent after disk full: head=%d lastHead=%d events=%d", page.Head, lastHead, len(page.Events))
	}
	if page.Head == 0 {
		t.Fatal("no boundary committed before disk filled")
	}
}

func TestStoreRejectsUnknownSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox.db")
	s, err := Open(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`PRAGMA user_version=3`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, 0)
	if err == nil {
		s.Close()
		t.Fatal("unknown schema accepted")
	}
}

func TestStoreRejectsInvalidTerminalDigest(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "outbox.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	event := []byte(`{"runId":"run","eventId":"one","sequence":1}`)
	terminal := &Terminal{RunID: "run", FinalSequence: 1, EventCount: 1, SHA256: "wrong"}
	if err := s.CommitBoundary(ctx, "run", [][]byte{event}, []byte("state"), terminal); err == nil {
		t.Fatal("invalid digest accepted")
	}
	page, err := s.ReadPage(ctx, "run", 0, 500)
	if err != nil || page.Head != 0 {
		t.Fatalf("terminal failure leaked event: %+v %v", page, err)
	}
}

func TestStoreReportsClosedStorageUnavailable(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "outbox.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	event := []byte(`{"runId":"run","eventId":"one","sequence":1}`)
	if err := s.CommitBoundary(ctx, "run", [][]byte{event}, []byte("state"), nil); !errors.Is(err, ErrStorageUnavailable) {
		t.Fatalf("expected unavailable, got %v", err)
	}
}

func TestStoreReportsWriteFailureAsUnavailable(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "outbox.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.ExecContext(ctx, `PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	event := []byte(`{"runId":"run","eventId":"one","sequence":1}`)
	if err := s.CommitBoundary(ctx, "run", [][]byte{event}, []byte("state"), nil); !errors.Is(err, ErrStorageUnavailable) {
		t.Fatalf("write failure should be unavailable: %v", err)
	}
	page, err := s.ReadPage(ctx, "run", 0, 500)
	if err != nil || page.Head != 0 || len(page.Events) != 0 {
		t.Fatalf("write failure exposed boundary: %+v %v", page, err)
	}
}

func TestStoreReportsSQLiteFullWithoutPartialBoundary(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "outbox.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var pages int
	if err := s.db.QueryRowContext(ctx, `PRAGMA page_count`).Scan(&pages); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`PRAGMA max_page_count=%d`, pages)); err != nil {
		t.Fatal(err)
	}
	event := []byte(fmt.Sprintf(`{"runId":"run","eventId":"one","sequence":1,"data":"%s"}`, bytes.Repeat([]byte("x"), 100000)))
	if err := s.CommitBoundary(ctx, "run", [][]byte{event}, []byte("state"), nil); !errors.Is(err, ErrStorageUnavailable) {
		t.Fatalf("expected unavailable for SQLite full, got %v", err)
	}
	page, err := s.ReadPage(ctx, "run", 0, 500)
	if err != nil || page.Head != 0 || len(page.Events) != 0 {
		t.Fatalf("full database exposed boundary: %+v %v", page, err)
	}
}

func TestStoreRetryReportsReadFailureAsUnavailable(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "outbox.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	event := []byte(`{"runId":"run","eventId":"one","sequence":1}`)
	if err := s.CommitBoundary(ctx, "run", [][]byte{event}, []byte("state"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `DROP TABLE events`); err != nil {
		t.Fatal(err)
	}
	if err := s.CommitBoundary(ctx, "run", [][]byte{event}, []byte("state"), nil); !errors.Is(err, ErrStorageUnavailable) {
		t.Fatalf("retry read failure should be unavailable: %v", err)
	}
}

func TestDigestIsOrderedAndLengthDelimited(t *testing.T) {
	a := []byte(`{"eventId":"a"}`)
	b := []byte(`{"eventId":"b"}`)
	if Digest([][]byte{a, b}) == Digest([][]byte{b, a}) {
		t.Fatal("digest ignores event order")
	}
	// Splitting the same bytes across two envelopes must change the digest.
	if Digest([][]byte{[]byte("ab")}) == Digest([][]byte{[]byte("a"), []byte("b")}) {
		t.Fatal("digest ignores envelope boundaries")
	}
	want := "c9a36242823ea592ee3ecb595d234870ae9d610406896ca2af938e1d93badf14"
	if got := Digest([][]byte{a}); got != want {
		t.Fatalf("digest=%s want %s", got, want)
	}
}

func TestStoreComputesTerminalDigestWhenOmitted(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "outbox.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	one := []byte(`{"runId":"run","eventId":"one","sequence":1}`)
	two := []byte(`{"runId":"run","eventId":"two","sequence":2}`)
	terminal := &Terminal{RunID: "run", FinalSequence: 2, EventCount: 2}
	if err := s.CommitBoundary(ctx, "run", [][]byte{one, two}, []byte("state"), terminal); err != nil {
		t.Fatal(err)
	}
	page, err := s.ReadPage(ctx, "run", 0, 500)
	if err != nil {
		t.Fatal(err)
	}
	if page.Terminal == nil || page.Terminal.SHA256 != Digest([][]byte{one, two}) {
		t.Fatalf("store did not persist a verifiable digest: %+v", page.Terminal)
	}
	retry := &Terminal{RunID: "run", FinalSequence: 2, EventCount: 2}
	if err := s.CommitBoundary(ctx, "run", [][]byte{one, two}, []byte("state"), retry); err != nil {
		t.Fatalf("identical terminal retry: %v", err)
	}
}

func TestStorePersistsVerifiedTerminal(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "outbox.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	event := []byte(`{"runId":"run","eventId":"one","sequence":1}`)
	h := sha256.New()
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(event)))
	_, _ = h.Write(length[:])
	_, _ = h.Write(event)
	terminal := &Terminal{RunID: "run", FinalSequence: 1, EventCount: 1, SHA256: hex.EncodeToString(h.Sum(nil))}
	if err := s.CommitBoundary(ctx, "run", [][]byte{event}, []byte("state"), terminal); err != nil {
		t.Fatal(err)
	}
	page, err := s.ReadPage(ctx, "run", 1, 1)
	if err != nil || page.Terminal == nil || *page.Terminal != *terminal || page.Head != 1 {
		t.Fatalf("terminal: %+v %v", page, err)
	}
	second := []byte(`{"runId":"run","eventId":"two","sequence":2}`)
	if err := s.CommitBoundary(ctx, "run", [][]byte{second}, []byte("state2"), nil); err == nil {
		t.Fatal("completed run extended")
	}
}

func TestStoreConcurrentPageNeverExceedsReportedHead(t *testing.T) {
	ctx := context.Background()
	s, err := Open(filepath.Join(t.TempDir(), "outbox.db"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 1; i <= 100; i++ {
			event := []byte(fmt.Sprintf(`{"runId":"run","eventId":"e%d","sequence":%d}`, i, i))
			if err := s.CommitBoundary(ctx, "run", [][]byte{event}, []byte("state"), nil); err != nil {
				errs <- err
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 400; i++ {
			page, err := s.ReadPage(ctx, "run", 0, 500)
			if err != nil {
				errs <- err
				return
			}
			if int64(len(page.Events)) > page.Head {
				errs <- fmt.Errorf("%d events beyond head %d", len(page.Events), page.Head)
				return
			}
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
