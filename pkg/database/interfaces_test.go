package database

import "database/sql"

// Compile-time assertions that the standard library connection and transaction
// types satisfy Executor. If these ever break, the Executor method set has
// drifted from the *sql.DB / *sql.Tx intersection.
var (
	_ Executor = (*sql.DB)(nil)
	_ Executor = (*sql.Tx)(nil)
)

// Compile-time assertion that the unexported implementation satisfies the
// public Database interface.
var _ Database = (*database)(nil)
