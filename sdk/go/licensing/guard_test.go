package licensing

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/licensing/licensetest"
)

func serve(g http.Handler, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w
}
func handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
}

func TestGuardAbsenceRule(t *testing.T) {
	t.Setenv(FileEnv, "unused")
	if err := os.Unsetenv(FileEnv); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	g := Guard(context.Background(), "paid", handler(), WithLogger(log.New(&logs, "", 0)))
	defer g.Close()
	for i := 0; i < 3; i++ {
		if serve(g, "/app").Code != 200 {
			t.Fatal("unset env blocked legacy CLI")
		}
	}
	if strings.Count(logs.String(), "unset") != 1 {
		t.Fatalf("want one warning: %s", logs.String())
	}
	pub, priv := syntheticKey(t)
	licensetest.UseKey(t, pub)
	now := time.Now()
	c := claimsAt(now)
	c["plugins"] = []string{"other"}
	other := tokenFor(t, priv, header(), c)
	c = claimsAt(now.Add(-EntitlementGracePeriod - time.Minute))
	for name, token := range map[string]string{"missing": "", "forged": "bad.token.signature", "other": other, "expired": tokenFor(t, priv, header(), c)} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "token")
			t.Setenv(FileEnv, path)
			if name != "missing" {
				if err := os.WriteFile(path, []byte(token), 0600); err != nil {
					t.Fatal(err)
				}
			}
			guard := Guard(context.Background(), "paid", handler())
			defer guard.Close()
			for _, route := range []string{"/", "/app", "/health/", "/healthz"} {
				w := serve(guard, route)
				if w.Code != 503 || w.Body.String() != `{"error":"entitlement_expired"}` || w.Header().Get("Content-Type") != "application/json" {
					t.Fatalf("unexpected failure response: %d %s", w.Code, w.Body.String())
				}
			}
			if serve(guard, "/health").Code != 200 {
				t.Fatal("health blocked")
			}
		})
	}
	t.Setenv(FileEnv, "")
	configured := Guard(context.Background(), "paid", handler())
	defer configured.Close()
	if serve(configured, "/").Code != 503 {
		t.Fatal("set empty env treated as absent")
	}
	t.Setenv(FileEnv, t.TempDir())
	unreadable := Guard(context.Background(), "paid", handler())
	defer unreadable.Close()
	if serve(unreadable, "/").Code != 503 {
		t.Fatal("directory accepted")
	}
}

type fakeClock struct {
	mu       sync.Mutex
	now      time.Time
	ticker   *fakeTicker
	interval time.Duration
}
type fakeTicker struct {
	ticks   chan time.Time
	stopped chan struct{}
	once    sync.Once
}

func (f *fakeClock) Now() time.Time                   { f.mu.Lock(); defer f.mu.Unlock(); return f.now }
func (f *fakeClock) NewTicker(d time.Duration) Ticker { f.interval = d; return f.ticker }
func (f *fakeTicker) C() <-chan time.Time             { return f.ticks }
func (f *fakeTicker) Stop()                           { f.once.Do(func() { close(f.stopped) }) }
func (f *fakeClock) advance(d time.Duration) {
	f.mu.Lock()
	f.now = f.now.Add(d)
	now := f.now
	f.mu.Unlock()
	f.ticker.ticks <- now
}

