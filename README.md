# ⚓ wharf

**Multiplayer SSH apps in ~20 lines.** Write a [Bubble Tea](https://github.com/charmbracelet/bubbletea) model; wharf hands you verified identity, presence, and one-line broadcast to everyone else connected.

```
ssh canvas@your.host   # draw together, in your terminal
ssh chat@your.host     # talk — no signup, your SSH key is your account
```

[Charm's `wish`](https://github.com/charmbracelet/wish) gives you the SSH *transport* for a terminal app. wharf is the thin layer on top that everyone otherwise rebuilds by hand: **accounts, presence, multiplayer, and persistence**.

---

## Why an SSH key is the whole trick

When a client connects, the SSH handshake **cryptographically proves they hold the private key**. So the key fingerprint is a stable, verified, zero-signup account id — no email, no OAuth, no password, no GDPR surface. wharf turns it into a friendly handle (`brave-otter`) and a colour, deterministically, so even anonymous strangers have a consistent identity across visits.

> It's an *identity*, not an *authorisation*: anyone can generate a key. Perfect for chat/canvas/newsletters; layer your own limits for anything abuse-sensitive.

## Quick start

```sh
go run ./cmd/wharf          # :2222, in-memory store, zero extra deps
```

Or run it with a durable backend:

```sh
WHARF_STORE=sqlite go run ./examples/server                       # → ./wharf.db
WHARF_STORE=postgres WHARF_PG_DSN=postgres://… go run ./examples/server
```

Then, from another terminal:

```sh
ssh -p 2222 canvas@localhost
ssh -p 2222 chat@localhost
```

Open two terminals and watch the cursors move / messages appear in real time. The SSH **username selects the app**; an unknown name lands in a lobby. With a durable store, the drawing and your visit count are still there after a restart.

## Writing an app

An app is a function `*wharf.Session → tea.Model`. The session gives you identity, a renderer, the window size, and rooms.

```go
func main() {
    wharf.New(":2222").
        App("chat",   apps.Chat).
        App("canvas", apps.Canvas).
        Run()
}

func Chat(s *wharf.Session) tea.Model {
    return chatModel{
        me:   s.User,           // .Fingerprint (verified) + .ShortID ("brave-otter") + .Color
        room: s.Join("chat"),   // a shared, server-authoritative room
    }
}
```

Send to everyone in the room (including yourself, so your own message echoes):

```go
m.room.Broadcast(chatRecord{id: m.me.ShortID, text: text})
```

Receive — wharf delivers room traffic straight into your normal `Update`:

```go
case wharf.Event:                       // an app broadcast
    if r, ok := msg.Payload.(chatRecord); ok { ... }
case wharf.Presence:                    // roster changed (someone joined/left)
    m.roster = msg.Roster
```

That's the whole API: `Session`, `Join`, `Room.Broadcast`, and the `Event` / `Presence` messages.

## How it works

The one design decision that keeps it simple: **each room is an actor**. A single goroutine owns the member set, and every join/leave/broadcast is a message sent to it over a channel — so there are **no locks and no data races**, ever. Per-subscriber outbound buffers mean one slow client can never stall the room.

```
ssh session ─┐                 ┌─ room goroutine (owns members) ─┐
ssh session ─┼─ subscriber ──► │  join / leave / broadcast       │ ──► every session's Update
ssh session ─┘   (buffered)    └─────────────────────────────────┘
```

Room traffic is transport; **app state lives in the app** (see the canvas board / chat history) — unless you want the room to own it. Register a **stateful room** and the actor holds the state, folds actions through your reducer (still lock-free, it runs in the room's goroutine), hands every joiner a `Snapshot`, and broadcasts a `StateChanged` after each `Dispatch`:

```go
wharf.New(":2222").
    App("poll", apps.Poll).
    StateRoom("poll", apps.PollInitialState(), apps.PollReducer)
```

```go
case wharf.Snapshot:     m.tally = msg.State.(tally)   // current state on join
case wharf.StateChanged: m.tally = msg.State.(tally)   // after any Dispatch
// ... m.room.Dispatch(vote{"Vim"})
```

## Persistence

Identity is verified and stable, so it makes a perfect primary key. `Session.Bucket(name)` gives an app a namespaced key/value store; the headline use is **per-user state keyed by fingerprint** — no signup, no email:

```go
// "welcome back, visit #4" — durable, per pubkey, zero accounts
b := s.Bucket("visits")
raw, _, _ := b.Get(s.User.Fingerprint)
n, _ := strconv.Atoi(string(raw))
b.Set(s.User.Fingerprint, []byte(strconv.Itoa(n + 1)))
```

The backend is a small interface, and **each adapter is its own Go module** so importing `wharf` never drags in a database driver:

| Backend  | Import                  | Dependencies pulled |
|----------|-------------------------|---------------------|
| Memory   | built into `wharf`      | none (default)      |
| SQLite   | `wharf/store/sqlite`    | `modernc.org/sqlite` (pure Go, no CGO) |
| Postgres | `wharf/store/postgres`  | `jackc/pgx`         |
| Redis    | `wharf/store/redis`     | `redis/go-redis`    |
| bbolt    | `wharf/store/bbolt`     | `go.etcd.io/bbolt` (single file, no server) |

```go
st, _ := sqlite.Open("wharf.db")
wharf.New(":2222").Store(st).App("chat", apps.Chat).Run()
```

Writing a new backend means implementing five methods (`Get/Set/Delete/List/Close`) and passing the shared conformance suite in `wharf/storetest` — the same suite the SQLite and Postgres adapters pass against live databases.

## Operating it

There is **no shell** — only your registered apps run. Out of the box you get a per-IP rate limiter, panic recovery per session, an active-terminal gate (drops non-interactive bots), and connection logging. Everything else is a fluent option:

```go
wharf.New(":2222").
    Allow(func(u wharf.User) bool { return allowed[u.Fingerprint] }). // private server
    MaxSessionsPerKey(3).                 // cap concurrent sessions per key
    PerKeyConnectRate(30, 5).             // 30/min per key, burst 5
    IdleTimeout(30 * time.Minute).        // drop idle sessions
    Identify(mapKnownNames).              // fingerprint → real name
    TrustedUserCAKeys("ca.pub").          // accept only CA-signed keys
    Notify(telegram.New(token, chat)).NotifyOnConnect(true). // retention pings
    Metrics(":9100").                     // Prometheus /metrics
    App("chat", apps.Chat).
    Run()                                 // blocks; SIGINT/SIGTERM → graceful shutdown
```

`Run` returns on SIGINT/SIGTERM after a graceful `Shutdown`, so a caller's `defer store.Close()` actually fires. Notifiers (`notify/telegram`, `notify/webhook`) use only the standard library; apps can also ping on their own via `Session.Notify`. `Metrics` exposes `wharf_active_connections` and `wharf_room_members{room=…}`.

## What works today

Working and verified end-to-end over real SSH (real terminals, concurrent clients, live databases):

- ✅ **Verified identity for free** — pubkey fingerprint → stable `ShortID` + colour, zero signup
- ✅ **Multiplayer rooms** — one-line `Broadcast`, lock-free room actors, per-subscriber buffering
- ✅ **Stateful rooms** — room-owned state + reducer, `Snapshot` on join, `StateChanged` on `Dispatch`
- ✅ **Live presence** — accurate join/leave roster, cleaned up on disconnect
- ✅ **Selectable lobby** — connect with no app name and pick one from a keyboard menu
- ✅ **Persistence that survives restarts** — per-user state keyed by fingerprint and shared app state, across **SQLite, Postgres, Redis, and bbolt** (all pass one conformance suite; Postgres/Redis verified against live servers)
- ✅ **Retention hooks** — Telegram / webhook notifiers, on connect or on demand
- ✅ **Abuse & auth controls** — allowlist, per-key session cap + connect rate, idle timeout, identity mapping, SSH-CA trust
- ✅ **Graceful shutdown** + **Prometheus metrics**
- ✅ **Tooling** — `create-wharf-app` scaffolder, CI matrix (spins up Postgres + Redis)
- ✅ **Demo apps** — `canvas` (drawing), `chat` (keyless room), `poll` (stateful tally)

Pre-1.0: the API may still change before a tagged release.

## Layout

This is a multi-module repo so the core stays dependency-free:

```
wharf (module)
  wharf.go            Server, options, the SSH handler, graceful shutdown, metrics
  hub.go              Hub + room actor (broadcast, presence, stateful rooms)
  session.go          Session, Join, Room handle (Broadcast/Dispatch), Bucket, Notify
  store.go            Store interface + in-memory implementation
  identity.go         pubkey → ShortID + colour
  guard.go            per-key session cap + connect-rate limiter
  notify.go           Notifier interface
  lobby.go            selectable app menu (router model)
  storetest/          backend conformance suite
  apps/               canvas.go, chat.go, poll.go — the demos
  notify/telegram/    Telegram notifier      (stdlib only)
  notify/webhook/     webhook notifier        (stdlib only)
  cmd/wharf/          zero-dependency runnable (memory store)
  cmd/create-wharf-app/  scaffolder for a new app
  store/sqlite/       (module) SQLite adapter
  store/postgres/     (module) Postgres adapter
  store/redis/        (module) Redis adapter
  store/bbolt/        (module) bbolt adapter
  examples/server/    (module) runnable wiring every option via env vars
  .github/workflows/  CI matrix (Postgres + Redis services)
```

Local builds resolve the unpublished modules via `replace` directives in each adapter's `go.mod` (remove on publish).

## License

MIT.
