package postgres_test

import (
	"context"
	"os"
	"testing"

	"wharf/store/postgres"
	"wharf/storetest"
)

// Set WHARF_PG_TEST_DSN (e.g. postgres://wharf:wharf@localhost:5432/wharf?sslmode=disable)
// to run this against a real Postgres. It is skipped otherwise.
func TestPostgresConformance(t *testing.T) {
	dsn := os.Getenv("WHARF_PG_TEST_DSN")
	if dsn == "" {
		t.Skip("set WHARF_PG_TEST_DSN to run the Postgres conformance test")
	}
	ctx := context.Background()
	st, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// The conformance suite expects an empty store.
	for _, ns := range []string{"ns", "other", "empty"} {
		if e, _ := st.List(ctx, ns); len(e) > 0 {
			for _, kv := range e {
				_ = st.Delete(ctx, ns, kv.Key)
			}
		}
	}
	storetest.Run(t, st)
}
