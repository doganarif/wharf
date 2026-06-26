// Command create-wharf-app scaffolds a runnable wharf app to get you started.
//
//	go run ./cmd/create-wharf-app guestbook
//	# writes guestbook.go — then: go run ./guestbook.go, ssh -p 2222 guestbook@localhost
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		fmt.Fprintln(os.Stderr, "usage: create-wharf-app <name> [import-path]")
		fmt.Fprintln(os.Stderr, "  import-path defaults to \"wharf\"")
		os.Exit(2)
	}
	name := os.Args[1]
	importPath := "wharf"
	if len(os.Args) > 2 {
		importPath = os.Args[2]
	}

	lower := strings.ToLower(name)
	data := struct{ Name, Lower, Import string }{
		Name:   capitalize(name),
		Lower:  lower,
		Import: importPath,
	}

	out := filepath.Clean(lower + ".go")
	if _, err := os.Stat(out); err == nil {
		fmt.Fprintf(os.Stderr, "refusing to overwrite existing %s\n", out)
		os.Exit(1)
	}
	f, err := os.Create(out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	if err := starter.Execute(f, data); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("wrote %s\n\nnext:\n  go run ./%s\n  ssh -p 2222 %s@localhost\n", out, out, lower)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

var starter = template.Must(template.New("app").Parse(`package main

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"{{.Import}}"
)

func main() {
	if err := wharf.New(":2222").App("{{.Lower}}", {{.Name}}).Run(); err != nil {
		log.Fatal(err)
	}
}

// {{.Name}} is your wharf app. Reach it with: ssh -p 2222 {{.Lower}}@localhost
func {{.Name}}(s *wharf.Session) tea.Model {
	return {{.Lower}}Model{sess: s, room: s.Join("{{.Lower}}")}
}

type {{.Lower}}Model struct {
	sess   *wharf.Session
	room   *wharf.Room
	roster []wharf.User
	last   string
}

func (m {{.Lower}}Model) Init() tea.Cmd { return nil }

func (m {{.Lower}}Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			m.room.Broadcast("👋 from " + m.sess.User.ShortID)
		}
	case wharf.Event: // someone broadcast to the room
		if s, ok := msg.Payload.(string); ok {
			m.last = s
		}
	case wharf.Presence: // the roster changed
		m.roster = msg.Roster
	}
	return m, nil
}

func (m {{.Lower}}Model) View() string {
	r := m.sess.Renderer
	out := r.NewStyle().Bold(true).Render("⚓ {{.Lower}}") +
		"  " + m.sess.User.Style(r).Render(m.sess.User.ShortID) +
		"  (" + itoa(len(m.roster)) + " online)\n\n"
	if m.last != "" {
		out += m.last + "\n\n"
	}
	return out + r.NewStyle().Faint(true).Render("enter: broadcast · q: quit") + "\n"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
`))