func TestGuardRereads24h(t *testing.T) {
	pub, priv := syntheticKey(t)
	licensetest.UseKey(t, pub)
	now := time.Unix(1800000000, 0)
	clock := &fakeClock{now: now, ticker: &fakeTicker{ticks: make(chan time.Time), stopped: make(chan struct{})}}
	path := setToken(t, "invalid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	g := Guard(ctx, "paid", handler(), WithClock(clock))
	defer g.Close()
	if clock.interval != 24*time.Hour {
		t.Fatalf("interval %s", clock.interval)
	}
	if serve(g, "/app").Code != 503 {
		t.Fatal("invalid token accepted")
	}
	if err := os.WriteFile(path, []byte(tokenFor(t, priv, header(), claimsAt(now))), 0600); err != nil {
		t.Fatal(err)
	}
	if serve(g, "/app").Code != 503 {
		t.Fatal("file read before refresh")
	}
	clock.advance(24 * time.Hour)
	waitCachedValidity(t, g, true)
	waitStatus(t, g, 200)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if serve(g, "/app").Code != 200 {
		t.Fatal("cached authorization lost before refresh")
	}
	clock.advance(24 * time.Hour)
	waitStatus(t, g, 503)
	cancel()
	g.Close()
	g.Close()
	select {
	case <-clock.ticker.stopped:
	default:
		t.Fatal("ticker not stopped")
	}
}

func waitCachedValidity(t *testing.T, g *Middleware, valid bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		g.mu.RLock()
		got := g.valid
		g.mu.RUnlock()
		if got == valid {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("cached validity never reached %t", valid)
}

func waitStatus(t *testing.T, g *Middleware, status int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if serve(g, "/").Code == status {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("never reached status %d", status)
}

func TestGuardFailedSnapshotRetriesAfterMinute(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "invalid", true: "missing"}[missing], func(t *testing.T) {
			pub, priv := syntheticKey(t)
			licensetest.UseKey(t, pub)
			now := time.Unix(1800000000, 0)
			clock := &fakeClock{now: now, ticker: &fakeTicker{ticks: make(chan time.Time), stopped: make(chan struct{})}}
			path := setToken(t, "invalid")
			if missing {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			g := Guard(context.Background(), "paid", handler(), WithClock(clock))
			defer g.Close()
			if serve(g, "/app").Code != 503 {
				t.Fatal("invalid snapshot accepted")
			}
			if err := os.WriteFile(path, []byte(tokenFor(t, priv, header(), claimsAt(now))), 0600); err != nil {
				t.Fatal(err)
			}
			clock.mu.Lock()
			clock.now = now.Add(59 * time.Second)
			clock.mu.Unlock()
			if serve(g, "/app").Code != 503 {
				t.Fatal("retry exceeded once per minute")
			}
			clock.mu.Lock()
			clock.now = now.Add(60 * time.Second)
			clock.mu.Unlock()
			if serve(g, "/app").Code != 200 {
				t.Fatal("valid replacement not recovered without ticker")
			}
			clock.mu.Lock()
			clock.now = now.Add(61 * time.Second)
			clock.mu.Unlock()
			if serve(g, "/app").Code != 200 {
				t.Fatal("replacement lost after 61 seconds without ticker")
			}
		})
	}
}

func TestGuardHealthMethodsAndPaths(t *testing.T) {
	setToken(t, "invalid")
	g := Guard(context.Background(), "paid", handler())
	defer g.Close()
	for _, tc := range []struct {
		method, path string
		status       int
	}{
		{"GET", "/health", 200}, {"HEAD", "/health", 200}, {"POST", "/health", 503},
		{"GET", "/HEALTH", 503}, {"GET", "/health/x", 503}, {"GET", "/health/../app", 503},
		{"GET", "//health", 503}, {"GET", "/health?x=1", 200},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			g.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
			if w.Code != tc.status {
				t.Fatalf("got %d, want %d", w.Code, tc.status)
			}
		})
	}
}

func TestGuardCachesSignatureAndSlugUntilReload(t *testing.T) {
	pub, priv := syntheticKey(t)
	licensetest.UseKey(t, pub)
	setToken(t, tokenFor(t, priv, header(), claimsAt(time.Now())))
	g := Guard(context.Background(), "paid", handler())
	defer g.Close()
	replacement, _ := syntheticKey(t)
	licensetest.UseKey(t, replacement)
	if serve(g, "/app").Code != 200 {
		t.Fatal("cached snapshot reverified its signature on request")
	}
	g.reload()
	if serve(g, "/app").Code != 503 {
		t.Fatal("reload retained stale signature decision")
	}
}

func TestGuardCachedSignedExpiryBoundary(t *testing.T) {
	pub, priv := syntheticKey(t)
	licensetest.UseKey(t, pub)
	now := time.Unix(1800000000, 0)
	clock := &fakeClock{now: now, ticker: &fakeTicker{ticks: make(chan time.Time), stopped: make(chan struct{})}}
	setToken(t, tokenFor(t, priv, header(), claimsAt(now)))
	g := Guard(context.Background(), "paid", handler(), WithClock(clock))
	defer g.Close()
	for _, tc := range []struct {
		at     time.Time
		status int
	}{
		{now.Add(EntitlementGracePeriod), 200},
		{now.Add(EntitlementGracePeriod + time.Nanosecond), 503},
		{now.Add(EntitlementGracePeriod + time.Second), 503},
	} {
		clock.mu.Lock()
		clock.now = tc.at
		clock.mu.Unlock()
		if got := serve(g, "/app").Code; got != tc.status {
			t.Fatalf("at %s got %d, want %d", tc.at, got, tc.status)
		}
	}
}
