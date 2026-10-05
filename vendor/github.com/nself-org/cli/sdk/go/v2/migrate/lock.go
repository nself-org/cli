package migrate

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// lockConn opens one dedicated connection (not taken from the pool, so a
// pool limited to one connection cannot deadlock the Between callback) and
// takes the session-level advisory lock for this plugin on it. The wait is
// bounded by ctx. The lock is released when the connection is closed, which
// also happens if the process dies, so a crash can never strand it.
func lockConn(ctx context.Context, pool *pgxpool.Pool, key string) (*pgx.Conn, error) {
	cfg := pool.Config().ConnConfig.Copy()
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("migrate: connect for advisory lock: %w", err)
	}
	// A role or database default lock_timeout / statement_timeout would cut a
	// legitimate wait behind another replica short. The wait is bounded by
	// ctx instead; every file transaction sets its own limits.
	if _, err := conn.Exec(ctx, `SELECT set_config('lock_timeout', '0', false), set_config('statement_timeout', '0', false)`); err != nil {
		closeConn(conn)
		return nil, fmt.Errorf("migrate: prepare lock connection: %w", err)
	}
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock(hashtext($1))`, key); err != nil {
		closeConn(conn)
		return nil, fmt.Errorf("migrate: advisory lock %s: %w", key, err)
	}
	return conn, nil
}

// closeConn closes conn with a fresh context: the caller's may be cancelled
// on the error path, and the lock must still go.
func closeConn(conn *pgx.Conn) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = conn.Close(ctx)
}
