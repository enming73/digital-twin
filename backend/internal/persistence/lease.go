package persistence

import (
	"context"
	"errors"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
)

// WriterLease is an advisory session lock. A caller must additionally stop
// writes if its connection is lost; this primitive alone cannot enforce that.
type WriterLease struct {
	mu    sync.Mutex
	conn  *pgxpool.Conn
	runID string
}

func AcquireRunWriter(ctx context.Context, db *pgxpool.Pool, runID string) (*WriterLease, error) {
	if runID == "" {
		return nil, errors.New("empty run ID")
	}
	conn, err := db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	var acquired bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended($1, 7346182601))`, runID).Scan(&acquired); err != nil {
		conn.Release()
		return nil, err
	}
	if !acquired {
		conn.Release()
		return nil, errors.New("run already has a writer")
	}
	return &WriterLease{conn: conn, runID: runID}, nil
}

// Ping checks the actual lock-owning session, not merely another pooled connection.
func (l *WriterLease) Ping(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return errors.New("writer lease unavailable")
	}
	return l.conn.Ping(ctx)
}

func (l *WriterLease) Close(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return nil
	}
	conn := l.conn
	l.conn = nil
	var unlocked bool
	err := conn.QueryRow(ctx, `SELECT pg_advisory_unlock(hashtextextended($1, 7346182601))`, l.runID).Scan(&unlocked)
	if err != nil || !unlocked {
		raw := conn.Hijack()
		_ = raw.Close(context.Background())
		if err != nil {
			return err
		}
		return errors.New("writer lock was lost")
	}
	conn.Release()
	return nil
}
