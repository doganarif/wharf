// Command server runs wharf with everything wired through env vars, showing how
// the adapters and options plug in.
//
//	WHARF_STORE=memory|sqlite|postgres   pick a backend
//	WHARF_PG_DSN=postgres://…            DSN when store=postgres
//	WHARF_SQLITE_PATH=wharf.db           file when store=sqlite
//	WHARF_METRICS=:9100                  serve Prometheus metrics
//	WHARF_MAX_SESSIONS=3                 cap concurrent sessions per key
//	WHARF_WEBHOOK_URL=https://…          ping a webhook on every connect
package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"wharf"
	"wharf/apps"
	"wharf/notify/webhook"
	"wharf/store/postgres"
	"wharf/store/sqlite"
)

func main() {
	srv := wharf.New(envOr("WHARF_ADDR", ":2222")).
		App("canvas", apps.Canvas).
		App("chat", apps.Chat).
		App("poll", apps.Poll).
		StateRoom("poll", apps.PollInitialState(), apps.PollReducer)

	if addr := os.Getenv("WHARF_METRICS"); addr != "" {
		srv.Metrics(addr)
	}
	if n, err := strconv.Atoi(os.Getenv("WHARF_MAX_SESSIONS")); err == nil && n > 0 {
		srv.MaxSessionsPerKey(n)
	}
	if url := os.Getenv("WHARF_WEBHOOK_URL"); url != "" {
		srv.Notify(webhook.New(url)).NotifyOnConnect(true)
		log.Println("notify: webhook on connect")
	}

	switch os.Getenv("WHARF_STORE") {
	case "sqlite":
		st, err := sqlite.Open(envOr("WHARF_SQLITE_PATH", "wharf.db"))
		if err != nil {
			log.Fatalf("sqlite: %v", err)
		}
		defer st.Close()
		srv.Store(st)
		log.Println("store: sqlite")

	case "postgres":
		dsn := os.Getenv("WHARF_PG_DSN")
		if dsn == "" {
			log.Fatal("WHARF_PG_DSN is required when WHARF_STORE=postgres")
		}
		st, err := postgres.Open(context.Background(), dsn)
		if err != nil {
			log.Fatalf("postgres: %v", err)
		}
		defer st.Close()
		srv.Store(st)
		log.Println("store: postgres")

	default:
		log.Println("store: memory (set WHARF_STORE=sqlite|postgres for durability)")
	}

	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
