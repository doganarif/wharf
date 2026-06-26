package wharf

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Session is everything an app needs about one connected client: who they are
// (identity, for free), how to reach their room(s), persistence, a terminal
// renderer, and the initial window size.
type Session struct {
	User     User
	Renderer *lipgloss.Renderer
	Width    int
	Height   int
	Store    Store // never nil; defaults to an in-memory store

	App    string // the app's name, used to namespace this session's buckets
	hub    *Hub
	send   func(tea.Msg) // the program's Send; nil until bind()
	subs   []*subscriber
	notify func(Notification)
}

// Notify sends an out-of-band ping through the server's Notifier (if any),
// tagged with this session's user. Use it for retention: "a new issue is out".
// It is a no-op when no Notifier is configured.
func (s *Session) Notify(title, body string) {
	if s.notify != nil {
		s.notify(Notification{Title: title, Body: body, User: s.User})
	}
}

// Bucket returns a persistence bucket scoped to this app and the given name,
// e.g. s.Bucket("history") or s.Bucket("visits"). Namespacing by app keeps two
// apps from colliding in a shared store.
func (s *Session) Bucket(name string) *Bucket {
	return &Bucket{store: s.Store, ns: s.App + ":" + name}
}

// Room is a session's handle on a shared room. Broadcast sends a payload to
// every session currently in the room (including this one, which is how a
// sender sees its own message echoed).
type Room struct {
	sub *subscriber
}

func (r *Room) Broadcast(payload any) { r.sub.core.broadcast <- payload }
func (r *Room) Name() string          { return r.sub.core.name }

// Dispatch sends an action to a stateful room's reducer (registered via
// Server.StateRoom). The reducer runs in the room's goroutine and the resulting
// state is delivered to every member as a StateChanged message. On a plain room
// it is a no-op.
func (r *Room) Dispatch(action any) { r.sub.core.actions <- action }

// Join subscribes the session to a room and returns a handle for broadcasting.
// Safe to call from a model constructor (before the program runs) or later from
// Update; in both cases presence is updated and events start flowing.
func (s *Session) Join(name string) *Room {
	sub := &subscriber{core: s.hub.room(name), user: s.User, out: make(chan tea.Msg, 64)}
	s.subs = append(s.subs, sub)
	if s.send != nil { // joined after the program started; activate immediately
		go sub.pump(s.send)
		sub.core.join <- sub
	}
	return &Room{sub: sub}
}

// bind wires the session to its running program and activates any rooms joined
// during construction. Called by the server once the tea.Program exists.
func (s *Session) bind(send func(tea.Msg)) {
	s.send = send
	for _, sub := range s.subs {
		go sub.pump(send)
		sub.core.join <- sub
	}
}

// leaveAll removes the session from every room it joined. Called on disconnect.
func (s *Session) leaveAll() {
	for _, sub := range s.subs {
		sub.core.leave <- sub
	}
}
