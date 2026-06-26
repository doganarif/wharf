package apps

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"wharf"
)

// pollOptions are the fixed choices; keys 1..4 vote.
var pollOptions = []string{"Vim", "Emacs", "Nano", "VS Code"}

// pollState is the authoritative tally owned by the "poll" room. Reducers treat
// it as immutable: PollReducer returns a fresh map each time, so the snapshots
// held by individual sessions are never mutated under them.
type pollState struct {
	Counts map[string]int
}

type voteAction struct{ option string }

// PollInitialState is the starting tally for the "poll" stateful room.
func PollInitialState() any { return pollState{Counts: map[string]int{}} }

// PollReducer folds a vote into the tally. Register it with:
//
//	srv.App("poll", apps.Poll).StateRoom("poll", apps.PollInitialState(), apps.PollReducer)
func PollReducer(state, action any) any {
	ps, _ := state.(pollState)
	va, ok := action.(voteAction)
	if !ok {
		return ps
	}
	next := pollState{Counts: make(map[string]int, len(ps.Counts)+1)}
	for k, v := range ps.Counts {
		next.Counts[k] = v
	}
	next.Counts[va.option]++
	return next
}

// Poll is a live, multiplayer tally backed by a stateful room: no shared mutable
// state in the app, no manual snapshot wiring — joiners get the current tally,
// votes fan out as state updates.
func Poll(s *wharf.Session) tea.Model {
	return pollModel{sess: s, room: s.Join("poll"), counts: map[string]int{}}
}

type pollModel struct {
	sess   *wharf.Session
	room   *wharf.Room
	counts map[string]int
	roster []wharf.User
}

func (m pollModel) Init() tea.Cmd { return nil }

func (m pollModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.sess.Width, m.sess.Height = msg.Width, msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "1", "2", "3", "4":
			i := int(msg.String()[0] - '1')
			if i < len(pollOptions) {
				m.room.Dispatch(voteAction{option: pollOptions[i]})
			}
		}

	case wharf.Snapshot:
		if ps, ok := msg.State.(pollState); ok {
			m.counts = ps.Counts
		}
	case wharf.StateChanged:
		if ps, ok := msg.State.(pollState); ok {
			m.counts = ps.Counts
		}
	case wharf.Presence:
		m.roster = msg.Roster
	}
	return m, nil
}

func (m pollModel) View() string {
	r := m.sess.Renderer
	bold := r.NewStyle().Bold(true)
	bar := r.NewStyle().Foreground(m.sess.User.Color)

	total := 0
	for _, o := range pollOptions {
		total += m.counts[o]
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s   %d online · you are %s\n\n",
		bold.Render("⚓ poll — favourite editor?"),
		len(m.roster),
		m.sess.User.Style(r).Render(m.sess.User.ShortID))

	for i, o := range pollOptions {
		n := m.counts[o]
		width := 0
		if total > 0 {
			width = n * 30 / total
		}
		fmt.Fprintf(&b, " %d %-8s %s %d\n", i+1, o, bar.Render(strings.Repeat("█", width)), n)
	}
	fmt.Fprintf(&b, "\n%s", r.NewStyle().Faint(true).Render("press 1–4 to vote · q quit"))
	return b.String()
}
