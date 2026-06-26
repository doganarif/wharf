package wharf

import (
	"sync"

	"golang.org/x/time/rate"
)

// keyGuard enforces per-public-key limits: a cap on concurrent sessions and an
// optional connection rate. It complements the per-IP rate limiter — a single
// key can't open unlimited sessions or reconnect-storm.
type keyGuard struct {
	mu       sync.Mutex
	active   map[string]int
	limiters map[string]*rate.Limiter
	maxSess  int        // 0 = unlimited
	rate     rate.Limit // 0 = unlimited
	burst    int
}

func newKeyGuard() *keyGuard {
	return &keyGuard{active: map[string]int{}, limiters: map[string]*rate.Limiter{}}
}

// admit reports whether a new session for fingerprint fp may start, and
// reserves a slot if so. Pair every true with a release.
func (g *keyGuard) admit(fp string) (bool, string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.rate > 0 {
		lim := g.limiters[fp]
		if lim == nil {
			lim = rate.NewLimiter(g.rate, g.burst)
			g.limiters[fp] = lim
		}
		if !lim.Allow() {
			return false, "connecting too fast, try again shortly"
		}
	}
	if g.maxSess > 0 && g.active[fp] >= g.maxSess {
		return false, "too many concurrent sessions for this key"
	}
	g.active[fp]++
	return true, ""
}

func (g *keyGuard) release(fp string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active[fp] <= 1 {
		delete(g.active, fp)
	} else {
		g.active[fp]--
	}
}

func (g *keyGuard) activeTotal() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for _, c := range g.active {
		n += c
	}
	return n
}
