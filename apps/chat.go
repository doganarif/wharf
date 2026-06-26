package apps

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"wharf"
)

const chatHistoryMax = 200

// chatRecord is one posted message. It's broadcast to the room and persisted so
// new joiners (and restarts) see recent history.
type chatRecord struct {
	id    string
	color lipgloss.Color
	text  string
}

// storedMsg is the on-disk form of a chatRecord (exported fields for JSON).
type storedMsg struct {
	ID, Color, Text string
}

// chatStore is the process-local history cache, hydrated from and written
// through to a wharf.Bucket so it survives restarts. The whole (capped) log is
// stored as one JSON blob — simple and plenty fast at 200 messages.
type chatStore struct {
	mu     sync.Mutex
	recs   []chatRecord
	bucket *wharf.Bucket
	loaded bool
}

var chatLog = &chatStore{}

func (s *chatStore) hydrate(b *wharf.Bucket) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return
	}
	s.bucket = b
	if raw, ok, err := b.Get("log"); err == nil && ok {
		var stored []storedMsg
		if json.Unmarshal(raw, &stored) == nil {
			for _, m := range stored {
				s.recs = append(s.recs, chatRecord{id: m.ID, color: lipgloss.Color(m.Color), text: m.Text})
			}
		}
	}
	s.loaded = true
}

func (s *chatStore) append(r chatRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recs = append(s.recs, r)
	if len(s.recs) > chatHistoryMax {
		s.recs = s.recs[len(s.recs)-chatHistoryMax:]
	}
	if s.bucket != nil {
		stored := make([]storedMsg, len(s.recs))
		for i, rec := range s.recs {
			stored[i] = storedMsg{ID: rec.id, Color: string(rec.color), Text: rec.text}
		}
		if data, err := json.Marshal(stored); err == nil {
			_ = s.bucket.Set("log", data)
		}
	}
}

func (s *chatStore) snapshot() []chatRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]chatRecord(nil), s.recs...)
}

// recordVisit bumps and returns this user's visit count, keyed by fingerprint.
// This is the headline of pubkey identity: durable per-user state, no signup.
func recordVisit(s *wharf.Session) int {
	b := s.Bucket("visits")
	n := 0
	if raw, ok, err := b.Get(s.User.Fingerprint); err == nil && ok {
		n, _ = strconv.Atoi(string(raw))
	}
	n++
	_ = b.Set(s.User.Fingerprint, []byte(strconv.Itoa(n)))
	return n
}

// Chat is a keyless, signup-less chat room: your SSH key is your identity.
func Chat(s *wharf.Session) tea.Model {
	chatLog.hydrate(s.Bucket("history"))
	m := chatModel{sess: s, room: s.Join("chat"), visit: recordVisit(s)}
	for _, r := range chatLog.snapshot() {
		m.lines = append(m.lines, chatLine{id: r.id, color: r.color, text: r.text})
	}
	return m
}

type chatLine struct {
	id     string
	color  lipgloss.Color
	text   string
	system bool
}

type chatModel struct {
	sess   *wharf.Session
	room   *wharf.Room
	visit  int
	input  string
	lines  []chatLine
	roster []wharf.User
	known  map[string]bool // for join/leave diffing; nil until first presence
}

func (m chatModel) Init() tea.Cmd { return nil }

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.sess.Width, m.sess.Height = msg.Width, msg.Height

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			if text := strings.TrimSpace(m.input); text != "" {
				rec := chatRecord{id: m.sess.User.ShortID, color: m.sess.User.Color, text: text}
				chatLog.append(rec)   // persist for joiners + restarts
				m.room.Broadcast(rec) // echo comes back to us too, which renders it
			}
			m.input = ""
		case tea.KeyBackspace:
			if n := len(m.input); n > 0 {
				m.input = m.input[:n-1]
			}
		case tea.KeySpace:
			m.input += " "
		case tea.KeyRunes:
			m.input += string(msg.Runes)
		}

	case wharf.Event:
		if r, ok := msg.Payload.(chatRecord); ok {
			m.lines = append(m.lines, chatLine{id: r.id, color: r.color, text: r.text})
		}

	case wharf.Presence:
		m.lines = append(m.lines, m.presenceDiff(msg.Roster)...)
		m.roster = msg.Roster
	}
	return m, nil
}

// presenceDiff turns roster changes into "joined"/"left" system lines. The
// first roster just seeds the known set silently.
func (m *chatModel) presenceDiff(roster []wharf.User) []chatLine {
	next := map[string]bool{}
	for _, u := range roster {
		next[u.ShortID] = true
	}
	if m.known == nil {
		m.known = next
		return nil
	}
	var out []chatLine
	for _, u := range roster {
		if !m.known[u.ShortID] {
			out = append(out, chatLine{system: true, text: u.ShortID + " joined"})
		}
	}
	for id := range m.known {
		if !next[id] {
			out = append(out, chatLine{system: true, text: id + " left"})
		}
	}
	m.known = next
	return out
}

func (m chatModel) View() string {
	r := m.sess.Renderer
	h := m.sess.Height
	if h <= 0 {
		h = 24
	}

	header := fmt.Sprintf("%s  %d online  ·  you are %s (visit #%d)",
		r.NewStyle().Bold(true).Render("⚓ chat"),
		len(m.roster),
		m.sess.User.Style(r).Render(m.sess.User.ShortID),
		m.visit,
	)
	prompt := r.NewStyle().Bold(true).Render("› ") + m.input + r.NewStyle().Blink(true).Render("▏")

	// Body shows the most recent lines that fit between header and prompt.
	body := h - 3
	if body < 1 {
		body = 1
	}
	start := 0
	if len(m.lines) > body {
		start = len(m.lines) - body
	}
	var b strings.Builder
	for _, ln := range m.lines[start:] {
		if ln.system {
			fmt.Fprintf(&b, "%s\n", r.NewStyle().Faint(true).Italic(true).Render("— "+ln.text+" —"))
			continue
		}
		fmt.Fprintf(&b, "%s %s\n", r.NewStyle().Foreground(ln.color).Bold(true).Render(ln.id+":"), ln.text)
	}

	// Pad so the prompt sits at the bottom.
	rendered := strings.Count(b.String(), "\n")
	pad := strings.Repeat("\n", max(0, body-rendered))
	return header + "\n" + b.String() + pad + prompt
}
