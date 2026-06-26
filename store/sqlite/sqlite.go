// Package sqlite provides a pure-Go (no CGO) SQLite-backed wharf.Store.
//
//	st, err := sqlite.Open("wharf.db")
//	if err != nil { log.Fatal(err) }
//	wharf.New(":2222").Store(st).App("chat", apps.Chat).Run()
package sqlite

import (
	"context"
	"database/sql"

	"wharf"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" driver
)

// compile-time check that the adapter satisfies the interface
var _ wharf.Store = (*Store)(nil)

// Store is a wharf.Store backed by a SQLite file.
type Store struct{ db *sql.DB }

const schema = `CREATE TABLE IF NOT EXISTS wharf_kv (
	ns    TEXT NOT NULL,
	key   TEXT NOT NULL,
	value BLOB NOT NULL,
	PRIMARY KEY (ns, key)
);`

// Open opens (or creates) the database at path and ensures the schema exists.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// SQLite writes are serialised by a single writer; one connection avoids
	// "database is locked" under concurrent sessions. KV writes are tiny.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Get(ctx context.Context, ns, key string) ([]byte, bool, error) {
	var v []byte
	err := s.db.QueryRowContext(ctx, `SELECT value FROM wharf_kv WHERE ns=? AND key=?`, ns, key).Scan(&v)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return v, true, nil
}

func (s *Store) Set(ctx context.Context, ns, key string, value []byte) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO wharf_kv(ns,key,value) VALUES(?,?,?)
		 ON CONFLICT(ns,key) DO UPDATE SET value=excluded.value`,
		ns, key, value)
	return err
}

func (s *Store) Delete(ctx context.Context, ns, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM wharf_kv WHERE ns=? AND key=?`, ns, key)
	return err
}

func (s *Store) List(ctx context.Context, ns string) ([]wharf.Entry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key,value FROM wharf_kv WHERE ns=? ORDER BY key`, ns)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []wharf.Entry
	for rows.Next() {
		var e wharf.Entry
		if err := rows.Scan(&e.Key, &e.Value); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) Close() error { return s.db.Close() }
