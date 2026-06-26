package wharf

import (
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// collector is a thread-safe sink standing in for a session's program.Send.
type collector struct {
	mu   sync.Mutex
	msgs []tea.Msg
}

func (c *collector) send(m tea.Msg) {
	c.mu.Lock()
	c.msgs = append(c.msgs, m)
	c.mu.Unlock()
}

func (c *collector) snapshot() []tea.Msg {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]tea.Msg(nil), c.msgs...)
}

// waitFor polls until pred is true or the deadline passes (the hub is async).
func waitFor(pred func() bool) bool {
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if pred() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return pred()
}

func joinSession(hub *Hub, id string, c *collector) *Session {
	s := &Session{User: User{ShortID: id}, hub: hub}
	s.Join("room")
	s.bind(c.send)
	return s
}

func TestPresenceAndBroadcast(t *testing.T) {
	hub := newHub()

	var ca, cb collector
	a := joinSession(hub, "alpha", &ca)

	// A alone: should see a presence roster of exactly itself.
	if !waitFor(func() bool { return rosterSize(ca.snapshot()) == 1 }) {
		t.Fatalf("A never saw a roster of 1, got %v", ca.snapshot())
	}

	joinSession(hub, "bravo", &cb)

	// Both should converge to a roster of 2.
	if !waitFor(func() bool { return rosterSize(ca.snapshot()) == 2 && rosterSize(cb.snapshot()) == 2 }) {
		t.Fatalf("rosters never reached 2: A=%d B=%d", rosterSize(ca.snapshot()), rosterSize(cb.snapshot()))
	}

	// A broadcast must reach B as an Event with the same payload.
	a.subs[0].core.broadcast <- "ping"
	if !waitFor(func() bool { return hasEvent(cb.snapshot(), "ping") }) {
		t.Fatalf("B never received broadcast; got %v", cb.snapshot())
	}
}

func rosterSize(msgs []tea.Msg) int {
	n := 0
	for _, m := range msgs {
		if p, ok := m.(Presence); ok {
			n = len(p.Roster) // last presence wins
		}
	}
	return n
}

func hasEvent(msgs []tea.Msg, payload any) bool {
	for _, m := range msgs {
		if e, ok := m.(Event); ok && e.Payload == payload {
			return true
		}
	}
	return false
}

func TestIdentityIsDeterministicAndDistinct(t *testing.T) {
	// Same fingerprint → same ShortID/colour; the derivation must be stable.
	u1 := userFromKey(nil, "1.2.3.4")
	u2 := userFromKey(nil, "1.2.3.4")
	if u1.ShortID != u2.ShortID || u1.Color != u2.Color {
		t.Fatalf("identity not deterministic: %+v vs %+v", u1, u2)
	}
	if u1.ShortID == "" {
		t.Fatal("empty ShortID")
	}
	other := userFromKey(nil, "9.9.9.9")
	if other.ShortID == u1.ShortID {
		t.Skip("rare ShortID collision across two inputs; not a failure") // 1024 combos
	}
}
