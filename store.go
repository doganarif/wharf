package wharf

import (
	"context"
	"sort"
	"sync"
)

// Store is wharf's persistence seam: a small, namespaced key/value store.
//
// The domain is "identity-keyed small state" — per-user read/unread and
// bookmarks (key = fingerprint), plus modest app state (chat history, canvas
// cells). It is deliberately not a document or relational API.
//
// Implementations MUST be safe for concurrent use. Namespaces and keys are
// opaque strings; values are bytes (callers marshal their own types). A missing
// key is not an error — Get returns ok=false.
//
// The core ships MemoryStore (zero dependencies). Durable backends live in
// their own modules so importing wharf never drags in a database driver:
//
//	wharf/store/sqlite    — pure-Go SQLite (no CGO)
//	wharf/store/postgres  — Postgres via pgx
type Store interface {
	Get(ctx context.Context, ns, key string) (value []byte, ok bool, err error)
	Set(ctx context.Context, ns, key string, value []byte) error
	Delete(ctx context.Context, ns, key string) error
	List(ctx context.Context, ns string) ([]Entry, error)
	Close() error
}

// Entry is one key/value pair, returned by List (sorted by key).
type Entry struct {
	Key   string
	Value []byte
}

// Bucket is a Store scoped to a single namespace, so app code never juggles
// namespace strings. Get a bucket with Session.Bucket(name).
type Bucket struct {
	store Store
	ns    string
}

// Methods use context.Background for ergonomic call sites; reach for the Store
// directly if you need a deadline or cancellation.
func (b *Bucket) Get(key string) ([]byte, bool, error) {
	return b.store.Get(context.Background(), b.ns, key)
}
func (b *Bucket) Set(key string, value []byte) error {
	return b.store.Set(context.Background(), b.ns, key, value)
}
func (b *Bucket) Delete(key string) error {
	return b.store.Delete(context.Background(), b.ns, key)
}
func (b *Bucket) List() ([]Entry, error) {
	return b.store.List(context.Background(), b.ns)
}

// MemoryStore is an in-process Store. State is lost on restart — fine for
// demos, tests, and ephemeral apps; swap in a durable backend for the rest.
type MemoryStore struct {
	mu sync.RWMutex
	m  map[string]map[string][]byte
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{m: map[string]map[string][]byte{}}
}

func (s *MemoryStore) Get(_ context.Context, ns, key string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[ns][key]
	return cloneBytes(v), ok, nil
}

func (s *MemoryStore) Set(_ context.Context, ns, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.m[ns] == nil {
		s.m[ns] = map[string][]byte{}
	}
	s.m[ns][key] = cloneBytes(value)
	return nil
}

func (s *MemoryStore) Delete(_ context.Context, ns, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m[ns], key)
	return nil
}

func (s *MemoryStore) List(_ context.Context, ns string) ([]Entry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entry, 0, len(s.m[ns]))
	for k, v := range s.m[ns] {
		out = append(out, Entry{Key: k, Value: cloneBytes(v)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (s *MemoryStore) Close() error { return nil }

func cloneBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	c := make([]byte, len(b))
	copy(c, b)
	return c
}
