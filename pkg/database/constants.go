package database

// Dialect identifies the SQL dialect a Database speaks.
type Dialect string

const (
	DialectPostgres  Dialect = "postgres"
	DialectMySQL     Dialect = "mysql"
	DialectSQLite    Dialect = "sqlite"
	DialectMSSQL     Dialect = "mssql"
	DialectSQLServer Dialect = "sqlserver"
)
