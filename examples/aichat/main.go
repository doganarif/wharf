// Command aichat is an EXAMPLE: a streaming Claude chat served over SSH on
// wharf, using the first-party Anthropic API (github.com/anthropics/anthropic-sdk-go).
//
// It exists to show the shape, not to be run in production — the point is that
// an LLM is "just an app" on top of wharf. wharf gives you verified identity,
// persistence, presence, and abuse controls; you bring the model. Putting this
// in its own module keeps the Anthropic SDK out of wharf's core.
//
//	ANTHROPIC_API_KEY=sk-... go run ./examples/aichat
//	ssh -p 2222 chat@localhost
//
// (Without a key it still builds and renders; the API call just errors — fine
// for a demo.)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	tea "github.com/charmbracelet/bubbletea"
	"wharf"
)

const systemPrompt = "You are a helpful assistant living in a terminal, reached over SSH. " +
	"Keep replies short and plain-text — no markdown headings or tables."

func main() {
	// NewClient reads ANTHROPIC_API_KEY from the environment.
	client := anthropic.NewClient()

	err := wharf.New(":2222").
		// wharf defaults to an in-memory store; swap in wharf/store/sqlite etc.
		// via .Store(...) to make conversations survive restarts.
		App("chat", func(s *wharf.Session) tea.Model { return newChat(s, client) }).
		Run()
	if err != nil {
		log.Fatal(err)
	}
}

// turn is one message; we persist a slice of these per user (keyed by the SSH
// key fingerprint) and rebuild Anthropic messages from them on each request.
type turn struct {
	Role string `json:"role"` // "user" | "assistant"
	Text string `json:"text"`
}

// chunk is one streamed token (or the terminal/err signal); it doubles as the
// tea.Msg the UI reacts to.
type chunk struct {
	text string
	err  error
	done bool
}

// streamReply calls Claude with the conversation so far and streams text deltas
// back on a channel — the standard way to drive a Bubble Tea UI from a stream.
func streamReply(ctx context.Context, client anthropic.Client, history []turn) <-chan chunk {
	out := make(chan chunk)
	go func() {
		defer close(out)

		msgs := make([]anthropic.MessageParam, 0, len(history))
		for _, t := range history {
			block := anthropic.NewTextBlock(t.Text)
			if t.Role == "assistant" {
				msgs = append(msgs, anthropic.NewAssistantMessage(block))
			} else {
				msgs = append(msgs, anthropic.NewUserMessage(block))
			}
		}

		stream := client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
			Model:     anthropic.ModelClaudeOpus4_8, // default per the model guidance
			MaxTokens: 1024,                         // deliberately short for a terminal
			System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
			Messages:  msgs,
		})
		for stream.Next() {
			if ev, ok := stream.Current().AsAny().(anthropic.ContentBlockDeltaEvent); ok {
				if d, ok := ev.Delta.AsAny().(anthropic.TextDelta); ok {
					out <- chunk{text: d.Text}
				}
			}
		}
		if err := stream.Err(); err != nil {
			out <- chunk{err: err}
			return
		}
		out <- chunk{done: true}
	}()
	return out
}

func newChat(s *wharf.Session, client anthropic.Client) tea.Model {
	m := chatModel{sess: s, client: client, store: s.Bucket("history")}
	if raw, ok, _ := m.store.Get(s.User.Fingerprint); ok { // resume prior conversation
		_ = json.Unmarshal(raw, &m.history)
	}
	return m
}

type chatModel struct {
	sess      *wharf.Session
	client    anthropic.Client
	store     *wharf.Bucket
	history   []turn
	input     string
	cur       string // assistant text streaming in this turn
	ch        <-chan chunk
	streaming bool
	status    string
	height    int
}

func (m *chatModel) save() {
	if data, err := json.Marshal(m.history); err == nil {
		_ = m.store.Set(m.sess.User.Fingerprint, data)
	}
}

func (m chatModel) Init() tea.Cmd { return nil }

func (m chatModel) waitChunk() tea.Cmd {
	ch := m.ch
	return func() tea.Msg { return <-ch }
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			text := strings.TrimSpace(m.input)
			m.input = ""
			if text == "" || m.streaming {
				return m, nil
			}
			m.history = append(m.history, turn{Role: "user", Text: text})
			m.save()
			m.streaming, m.cur, m.status = true, "", ""
			m.ch = streamReply(context.Background(), m.client, m.history)
			return m, m.waitChunk()
		case tea.KeyBackspace:
			if n := len(m.input); n > 0 {
				m.input = m.input[:n-1]
			}
		case tea.KeySpace:
			m.input += " "
		case tea.KeyRunes:
			m.input += string(msg.Runes)
		}

	case chunk:
		switch {
		case msg.err != nil:
			m.streaming, m.status = false, "error: "+msg.err.Error()
		case msg.done:
			m.streaming = false
			m.history = append(m.history, turn{Role: "assistant", Text: m.cur})
			m.cur = ""
			m.save()
		default:
			m.cur += msg.text
			return m, m.waitChunk()
		}
	}
	return m, nil
}

func (m chatModel) View() string {
	r := m.sess.Renderer
	h := m.height
	if h <= 0 {
		h = 24
	}
	bot := r.NewStyle().Bold(true)

	header := fmt.Sprintf("%s  you are %s",
		bot.Render("⚓ ai chat"), m.sess.User.Style(r).Render(m.sess.User.ShortID))

	var b strings.Builder
	for _, t := range m.history {
		who := "you"
		if t.Role == "assistant" {
			who = "ai"
		}
		fmt.Fprintf(&b, "%s %s\n", bot.Render(who), t.Text)
	}
	if m.streaming {
		fmt.Fprintf(&b, "%s %s▍\n", bot.Render("ai"), m.cur)
	}

	body := h - 3
	if body < 1 {
		body = 1
	}
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(lines) > body {
		lines = lines[len(lines)-body:]
	}
	pad := strings.Repeat("\n", max(0, body-len(lines)))

	prompt := bot.Render("› ") + m.input
	if m.streaming {
		prompt = r.NewStyle().Faint(true).Render("…thinking")
	}
	foot := m.status
	if foot == "" {
		foot = "enter to send · ctrl+c to quit"
	}
	return header + "\n" + strings.Join(lines, "\n") + pad + "\n" + prompt + "\n" + r.NewStyle().Faint(true).Render(foot)
}
