package wharf

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// lobby returns an AppFunc shown when the SSH username matches no app. It lists
// the registered apps and lets the user pick one with the keyboard — no need to
// reconnect with the app name as the SSH user. Once an app is chosen, the lobby
// delegates entirely to it for the rest of the session.
func (s *Server) lobby() AppFunc {
	apps := make([]namedApp, len(s.order))
	for i, name := range s.order {
		apps[i] = namedApp{name: name, fn: s.apps[name]}
	}
	return func(sess *Session) tea.Model {
		return &lobbyModel{sess: sess, apps: apps, w: sess.Width, h: sess.Height}
	}
}

type namedApp struct {
	name string
	fn   AppFunc
}

type lobbyModel struct {
	sess   *Session
	apps   []namedApp
	cursor int
	active tea.Model // once set, the chosen app takes over
	w, h   int
}

func (m *lobbyModel) Init() tea.Cmd { return nil }

func (m *lobbyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sz, ok := msg.(tea.WindowSizeMsg); ok {
		m.w, m.h = sz.Width, sz.Height
	}

	// After a choice, the app owns the session.
	if m.active != nil {
		next, cmd := m.active.Update(msg)
		m.active = next
		return m, cmd
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.apps)-1 {
				m.cursor++
			}
		case "enter", "right", "l":
			if len(m.apps) == 0 {
				return m, nil
			}
			child := m.apps[m.cursor].fn(m.sess)
			initCmd := child.Init()
			child, sizeCmd := child.Update(tea.WindowSizeMsg{Width: m.w, Height: m.h})
			m.active = child
			return m, tea.Batch(initCmd, sizeCmd)
		}
	}
	return m, nil
}

func (m *lobbyModel) View() string {
	if m.active != nil {
		return m.active.View()
	}

	r := m.sess.Renderer
	title := r.NewStyle().Bold(true).Foreground(m.sess.User.Color)
	dim := r.NewStyle().Faint(true)
	sel := r.NewStyle().Bold(true).Foreground(m.sess.User.Color)

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", title.Render("⚓ wharf"))
	fmt.Fprintf(&b, "you are %s\n\n", m.sess.User.Style(r).Render(m.sess.User.ShortID))

	if len(m.apps) == 0 {
		b.WriteString("no apps registered yet.\n")
	} else {
		b.WriteString("pick an app:\n\n")
		for i, a := range m.apps {
			if i == m.cursor {
				fmt.Fprintf(&b, "  %s\n", sel.Render("› "+a.name))
			} else {
				fmt.Fprintf(&b, "    %s\n", a.name)
			}
		}
	}
	fmt.Fprintf(&b, "\n%s", dim.Render("↑/↓ move · enter select · q quit"))
	return b.String()
}
