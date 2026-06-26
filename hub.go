package wharf

import (
	"sort"
	"sync"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"
)

// Event carries an application broadcast to every session in a room. It is
// delivered into each session's bubbletea Update loop as a tea.Msg.
type Event struct {
	Room    string
	Payload any
}

// Presence is delivered to every session in a room whenever the roster changes
// (someone joins or leaves). It too arrives as a tea.Msg.
type Presence struct {
	Room   string
	Roster []User
}

// Reducer folds an action into a stateful room's state. It runs inside the
// room's single goroutine, so it never needs locks.
type Reducer func(state, action any) any

// Snapshot is delivered to a session the moment it joins a stateful room, so it
// renders the current state immediately.
type Snapshot struct {
	Room  string
	State any
}

// StateChanged is delivered to every session in a stateful room after an action
// is reduced.
type StateChanged struct {
	Room   string
	State  any
	Action any
}

// Hub owns all rooms for a server and hands out room actors on demand.
type Hub struct {
	mu    sync.Mutex
	rooms map[string]*roomState
	defs  map[string]stateDef // stateful-room definitions, by name
}

type stateDef struct {
	initial any
	reducer Reducer
}

func newHub() *Hub {
	return &Hub{rooms: map[string]*roomState{}, defs: map[string]stateDef{}}
}

// registerState marks a room name as stateful; rooms created under it own the
// given initial state and apply actions through reducer.
func (h *Hub) registerState(name string, initial any, reducer Reducer) {
	h.mu.Lock()
	h.defs[name] = stateDef{initial: initial, reducer: reducer}
	h.mu.Unlock()
}

func (h *Hub) room(name string) *roomState {
	h.mu.Lock()
	defer h.mu.Unlock()
	r, ok := h.rooms[name]
	if !ok {
		r = &roomState{
			name:      name,
			join:      make(chan *subscriber),
			leave:     make(chan *subscriber),
			broadcast: make(chan any, 64),
			actions:   make(chan any, 64),
		}
		if def, ok := h.defs[name]; ok {
			r.reducer = def.reducer
			r.state = def.initial
		}
		h.rooms[name] = r
		go r.run()
	}
	return r
}

// RoomStat is a point-in-time member count for one room.
type RoomStat struct {
	Name    string
	Members int
}

func (h *Hub) stats() []RoomStat {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]RoomStat, 0, len(h.rooms))
	for name, r := range h.rooms {
		out = append(out, RoomStat{Name: name, Members: int(r.count.Load())})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// subscriber is one connected session's membership in one room. Messages bound
// for the session are buffered on out and delivered by a dedicated pump
// goroutine, so a slow or not-yet-started client can never stall the room.
type subscriber struct {
	core *roomState
	user User
	out  chan tea.Msg
}

func (s *subscriber) pump(send func(tea.Msg)) {
	for msg := range s.out {
		send(msg) // blocks until the program consumes it; that's fine, it's our goroutine
	}
}

// roomState is a room actor. A single goroutine owns the member set (and, for
// stateful rooms, the state), so all mutation is serialised through channels —
// no locks, no data races.
type roomState struct {
	name      string
	join      chan *subscriber
	leave     chan *subscriber
	broadcast chan any
	actions   chan any
	count     atomic.Int64 // current member count, for observability

	// stateful rooms only:
	reducer Reducer
	state   any
}

func (r *roomState) run() {
	members := map[*subscriber]bool{}
	for {
		select {
		case s := <-r.join:
			members[s] = true
			r.count.Store(int64(len(members)))
			if r.reducer != nil {
				deliver(s, Snapshot{Room: r.name, State: r.state})
			}
			r.emitPresence(members)

		case s := <-r.leave:
			if members[s] {
				delete(members, s)
				r.count.Store(int64(len(members)))
				close(s.out) // stop the pump; safe here — no further deliver to s
				r.emitPresence(members)
			}

		case payload := <-r.broadcast:
			ev := Event{Room: r.name, Payload: payload}
			for s := range members {
				deliver(s, ev)
			}

		case action := <-r.actions:
			if r.reducer != nil {
				r.state = r.reducer(r.state, action)
				sc := StateChanged{Room: r.name, State: r.state, Action: action}
				for s := range members {
					deliver(s, sc)
				}
			}
		}
	}
}

func (r *roomState) emitPresence(members map[*subscriber]bool) {
	roster := make([]User, 0, len(members))
	for s := range members {
		roster = append(roster, s.user)
	}
	sort.Slice(roster, func(i, j int) bool { return roster[i].ShortID < roster[j].ShortID })
	p := Presence{Room: r.name, Roster: roster}
	for s := range members {
		deliver(s, p)
	}
}

// deliver is non-blocking: if a subscriber's buffer is full we drop the message
// rather than stall every other member of the room. Presence and cursor updates
// are idempotent enough that an occasional drop under load is harmless.
func deliver(s *subscriber, msg tea.Msg) {
	select {
	case s.out <- msg:
	default:
	}
}
