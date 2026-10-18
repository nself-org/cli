package entitlement

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/license"
)

// arGuard mirrors sdk/go/licensing Guard (the SDK is a separate module the CLI module does not
// import): it re-reads the entitlement file every 24 h, a complete token sets deadline = signed
// exp + 30 d, an empty or invalid file revokes, and every request checks now <= deadline.
type arGuard struct {
	path, slug string
	valid      bool
	deadline   time.Time
	last       time.Time
}

const (
	simGrace = 30 * 24 * time.Hour
	len60    = 60 * 24
)

func (g *arGuard) reload(now time.Time) {
	g.last = now
	data, err := os.ReadFile(g.path)
	claims, perr := license.ParseLicenseJWT(string(data))
	if err != nil || perr != nil {
		g.valid = false
		return
	}
	g.valid = false
	for _, name := range claims.Plugins {
		g.valid = g.valid || name == g.slug || name == "*"
	}
	g.deadline = time.Unix(claims.Exp, 0).Add(simGrace)
}

// serve reports whether a request at now is answered (true) or gets 503 (false).
func (g *arGuard) serve(now time.Time) bool {
	if g.last.IsZero() || now.Sub(g.last) >= 24*time.Hour {
		g.reload(now)
	}
	return g.valid && !now.After(g.deadline)
}

// simRun drives the timer schedule (every 6 h + 0..30 min jitter) and an hourly request;
// step(hour, mode) sets the ping mode for that simulated hour.
type simRun struct {
	t       *testing.T
	ping    *arPing
	guard   *arGuard
	project string
	start   time.Time
	nextRun time.Time
	runs    int
	fails   int
	lastOK  time.Time // issue time of the newest token a run wrote
}

type arClock struct{ p *arPing }

func (c arClock) Now() time.Time { c.p.mu.Lock(); defer c.p.mu.Unlock(); return c.p.now }

func newSim(t *testing.T) *simRun {
	t.Helper()
	compattest.Set(t, true)
	start := time.Now()
	p := newArPing(t, start, "chat")
	p.seedCache(t)
	fakeTimer(t)
	old := refreshClock
	refreshClock = arClock{p}
	t.Cleanup(func() { refreshClock = old })
	s := &simRun{t: t, ping: p, project: t.TempDir(), start: start, nextRun: start}
	s.guard = &arGuard{path: Path(s.project), slug: "chat"}
	return s
}

// step advances one simulated hour: it fires due timer runs, then serves one request.
func (s *simRun) step(hour int, mode string) bool {
	now := s.start.Add(time.Duration(hour) * time.Hour)
	for !s.nextRun.After(now) {
		s.ping.set(mode, s.nextRun)
		s.runs++
		_, err := RunScheduled(context.Background(), s.project)
		if err == nil {
			s.lastOK = s.nextRun
		} else {
			var failed *RefreshError
			if !errors.As(err, &failed) {
				s.t.Fatalf("unexpected error %v", err)
			}
			s.fails++
		}
		s.nextRun = s.nextRun.Add(6*time.Hour + time.Duration(s.runs*7%30)*time.Minute)
	}
	return s.guard.serve(now)
}

func TestSim60Days(t *testing.T) {
	s := newSim(t)
	for hour := 0; hour < 60*24; hour++ {
		if !s.step(hour, "ok") {
			t.Fatalf("503 at day %.1f although the licence is paid and reachable", float64(hour)/24)
		}
	}
	if s.fails != 0 || s.runs < 200 {
		t.Fatalf("timer runs=%d failures=%d", s.runs, s.fails)
	}
}

func TestSimPingDown(t *testing.T) {
	s := newSim(t)
	downFrom, upAt := 10*24, 45*24
	var lastIssued time.Time
	first503, recovered := -1, -1
	for hour := 0; hour < 60*24; hour++ {
		mode := "ok"
		if hour >= downFrom && hour < upAt {
			mode = "5xx"
		}
		ok := s.step(hour, mode)
		if hour == upAt-1 {
			lastIssued = s.lastOK
		}
		if !ok && first503 < 0 {
			first503 = hour
		}
		if ok && first503 >= 0 && recovered < 0 {
			recovered = hour
		}
		if hour < downFrom && !ok {
			t.Fatalf("503 on day %d while ping was up", hour/24)
		}
	}
	// The last token was issued just before day 10 with exp = issue + 24 h; grace ends 30 d after exp.
	graceEnd := lastIssued.Add(24*time.Hour + simGrace)
	got := s.start.Add(time.Duration(first503) * time.Hour)
	if first503 < 0 || got.Before(graceEnd) || got.After(graceEnd.Add(25*time.Hour)) {
		t.Fatalf("first 503 at %v, grace ends %v", got, graceEnd)
	}
	// Ping returns at day 45: service returns within one timer period + one Guard period.
	if recovered < upAt || recovered > upAt+7+24 {
		t.Fatalf("recovered at hour %d, ping returned at hour %d", recovered, upAt)
	}
}

func TestSimRevoked(t *testing.T) {
	s := newSim(t)
	revokedFrom := 10 * 24
	var lastIssued time.Time
	first503 := -1
	for hour := 0; hour < len60; hour++ {
		mode := "ok"
		if hour >= revokedFrom {
			mode = "revoked"
		}
		ok := s.step(hour, mode)
		if hour == len60-1 {
			lastIssued = s.lastOK
		}
		if !ok && first503 < 0 {
			first503 = hour
		}
	}
	graceEnd := lastIssued.Add(24*time.Hour + simGrace)
	got := s.start.Add(time.Duration(first503) * time.Hour)
	if first503 < 0 || got.Before(graceEnd) || got.After(graceEnd.Add(25*time.Hour)) {
		t.Fatalf("revoked licence: first 503 at %v, want after last exp + 30 d = %v", got, graceEnd)
	}
}
