package wharf

import (
	"testing"

	"golang.org/x/time/rate"
)

func TestKeyGuardMaxSessions(t *testing.T) {
	g := newKeyGuard()
	g.maxSess = 2

	if ok, _ := g.admit("k"); !ok {
		t.Fatal("first session should be admitted")
	}
	if ok, _ := g.admit("k"); !ok {
		t.Fatal("second session should be admitted")
	}
	if ok, reason := g.admit("k"); ok || reason == "" {
		t.Fatalf("third session should be rejected, got ok=%v reason=%q", ok, reason)
	}
	// A different key is unaffected.
	if ok, _ := g.admit("other"); !ok {
		t.Fatal("different key should be admitted")
	}
	// Releasing frees a slot.
	g.release("k")
	if ok, _ := g.admit("k"); !ok {
		t.Fatal("slot should be free after release")
	}
	// k has 2 active, "other" has 1 → 3 total.
	if total := g.activeTotal(); total != 3 {
		t.Fatalf("activeTotal = %d, want 3", total)
	}
}

func TestKeyGuardRate(t *testing.T) {
	g := newKeyGuard()
	g.rate = rate.Limit(1) // 1/sec
	g.burst = 2

	if ok, _ := g.admit("k"); !ok {
		t.Fatal("burst 1 should pass")
	}
	if ok, _ := g.admit("k"); !ok {
		t.Fatal("burst 2 should pass")
	}
	if ok, reason := g.admit("k"); ok || reason == "" {
		t.Fatalf("third connect should be rate-limited, got ok=%v", ok)
	}
}
