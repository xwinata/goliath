package database

import (
	"context"
	"database/sql"
)

type Database interface {
	// returns the name of the database
	Name() string

	// returns the SQL dialect spoken by the database
	Dialect() Dialect

	// returns the underlying database connection pool
	Connection() (*sql.DB, error)
}

// Executor abstracts the query-execution methods that are common to both
// *sql.DB and *sql.Tx, so callers can operate against either a connection pool
// or an in-progress transaction interchangeably.
type Executor interface {
	Exec(query string, args ...any) (sql.Result, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)

	Query(query string, args ...any) (*sql.Rows, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)

	QueryRow(query string, args ...any) *sql.Row
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row

	Prepare(query string) (*sql.Stmt, error)
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
}
