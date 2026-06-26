// Package storetest provides a conformance suite for wharf.Store backends.
// Each adapter runs it against a live store to prove identical semantics.
//
//	func TestSQLite(t *testing.T) {
//	    s, _ := sqlite.Open(filepath.Join(t.TempDir(), "t.db"))
//	    defer s.Close()
//	    storetest.Run(t, s)
//	}
package storetest

import (
	"bytes"
	"context"
	"testing"

	"wharf"
)

// Run exercises the full Store contract. The store should be empty.
func Run(t *testing.T, s wharf.Store) {
	t.Helper()
	ctx := context.Background()

	// Missing key → ok=false, no error.
	if _, ok, err := s.Get(ctx, "ns", "missing"); err != nil || ok {
		t.Fatalf("missing key: ok=%v err=%v, want ok=false err=nil", ok, err)
	}

	// Set then Get round-trips the value.
	if err := s.Set(ctx, "ns", "k", []byte("v1")); err != nil {
		t.Fatalf("set: %v", err)
	}
	if v, ok, err := s.Get(ctx, "ns", "k"); err != nil || !ok || !bytes.Equal(v, []byte("v1")) {
		t.Fatalf("get after set: v=%q ok=%v err=%v", v, ok, err)
	}

	// Set overwrites.
	if err := s.Set(ctx, "ns", "k", []byte("v2")); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	if v, _, _ := s.Get(ctx, "ns", "k"); !bytes.Equal(v, []byte("v2")) {
		t.Fatalf("overwrite value = %q, want v2", v)
	}

	// Namespaces are isolated.
	if err := s.Set(ctx, "other", "k", []byte("x")); err != nil {
		t.Fatalf("set other ns: %v", err)
	}
	if v, _, _ := s.Get(ctx, "ns", "k"); !bytes.Equal(v, []byte("v2")) {
		t.Fatalf("namespace leak: ns/k = %q", v)
	}

	// List returns a namespace's entries sorted by key.
	if err := s.Set(ctx, "ns", "a", []byte("1")); err != nil {
		t.Fatalf("set a: %v", err)
	}
	entries, err := s.List(ctx, "ns")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 || entries[0].Key != "a" || entries[1].Key != "k" {
		t.Fatalf("list = %+v, want keys [a k] sorted", entries)
	}

	// Delete removes; deleting a missing key is a no-op.
	if err := s.Delete(ctx, "ns", "k"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok, _ := s.Get(ctx, "ns", "k"); ok {
		t.Fatal("key present after delete")
	}
	if err := s.Delete(ctx, "ns", "k"); err != nil {
		t.Fatalf("delete missing should be no-op: %v", err)
	}

	// Empty namespace → empty list, no error.
	if e, err := s.List(ctx, "empty"); err != nil || len(e) != 0 {
		t.Fatalf("list empty ns: %v %v", e, err)
	}
}
