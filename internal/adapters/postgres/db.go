package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DBTX is satisfied by both *pgxpool.Pool and pgx.Tx, letting a repository
// run against either a plain pooled connection or an explicit transaction
// without any change to the repository's own code. cmd/import uses this to
// wrap a whole import run in one transaction instead of autocommitting each
// write, so a failure partway through rolls back cleanly instead of leaving
// Postgres with half-imported places and orphaned scripts.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}
