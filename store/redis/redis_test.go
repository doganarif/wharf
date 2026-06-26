package redis_test

import (
	"context"
	"os"
	"testing"

	rstore "wharf/store/redis"
	"wharf/storetest"
)

// Set WHARF_REDIS_TEST_ADDR (e.g. redis://localhost:6379) to run this against a
// real Redis. It is skipped otherwise.
func TestRedisConformance(t *testing.T) {
	addr := os.Getenv("WHARF_REDIS_TEST_ADDR")
	if addr == "" {
		t.Skip("set WHARF_REDIS_TEST_ADDR to run the Redis conformance test")
	}
	ctx := context.Background()
	st, err := rstore.Open(ctx, addr)
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
