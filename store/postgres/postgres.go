// Package postgres provides a Postgres-backed wharf.Store via pgx.
//
//	st, err := postgres.Open(ctx, "postgres://user:pass@host:5432/db")
//	if err != nil { log.Fatal(err) }
//	wharf.New(":2222").Store(st).App("chat", apps.Chat).Run()
package postgres

import (
	"context"
	"database/sql"

	"wharf"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver
)

var _ wharf.Store = (*Store)(nil)

// Store is a wharf.Store backed by Postgres.
type Store struct{ db *sql.DB }

const schema = `CREATE TABLE IF NOT EXISTS wharf_kv (
	ns    TEXT  NOT NULL,
	key   TEXT  NOT NULL,
	value BYTEA NOT NULL,
	PRIMARY KEY (ns, key)
);`

// Open connects using a libpq/pgx DSN, verifies the connection, and ensures
// the schema exists.
func Open(ctx context.Context, dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.ExecContext(ctx, schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Get(ctx context.Context, ns, key string) ([]byte, bool, error) {
	var v []byte
	err := s.db.QueryRowContext(ctx, `SELECT value FROM wharf_kv WHERE ns=$1 AND key=$2`, ns, key).Scan(&v)
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
		`INSERT INTO wharf_kv(ns,key,value) VALUES($1,$2,$3)
		 ON CONFLICT (ns,key) DO UPDATE SET value=excluded.value`,
		ns, key, value)
	return err
}

func (s *Store) Delete(ctx context.Context, ns, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM wharf_kv WHERE ns=$1 AND key=$2`, ns, key)
	return err
}

func (s *Store) List(ctx context.Context, ns string) ([]wharf.Entry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key,value FROM wharf_kv WHERE ns=$1 ORDER BY key`, ns)
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
