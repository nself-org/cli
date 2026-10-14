package licensing

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// Clock supplies a testable current time and owned refresh ticker.
type Clock interface {
	Now() time.Time
	NewTicker(time.Duration) Ticker
}

// Ticker supplies refresh events and releases timer resources.
type Ticker interface {
	C() <-chan time.Time
	Stop()
}
type realClock struct{}
type realTicker struct{ *time.Ticker }

// Purpose: provide wall time. Inputs: none. Outputs: current time.
// Constraints: production clock. SPORT: plugin.entitlement-token.
func (realClock) Now() time.Time { return time.Now() }

// Purpose: own refresh timer. Inputs: interval. Outputs: stoppable ticker.
// Constraints: released by guard. SPORT: plugin.entitlement-token.
func (realClock) NewTicker(d time.Duration) Ticker { return realTicker{time.NewTicker(d)} }

// Purpose: expose events. Inputs: none. Outputs: ticker channel.
// Constraints: stopped on shutdown. SPORT: plugin.entitlement-token.
func (t realTicker) C() <-chan time.Time { return t.Ticker.C }

// GuardOption configures guard infrastructure without changing signature policy.
type GuardOption func(*Middleware)

// Purpose: inject time. Inputs: clock. Outputs: guard option.
// Constraints: nil retains wall clock. SPORT: plugin.entitlement-token.
func WithClock(clock Clock) GuardOption {
	return func(g *Middleware) {
		if clock != nil {
			g.clock = clock
		}
	}
}

// Purpose: direct absence warnings. Inputs: logger. Outputs: guard option.
// Constraints: nil retains standard logger. SPORT: plugin.entitlement-token.
func WithLogger(logger *log.Logger) GuardOption {
	return func(g *Middleware) {
		if logger != nil {
			g.logger = logger
		}
	}
}

// Middleware owns a refresh goroutine; cancel its context or call Close at shutdown.
type Middleware struct {
	slug, path string
	absent     bool
	next       http.Handler
	clock      Clock
	logger     *log.Logger
	mu         sync.RWMutex
	deadline   int64
	valid      bool
	lastReload time.Time
	cancel     context.CancelFunc
	done       chan struct{}
}

// Purpose: guard every application route. Inputs: owner context, slug, handler, options.
// Outputs: owned middleware. Constraints: only /health bypasses configured-file checks;
// unset env serves with one warning until the v1.6.0 sunset. SPORT: plugin.entitlement-token.
func Guard(ctx context.Context, slug string, next http.Handler, options ...GuardOption) *Middleware {
	path, present := os.LookupEnv(FileEnv)
	g := &Middleware{slug: slug, path: path, absent: !present, next: next, clock: realClock{}, logger: log.Default(), done: make(chan struct{})}
	for _, option := range options {
		option(g)
	}
	ctx, g.cancel = context.WithCancel(ctx)
	if g.absent {
		g.logger.Print("NSELF_ENTITLEMENT_FILE unset: runtime entitlement check bypassed for legacy CLI; support ends at v1.6.0")
		close(g.done)
		return g
	}
	g.reload()
	ticker := g.clock.NewTicker(24 * time.Hour)
	go g.refresh(ctx, ticker)
	return g
}

// Purpose: own polling lifetime. Inputs: owner context and ticker.
// Outputs: refreshed file snapshots. Constraints: no detached timers. SPORT: plugin.entitlement-token.
func (g *Middleware) refresh(ctx context.Context, ticker Ticker) {
	defer close(g.done)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			g.reload()
		}
	}
}

// Purpose: replace cached token. Inputs: configured path. Outputs: snapshot.
// Constraints: read failures erase authorization. SPORT: plugin.entitlement-token.
func (g *Middleware) reload() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reloadLocked(g.clock.Now())
}

// Purpose: verify a new snapshot once. Inputs: current time. Outputs: cached decision.
// Constraints: caller holds mu; failures erase authorization. SPORT: plugin.entitlement-token.
func (g *Middleware) reloadLocked(now time.Time) {
	g.lastReload = now
	g.valid = false
	data, err := readTokenFile(g.path)
	if err == nil {
		g.deadline, err = verifiedDeadline(strings.TrimSpace(string(data)), g.slug)
		g.valid = err == nil
	}
}

// Purpose: enforce route authorization. Inputs: HTTP request. Outputs: downstream or 503.
// Constraints: expiration rechecked on every request. SPORT: plugin.entitlement-token.
func (g *Middleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if g.absent || (r.URL.Path == "/health" && (r.Method == http.MethodGet || r.Method == http.MethodHead)) {
		g.next.ServeHTTP(w, r)
		return
	}
	g.mu.Lock()
	now := g.clock.Now()
	if !g.authorized(now) && now.Sub(g.lastReload) >= time.Minute {
		g.reloadLocked(now)
	}
	authorized := g.authorized(now)
	g.mu.Unlock()
	if !authorized {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"entitlement_expired"}`))
		return
	}
	g.next.ServeHTTP(w, r)
}

// Purpose: check cached signed expiry. Inputs: time. Outputs: authorization.
// Constraints: caller holds mu; grace includes its exact final instant. SPORT: plugin.entitlement-token.
func (g *Middleware) authorized(now time.Time) bool {
	return g.valid && (now.Unix() < g.deadline || (now.Unix() == g.deadline && now.Nanosecond() == 0))
}

// Purpose: release guard resources. Inputs: none. Outputs: joined refresh worker.
// Constraints: idempotent and safe concurrently. SPORT: plugin.entitlement-token.
func (g *Middleware) Close() { g.cancel(); <-g.done }
