// Package wharf is a small framework for building pubkey-authed, multiplayer
// SSH apps. You write a bubbletea model; wharf hands you verified identity,
// presence, persistence, and one-line broadcast to everyone else connected.
//
//	wharf.New(":2222").
//	    App("chat", chatApp).
//	    App("canvas", canvasApp).
//	    Run()
//
// The SSH username selects the app: `ssh chat@host` runs the "chat" app. An
// unknown or empty username lands in a lobby that lists the available apps.
package wharf

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	bm "github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
	"github.com/charmbracelet/wish/ratelimiter"
	"github.com/charmbracelet/wish/recover"
	"github.com/muesli/termenv"
	"golang.org/x/time/rate"
)

// AppFunc builds a bubbletea model for one connection. The Session gives it
// identity, rooms, persistence, a renderer, and the initial window size.
type AppFunc func(*Session) tea.Model

// Server is a wharf SSH server. Build it fluently with New().App(...).Run().
type Server struct {
	addr    string
	hostKey string
	hub     *Hub
	store   Store
	apps    map[string]AppFunc
	order   []string

	allow    func(User) bool
	identify func(User) User
	caKeys   string
	idle     time.Duration
	guard    *keyGuard

	notifier        Notifier
	notifyOnConnect bool

	metricsAddr string
}

// New returns a server listening on addr (e.g. ":2222"). It uses an in-memory
// store by default; call Store to make state durable across restarts.
func New(addr string) *Server {
	return &Server{
		addr:    addr,
		hostKey: ".ssh/wharf_host_key",
		hub:     newHub(),
		store:   NewMemoryStore(),
		apps:    map[string]AppFunc{},
		guard:   newKeyGuard(),
	}
}

// App registers an app under a name. The name is the SSH username used to reach
// it (`ssh <name>@host`).
func (s *Server) App(name string, fn AppFunc) *Server {
	if _, dup := s.apps[name]; !dup {
		s.order = append(s.order, name)
	}
	s.apps[name] = fn
	return s
}

// StateRoom registers a stateful room: members get the current state on join
// and a StateChanged after every Dispatch, with actions folded by reducer in
// the room's own goroutine. See apps.Poll for an example.
func (s *Server) StateRoom(name string, initial any, reducer Reducer) *Server {
	s.hub.registerState(name, initial, reducer)
	return s
}

// Store sets the persistence backend (e.g. a wharf/store/sqlite store). Apps
// reach it via Session.Bucket.
func (s *Server) Store(st Store) *Server { s.store = st; return s }

// HostKeyPath overrides where the server's host key is stored (generated on
// first run if absent).
func (s *Server) HostKeyPath(path string) *Server { s.hostKey = path; return s }

// Allow restricts who may connect: the function receives the verified identity
// and returns whether to accept the key. Without it, any key is accepted (an
// identity, not an authorisation). Rejection happens at the SSH auth step.
func (s *Server) Allow(fn func(User) bool) *Server { s.allow = fn; return s }

// Identify rewrites the verified identity before any app sees it — e.g. map a
// known fingerprint to a real name. The fingerprint should be left intact.
func (s *Server) Identify(fn func(User) User) *Server { s.identify = fn; return s }

// TrustedUserCAKeys restricts logins to keys signed by the given SSH CA
// (authorized_keys-format file of @cert-authority keys).
func (s *Server) TrustedUserCAKeys(path string) *Server { s.caKeys = path; return s }

// MaxSessionsPerKey caps how many sessions one public key may hold at once
// (0 = unlimited).
func (s *Server) MaxSessionsPerKey(n int) *Server { s.guard.maxSess = n; return s }

// PerKeyConnectRate limits new connections per key to perMinute, allowing short
// bursts of burst (0 = unlimited).
func (s *Server) PerKeyConnectRate(perMinute, burst int) *Server {
	if perMinute > 0 {
		s.guard.rate = rate.Limit(float64(perMinute) / 60.0)
		s.guard.burst = burst
	}
	return s
}

// IdleTimeout disconnects a session after d of no activity (0 = never). Note
// this counts input only, so silent watchers may be dropped — keep it generous.
func (s *Server) IdleTimeout(d time.Duration) *Server { s.idle = d; return s }

// Notify sets the retention notifier used by Session.Notify and (optionally)
// connect pings.
func (s *Server) Notify(n Notifier) *Server { s.notifier = n; return s }

// NotifyOnConnect, when set with a Notifier, fires a ping each time someone
// connects to an app.
func (s *Server) NotifyOnConnect(on bool) *Server { s.notifyOnConnect = on; return s }

