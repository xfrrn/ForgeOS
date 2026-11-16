package testdb

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/lib/pq"
)

// Open creates a disposable schema in the configured PostgreSQL test database.
func Open(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("FORGEOS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set FORGEOS_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	target, err := url.Parse(dsn)
	if err != nil || (target.Scheme != "postgres" && target.Scheme != "postgresql") {
		t.Fatal("FORGEOS_TEST_DATABASE_URL must be a postgres URL")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	var id [12]byte
	if _, err := rand.Read(id[:]); err != nil {
		t.Fatal(err)
	}
	schema := "forgeos_test_" + hex.EncodeToString(id[:])
	if _, err = admin.Exec("CREATE SCHEMA " + pq.QuoteIdentifier(schema)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec("DROP SCHEMA " + pq.QuoteIdentifier(schema) + " CASCADE"); err != nil {
			t.Error(err)
		}
	})
	q := target.Query()
	q.Set("search_path", schema)
	target.RawQuery = q.Encode()
	db, err := sql.Open("postgres", target.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
