package database

import (
	"database/sql"
	"database/sql/driver"
	"strings"
	"testing"
)

func TestNew_Accessors(t *testing.T) {
	db := New(Config{
		Name:    "primary",
		Dialect: DialectPostgres,
	})

	if got := db.Name(); got != "primary" {
		t.Errorf("Name() = %q, want primary", got)
	}
	if got := db.Dialect(); got != DialectPostgres {
		t.Errorf("Dialect() = %q, want %q", got, DialectPostgres)
	}
}

func TestDSN(t *testing.T) {
	base := Config{
		Name:     "db",
		Username: "user",
		Password: "p@ss word", // contains chars that must be escaped in URLs
		Host:     "localhost",
		Port:     5432,
		Database: "app",
	}

	cases := []struct {
		name       string
		dialect    Dialect
		wantDriver string
		wantParts  []string // substrings the DSN must contain
	}{
		{
			name:       "postgres",
			dialect:    DialectPostgres,
			wantDriver: "postgres",
			wantParts:  []string{"postgres://", "user", "localhost:5432", "/app"},
		},
		{
			name:       "mysql",
			dialect:    DialectMySQL,
			wantDriver: "mysql",
			wantParts:  []string{"user:", "@tcp(localhost:5432)/app"},
		},
		{
			name:       "mssql",
			dialect:    DialectMSSQL,
			wantDriver: "sqlserver",
			wantParts:  []string{"sqlserver://", "localhost:5432", "database=app"},
		},
		{
			name:       "sqlserver",
			dialect:    DialectSQLServer,
			wantDriver: "sqlserver",
			wantParts:  []string{"sqlserver://", "database=app"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			cfg.Dialect = tc.dialect
			driver, dsn, err := cfg.dsn()
			if err != nil {
				t.Fatalf("dsn() error: %v", err)
			}
			if driver != tc.wantDriver {
				t.Errorf("driver = %q, want %q", driver, tc.wantDriver)
			}
			for _, part := range tc.wantParts {
				if !strings.Contains(dsn, part) {
					t.Errorf("dsn %q missing %q", dsn, part)
				}
			}
		})
	}
}

func TestDSN_PostgresEscapesCredentials(t *testing.T) {
	cfg := Config{
		Dialect:  DialectPostgres,
		Username: "u",
		Password: "p@ss word",
		Host:     "h",
		Port:     1,
		Database: "d",
	}
	_, dsn, err := cfg.dsn()
	if err != nil {
		t.Fatal(err)
	}
	// The raw password (with space and @) must not appear unescaped.
	if strings.Contains(dsn, "p@ss word") {
		t.Errorf("password not escaped in DSN: %q", dsn)
	}
}

func TestDSN_SQLiteUsesDatabaseAsPath(t *testing.T) {
	cfg := Config{Dialect: DialectSQLite, Database: "/tmp/app.db"}
	driver, dsn, err := cfg.dsn()
	if err != nil {
		t.Fatal(err)
	}
	if driver != "sqlite" {
		t.Errorf("driver = %q, want sqlite", driver)
	}
	if dsn != "/tmp/app.db" {
		t.Errorf("dsn = %q, want /tmp/app.db", dsn)
	}
}

func TestDSN_Errors(t *testing.T) {
	t.Run("empty dialect", func(t *testing.T) {
		cfg := Config{Name: "x"}
		if _, _, err := cfg.dsn(); err == nil {
			t.Fatal("expected error for empty dialect")
		} else if !strings.Contains(err.Error(), "dialect is required") {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("unsupported dialect", func(t *testing.T) {
		cfg := Config{Name: "x", Dialect: Dialect("oracle")}
		if _, _, err := cfg.dsn(); err == nil {
			t.Fatal("expected error for unsupported dialect")
		} else if !strings.Contains(err.Error(), "unsupported dialect") {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

func TestConnection_EmptyDialectReturnsError(t *testing.T) {
	db := New(Config{Name: "x"}) // no dialect
	conn, err := db.Connection()
	if err == nil {
		t.Fatal("expected error when dialect is empty")
	}
	if conn != nil {
		t.Errorf("expected nil connection on error, got %v", conn)
	}
}

// TestConnection_MemoizesResult verifies that Connection() runs its open logic
// once (via sync.Once) and returns the same cached result on subsequent calls.
// We use the empty-dialect error path because it is deterministic and does not
// require a registered SQL driver.
func TestConnection_MemoizesResult(t *testing.T) {
	d := &database{cfg: Config{Name: "x"}} // empty dialect -> dsn() error

	c1, e1 := d.Connection()
	c2, e2 := d.Connection()

	if e1 == nil || e2 == nil {
		t.Fatal("expected an error from Connection with empty dialect")
	}
	if e1 != e2 {
		t.Errorf("expected the same cached error on repeat calls, got %v then %v", e1, e2)
	}
	if c1 != nil || c2 != nil {
		t.Errorf("expected nil connection on the error path")
	}
}

// fakeDriver is a no-op driver.Driver registered so we can exercise sql.Open,
// which Connection() calls internally. Without a registered driver, sql.Open
// fails immediately regardless of the DSN.
type fakeDriver struct{}

func (fakeDriver) Open(string) (driver.Conn, error) { return nil, nil }

func init() {
	sql.Register("goliath-fake", fakeDriver{})
}

// TestConnection_OpensAndCachesWithDriver drives Connection() through a
// successful sql.Open by using a database whose dsn() resolves to the registered
// fake driver, then confirms the returned *sql.DB is the same cached instance.
func TestConnection_OpensAndCachesWithDriver(t *testing.T) {
	d := &database{dsnFunc: func() (string, string, error) {
		return "goliath-fake", "", nil
	}}

	db1, err := d.Connection()
	if err != nil {
		t.Fatalf("Connection() error: %v", err)
	}
	if db1 == nil {
		t.Fatal("expected a non-nil *sql.DB")
	}
	t.Cleanup(func() { _ = db1.Close() })

	db2, err := d.Connection()
	if err != nil {
		t.Fatalf("second Connection() error: %v", err)
	}
	if db1 != db2 {
		t.Errorf("expected the same cached *sql.DB, got two instances")
	}
}
