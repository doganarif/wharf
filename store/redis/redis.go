// Package redis provides a Redis-backed wharf.Store. Each namespace is one
// Redis hash, so per-namespace listing is a single HGETALL. Good for the
// ephemeral-but-shared case and for spanning multiple server instances.
//
//	st, err := redis.Open(ctx, "redis://localhost:6379")
//	wharf.New(":2222").Store(st).App("chat", apps.Chat).Run()
package redis

import (
	"context"
	"sort"
	"strings"

	"wharf"

	goredis "github.com/redis/go-redis/v9"
)

var _ wharf.Store = (*Store)(nil)

// Store is a wharf.Store backed by Redis.
type Store struct {
	c      *goredis.Client
	prefix string
}

// Open connects to Redis. addr may be a URL ("redis://host:6379/0") or a plain
// "host:port". Keys are stored under the hash "wharf:<namespace>".
func Open(ctx context.Context, addr string) (*Store, error) {
	var opt *goredis.Options
	if strings.Contains(addr, "://") {
		var err error
		if opt, err = goredis.ParseURL(addr); err != nil {
			return nil, err
		}
	} else {
		opt = &goredis.Options{Addr: addr}
	}
	c := goredis.NewClient(opt)
	if err := c.Ping(ctx).Err(); err != nil {
		c.Close()
		return nil, err
	}
	return &Store{c: c, prefix: "wharf:"}, nil
}

func (s *Store) hkey(ns string) string { return s.prefix + ns }

func (s *Store) Get(ctx context.Context, ns, key string) ([]byte, bool, error) {
	v, err := s.c.HGet(ctx, s.hkey(ns), key).Result()
	if err == goredis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return []byte(v), true, nil
}

func (s *Store) Set(ctx context.Context, ns, key string, value []byte) error {
	return s.c.HSet(ctx, s.hkey(ns), key, value).Err()
}

func (s *Store) Delete(ctx context.Context, ns, key string) error {
	return s.c.HDel(ctx, s.hkey(ns), key).Err()
}

func (s *Store) List(ctx context.Context, ns string) ([]wharf.Entry, error) {
	m, err := s.c.HGetAll(ctx, s.hkey(ns)).Result()
	if err != nil {
		return nil, err
	}
	out := make([]wharf.Entry, 0, len(m))
	for k, v := range m {
		out = append(out, wharf.Entry{Key: k, Value: []byte(v)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (s *Store) Close() error { return s.c.Close() }
