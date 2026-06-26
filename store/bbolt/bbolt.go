// Package bbolt provides a single-file, embedded wharf.Store backed by bbolt —
// durable persistence with no server to run. Each namespace is a bbolt bucket.
//
//	st, err := bbolt.Open("wharf.boltdb")
//	wharf.New(":2222").Store(st).App("chat", apps.Chat).Run()
package bbolt

import (
	"context"

	"wharf"

	bolt "go.etcd.io/bbolt"
)

var _ wharf.Store = (*Store)(nil)

// Store is a wharf.Store backed by a bbolt file.
type Store struct{ db *bolt.DB }

// Open opens (or creates) the database at path.
func Open(path string) (*Store, error) {
	db, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Get(_ context.Context, ns, key string) ([]byte, bool, error) {
	var val []byte
	var found bool
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(ns))
		if b == nil {
			return nil
		}
		if v := b.Get([]byte(key)); v != nil {
			val = append([]byte(nil), v...) // copy: bbolt values are only valid in-tx
			found = true
		}
		return nil
	})
	return val, found, err
}

func (s *Store) Set(_ context.Context, ns, key string, value []byte) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(ns))
		if err != nil {
			return err
		}
		return b.Put([]byte(key), value)
	})
}

func (s *Store) Delete(_ context.Context, ns, key string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(ns))
		if b == nil {
			return nil
		}
		return b.Delete([]byte(key))
	})
}

func (s *Store) List(_ context.Context, ns string) ([]wharf.Entry, error) {
	var out []wharf.Entry
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(ns))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error { // bbolt iterates keys byte-sorted
			out = append(out, wharf.Entry{Key: string(k), Value: append([]byte(nil), v...)})
			return nil
		})
	})
	return out, err
}

func (s *Store) Close() error { return s.db.Close() }