// Metrics serves Prometheus-format metrics at addr/metrics (e.g. ":9100").
func (s *Server) Metrics(addr string) *Server { s.metricsAddr = addr; return s }

// Run starts the server and blocks until it errors or receives SIGINT/SIGTERM,
// at which point it shuts down gracefully and returns — so a caller's
// `defer store.Close()` actually runs.
func (s *Server) Run() error {
	perIP := ratelimiter.NewRateLimiter(rate.Every(time.Second), 5, 1024) // ~1/s per IP, burst 5
	opts := []ssh.Option{
		wish.WithAddress(s.addr),
		wish.WithHostKeyPath(s.hostKey),
		wish.WithPublicKeyAuth(s.authorize),
		wish.WithMiddleware(
			recover.Middleware(bm.MiddlewareWithProgramHandler(s.handler, termenv.ANSI256)),
			activeterm.Middleware(), // require an interactive terminal; drops bots
			ratelimiter.Middleware(perIP),
			logging.Middleware(),
		),
	}
	if s.idle > 0 {
		opts = append(opts, wish.WithIdleTimeout(s.idle))
	}
	if s.caKeys != "" {
		opts = append(opts, wish.WithTrustedUserCAKeys(s.caKeys))
	}

	srv, err := wish.NewServer(opts...)
	if err != nil {
		return err
	}

	if s.metricsAddr != "" {
		go s.serveMetrics()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		log.Printf("wharf listening on %s — apps: %v", s.addr, s.order)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if err == ssh.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		log.Println("wharf: shutting down…")
		shctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shctx)
	}
}

// authorize is the public-key auth hook: it accepts any key unless an allowlist
// is set. The handshake already proved key possession, so this is identity.
func (s *Server) authorize(ctx ssh.Context, key ssh.PublicKey) bool {
	if s.allow == nil {
		return true
	}
	return s.allow(userFromKey(key, ""))
}

// handler turns one SSH session into a running bubbletea program wired to wharf.
func (s *Server) handler(sess ssh.Session) *tea.Program {
	user := userFromKey(sess.PublicKey(), sess.RemoteAddr().String())
	fp := user.Fingerprint

	if ok, reason := s.guard.admit(fp); !ok {
		wish.Fatalln(sess, "wharf: "+reason)
		return nil
	}
	if s.identify != nil {
		user = s.identify(user)
	}

	name := sess.User()
	fn, ok := s.apps[name]
	if !ok {
		fn = s.lobby()
		name = "lobby"
	}

	pty, _, _ := sess.Pty()
	w := &Session{
		User:   user,
		Width:  pty.Window.Width,
		Height: pty.Window.Height,
		Store:  s.store,
		App:    name,
		hub:    s.hub,
		notify: s.fire,
	}

	opts := append([]tea.ProgramOption{tea.WithAltScreen()}, bm.MakeOptions(sess)...)
	p := tea.NewProgram(fn(w), opts...)

	// Join rooms BEFORE negotiating the renderer: MakeRenderer can block briefly
	// querying the terminal, and presence must not wait on that. The model reads
	// w.Renderer (via the *Session) only at View time, after this returns.
	w.bind(p.Send)
	w.Renderer = bm.MakeRenderer(sess)

	if s.notifyOnConnect {
		s.fire(Notification{Title: "wharf", Body: user.ShortID + " connected to " + name, User: user})
	}

	go func() { // clean up on disconnect
		<-sess.Context().Done()
		w.leaveAll()
		s.guard.release(fp)
	}()
	return p
}

// fire dispatches a notification asynchronously so a session never blocks on it.
func (s *Server) fire(n Notification) {
	if s.notifier == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.notifier.Notify(ctx, n); err != nil {
			log.Printf("wharf: notify: %v", err)
		}
	}()
}

func (s *Server) serveMetrics() {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		var b strings.Builder
		b.WriteString("# HELP wharf_room_members Current members per room.\n")
		b.WriteString("# TYPE wharf_room_members gauge\n")
		for _, rs := range s.hub.stats() {
			fmt.Fprintf(&b, "wharf_room_members{room=%q} %d\n", rs.Name, rs.Members)
		}
		b.WriteString("# HELP wharf_active_connections Active SSH sessions.\n")
		b.WriteString("# TYPE wharf_active_connections gauge\n")
		fmt.Fprintf(&b, "wharf_active_connections %d\n", s.guard.activeTotal())
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		io.WriteString(w, b.String())
	})
	log.Printf("wharf: metrics on %s/metrics", s.metricsAddr)
	if err := http.ListenAndServe(s.metricsAddr, mux); err != nil {
		log.Printf("wharf: metrics server: %v", err)
	}
}
