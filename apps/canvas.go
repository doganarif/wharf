package apps

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"wharf"
)

const (
	canvasW = 72
	canvasH = 20
)

// cell is one square of the shared canvas.
type cell struct {
	set   bool
	color lipgloss.Color
}

// paintMsg / cursorMsg are the two things broadcast to the room.
type paintMsg struct {
	x, y  int
	color lipgloss.Color
	erase bool
}
type cursorMsg struct {
	user wharf.User
	x, y int
}

// Canvas is a multiplayer drawing surface: every connected client sees every
// other client's coloured cursor move in real time and can paint together. The
// board is persisted per-cell in a bucket, so it survives restarts.
func Canvas(s *wharf.Session) tea.Model {
	board := s.Bucket("board")
	return canvasModel{
		sess:    s,
		room:    s.Join("canvas"),
		board:   board,
		cx:      canvasW / 2,
		cy:      canvasH / 2,
		cells:   loadBoard(board),
		cursors: map[string]cursorMsg{},
	}
}

// loadBoard rebuilds the grid from persisted cells (keys are "x:y").
func loadBoard(b *wharf.Bucket) [][]cell {
	cells := make([][]cell, canvasH)
	for y := range cells {
		cells[y] = make([]cell, canvasW)
	}
	entries, err := b.List()
	if err != nil {
		return cells
	}
	for _, e := range entries {
		x, y, ok := parseXY(e.Key)
		if ok && x >= 0 && x < canvasW && y >= 0 && y < canvasH {
			cells[y][x] = cell{set: true, color: lipgloss.Color(e.Value)}
		}
	}
	return cells
}

func parseXY(key string) (int, int, bool) {
	i := strings.IndexByte(key, ':')
	if i < 0 {
		return 0, 0, false
	}
	x, err1 := strconv.Atoi(key[:i])
	y, err2 := strconv.Atoi(key[i+1:])
	return x, y, err1 == nil && err2 == nil
}

type canvasModel struct {
	sess    *wharf.Session
	room    *wharf.Room
	board   *wharf.Bucket
	cx, cy  int
	pen     bool
	cells   [][]cell
	cursors map[string]cursorMsg // other users, by ShortID
	roster  []wharf.User
}

func (m canvasModel) Init() tea.Cmd {
	m.room.Broadcast(cursorMsg{user: m.sess.User, x: m.cx, y: m.cy})
	return nil
}

// paint persists the cell (authoritative) and broadcasts it; the broadcast
// echoes back to update every client's local view, painter included.
func (m canvasModel) paint(erase bool) {
	key := strconv.Itoa(m.cx) + ":" + strconv.Itoa(m.cy)
	if erase {
		_ = m.board.Delete(key)
	} else {
		_ = m.board.Set(key, []byte(m.sess.User.Color))
	}
	m.room.Broadcast(paintMsg{x: m.cx, y: m.cy, color: m.sess.User.Color, erase: erase})
}

func (m canvasModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.sess.Width, m.sess.Height = msg.Width, msg.Height

	case tea.KeyMsg:
		moved := false
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			m.cy, moved = max(0, m.cy-1), true
		case "down", "j":
			m.cy, moved = min(canvasH-1, m.cy+1), true
		case "left", "h":
			m.cx, moved = max(0, m.cx-1), true
		case "right", "l":
			m.cx, moved = min(canvasW-1, m.cx+1), true
		case " ":
			m.paint(false)
		case "x":
			m.paint(true)
		case "p":
			m.pen = !m.pen
		}
		if moved {
			if m.pen {
				m.paint(false)
			}
			m.room.Broadcast(cursorMsg{user: m.sess.User, x: m.cx, y: m.cy})
		}

	case wharf.Event:
		switch e := msg.Payload.(type) {
		case paintMsg:
			if e.erase {
				m.cells[e.y][e.x] = cell{}
			} else {
				m.cells[e.y][e.x] = cell{set: true, color: e.color}
			}
		case cursorMsg:
			if e.user.ShortID != m.sess.User.ShortID {
				m.cursors[e.user.ShortID] = e
			}
		}

	case wharf.Presence:
		m.roster = msg.Roster
		present := map[string]bool{}
		for _, u := range msg.Roster {
			present[u.ShortID] = true
		}
		for id := range m.cursors {
			if !present[id] {
				delete(m.cursors, id)
			}
		}
	}
	return m, nil
}

func (m canvasModel) View() string {
	r := m.sess.Renderer

	// Cursors indexed by position; mine drawn on top.
	type marker struct {
		color lipgloss.Color
		me    bool
	}
	at := map[[2]int]marker{}
	for _, c := range m.cursors {
		at[[2]int{c.x, c.y}] = marker{color: c.user.Color}
	}
	at[[2]int{m.cx, m.cy}] = marker{color: m.sess.User.Color, me: true}

	var grid strings.Builder
	for y := 0; y < canvasH; y++ {
		for x := 0; x < canvasW; x++ {
			if mk, ok := at[[2]int{x, y}]; ok {
				glyph := "◆"
				if mk.me {
					glyph = "✛"
				}
				grid.WriteString(r.NewStyle().Foreground(mk.color).Render(glyph))
			} else if c := m.cells[y][x]; c.set {
				grid.WriteString(r.NewStyle().Foreground(c.color).Render("█"))
			} else {
				grid.WriteByte(' ')
			}
		}
		if y < canvasH-1 {
			grid.WriteByte('\n')
		}
	}

	frame := r.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	pen := "off"
	if m.pen {
		pen = "on"
	}
	header := fmt.Sprintf("%s  %s online  ·  you are %s",
		r.NewStyle().Bold(true).Render("⚓ canvas"),
		r.NewStyle().Bold(true).Render(fmt.Sprintf("%d", len(m.roster))),
		m.sess.User.Style(r).Render(m.sess.User.ShortID),
	)
	footer := r.NewStyle().Faint(true).Render(
		fmt.Sprintf("move ←↓↑→/hjkl · space paint · x erase · p pen[%s] · q quit", pen))

	return header + "\n" + frame.Render(grid.String()) + "\n" + footer
}
