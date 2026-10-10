package updatecheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/version"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func versionServer(t *testing.T, body string, hits *int, hdr *http.Header) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			*hits++
		}
		if hdr != nil {
			*hdr = r.Header.Clone()
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func guards(t *testing.T, c *clock, env map[string]string) Guards {
	t.Helper()
	return Guards{
		TTY:     func() bool { return true },
		Env:     func(k string) string { return env[k] },
		Now:     c.now,
		Path:    filepath.Join(t.TempDir(), "cache", "update-check.json"),
		Current: "1.4.12",
	}
}

func TestCacheCorruptIsReplacedAndMode0600(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d", "update-check.json")
	if got := Load(p); got != (Cache{}) {
		t.Fatalf("missing file: %+v", got)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Load(p); got != (Cache{}) {
		t.Fatalf("corrupt file must load as zero: %+v", got)
	}
	want := Cache{CheckedAt: time.Unix(1000, 0).UTC(), Latest: "1.5.0", LastError: "x", HintedAt: time.Unix(2000, 0).UTC()}
	if err := Save(p, want); err != nil {
		t.Fatal(err)
	}
	if got := Load(p); got != want {
		t.Fatalf("round trip: %+v != %+v", got, want)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", st.Mode().Perm())
	}
	ents, _ := os.ReadDir(filepath.Dir(p))
	if len(ents) != 1 {
		t.Fatalf("temp files left behind: %v", ents)
	}
}

func TestRefreshRequestCarriesOnlyUserAgent(t *testing.T) {
	var hdr http.Header
	srv := versionServer(t, `{"latestCliVersion":"v1.5.0"}`, nil, &hdr)
	got, err := Refresh(context.Background(), NewClient(), srv.URL)
	if err != nil || got != "1.5.0" {
		t.Fatalf("Refresh = %q, %v", got, err)
	}
	var keys []string
	for k := range hdr {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if strings.Join(keys, ",") != "User-Agent" {
		t.Fatalf("request headers = %v, want only User-Agent", keys)
	}
	if ua := hdr.Get("User-Agent"); ua != "nself/"+version.GetVersion() {
		t.Fatalf("User-Agent = %q", ua)
	}
}

func TestRefreshRejectsBadAnswers(t *testing.T) {
	for _, body := range []string{``, `{}`, `{"latestCliVersion":""}`, `{"latestCliVersion":"abc"}`,
		`{"latestCliVersion":"1.5"}`, `{"latestCliVersion":"1.5.0-rc1"}`, `{"latestCliVersion":"1.5.0+x"}`,
		`{"latestCliVersion":"1.-5.0"}`, `{"latestCliVersion":"1.5.1234567890"}`, `{"latestCliVersion":"1..0"}`, `not json`, `{"latestCliVersion":7}`} {
		srv := versionServer(t, body, nil, nil)
		if got, err := Refresh(context.Background(), NewClient(), srv.URL); err == nil {
			t.Errorf("body %q accepted as %q", body, got)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"latestCliVersion":"9.9.9"}`))
	}))
	t.Cleanup(srv.Close)
	if _, err := Refresh(context.Background(), NewClient(), srv.URL); err == nil {
		t.Error("HTTP 500 accepted")
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		latest, cur string
		want        bool
	}{
		{"1.5.0", "1.4.12", true}, {"1.4.13", "1.4.12", true}, {"2.0.0", "1.99.99", true},
		{"1.4.12", "1.4.12", false}, {"1.4.11", "1.4.12", false}, {"1.10.0", "1.9.9", true},
		{"", "1.4.12", false}, {"x", "1.4.12", false}, {"1.5.0", "dev", false}, {"1.5.0", "", false},
		{"1.5.0-rc1", "1.4.12", false},
	}
	for _, c := range cases {
		if got := newer(c.latest, c.cur); got != c.want {
			t.Errorf("newer(%q,%q) = %v, want %v", c.latest, c.cur, got, c.want)
		}
	}
}

func TestRefreshAndHintLifecycleWithFakeClock(t *testing.T) {
	hits := 0
	srv := versionServer(t, `{"latestCliVersion":"1.5.0"}`, &hits, nil)
	clk := &clock{t: time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)}
	g := guards(t, clk, map[string]string{EnvOptIn: "1"})
	g.URL, g.Client = srv.URL, NewClient()

	if _, ok := Hint(g); ok {
		t.Fatal("hint before any refresh")
	}
	done := MaybeStartRefresh(g)
	if done == nil {
		t.Fatal("stale cache must start a refresh")
	}
	<-done
	c := Load(g.Path)
	if c.Latest != "1.5.0" || !c.CheckedAt.Equal(clk.t) || c.LastError != "" {
		t.Fatalf("cache after refresh: %+v", c)
	}
	line, ok := Hint(g)
	if !ok || !strings.Contains(line, "1.4.12") || !strings.Contains(line, "1.5.0") || !strings.Contains(line, "nself update") {
		t.Fatalf("hint = %q %v", line, ok)
	}
	if _, ok := Hint(g); ok {
		t.Fatal("second hint within 24h")
	}
	if MaybeStartRefresh(g) != nil || hits != 1 {
		t.Fatalf("fresh cache must not refresh again (hits=%d)", hits)
	}
	clk.t = clk.t.Add(23 * time.Hour)
	if _, ok := Hint(g); ok {
		t.Fatal("hint at 23h")
	}
	clk.t = clk.t.Add(2 * time.Hour)
	if _, ok := Hint(g); !ok {
		t.Fatal("no hint after 24h")
	}
	done = MaybeStartRefresh(g)
	if done == nil {
		t.Fatal("cache older than 24h must refresh")
	}
	<-done
	if hits != 2 {
		t.Fatalf("hits = %d, want 2", hits)
	}
}

func TestFailedRefreshIsRecordedAndKeepsLatest(t *testing.T) {
	srv := versionServer(t, `{"latestCliVersion":"junk"}`, nil, nil)
	clk := &clock{t: time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)}
	g := guards(t, clk, map[string]string{EnvOptIn: "1"})
	g.URL, g.Client = srv.URL, NewClient()
	if err := Save(g.Path, Cache{CheckedAt: clk.t.Add(-30 * time.Hour), Latest: "1.5.0"}); err != nil {
		t.Fatal(err)
	}
	<-MaybeStartRefresh(g)
	c := Load(g.Path)
	if c.Latest != "1.5.0" || c.LastError == "" || !c.CheckedAt.Equal(clk.t) {
		t.Fatalf("cache after failed refresh: %+v", c)
	}
	if MaybeStartRefresh(g) != nil {
		t.Fatal("a failed refresh must not be retried within 24h")
	}
}

func TestHangingServerIsAbandonedWithinBudget(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	clk := &clock{t: time.Now()}
	g := guards(t, clk, map[string]string{EnvOptIn: "1"})
	g.URL, g.Client = srv.URL, NewClient()
	start := time.Now()
	done := MaybeStartRefresh(g)
	if done == nil {
		t.Fatal("no refresh started")
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("MaybeStartRefresh blocked %v", d)
	}
	<-done
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("refresh took %v, budget is %v", d, Budget)
	}
	if c := Load(g.Path); c.LastError == "" || c.Latest != "" {
		t.Fatalf("hang must be recorded as an error: %+v", c)
	}
}

func TestAllowedGuards(t *testing.T) {
	clk := &clock{t: time.Now()}
	base := map[string]string{EnvOptIn: "1"}
	cases := []struct {
		name string
		env  map[string]string
		mut  func(*Guards)
		want bool
	}{
		{"opted in", base, nil, true},
		{"no opt-in", map[string]string{}, nil, false},
		{"opt-in 0", map[string]string{EnvOptIn: "0"}, nil, false},
		{"opt-in true is not 1", map[string]string{EnvOptIn: "true"}, nil, false},
		{"json", base, func(g *Guards) { g.JSON = true }, false},
		{"non-tty", base, func(g *Guards) { g.TTY = func() bool { return false } }, false},
		{"nil tty", base, func(g *Guards) { g.TTY = nil }, false},
		{"CI=true", map[string]string{EnvOptIn: "1", "CI": "true"}, nil, false},
		{"CI=false", map[string]string{EnvOptIn: "1", "CI": "false"}, nil, true},
		{"GITHUB_ACTIONS", map[string]string{EnvOptIn: "1", "GITHUB_ACTIONS": "true"}, nil, false},
		{"GITLAB_CI", map[string]string{EnvOptIn: "1", "GITLAB_CI": "true"}, nil, false},
		{"BUILDKITE", map[string]string{EnvOptIn: "1", "BUILDKITE": "true"}, nil, false},
		{"JENKINS_URL", map[string]string{EnvOptIn: "1", "JENKINS_URL": "http://j"}, nil, false},
		{"no cache path", base, func(g *Guards) { g.Path = "" }, false},
	}
	for _, c := range cases {
		g := guards(t, clk, c.env)
		if c.mut != nil {
			c.mut(&g)
		}
		if got := Allowed(g); got != c.want {
			t.Errorf("%s: Allowed = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNoRequestWhenNotAllowed(t *testing.T) {
	hits := 0
	srv := versionServer(t, `{"latestCliVersion":"1.5.0"}`, &hits, nil)
	clk := &clock{t: time.Now()}
	g := guards(t, clk, map[string]string{})
	g.URL, g.Client = srv.URL, NewClient()
	if MaybeStartRefresh(g) != nil || hits != 0 {
		t.Fatal("request without opt-in")
	}
	if _, err := os.Stat(g.Path); err == nil {
		t.Fatal("cache file written without opt-in")
	}
}

func TestHintIgnoresInvalidAndNotNewerCache(t *testing.T) {
	clk := &clock{t: time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)}
	for _, latest := range []string{"", "garbage", "1.4.12", "1.4.0", "1.5.0-rc1"} {
		g := guards(t, clk, map[string]string{EnvOptIn: "1"})
		if err := Save(g.Path, Cache{CheckedAt: clk.t, Latest: latest}); err != nil {
			t.Fatal(err)
		}
		if line, ok := Hint(g); ok {
			t.Errorf("latest %q gave hint %q", latest, line)
		}
	}
}

func TestHintNotRepeatedWhenCacheCannotBeSaved(t *testing.T) {
	clk := &clock{t: time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)}
	g := guards(t, clk, map[string]string{EnvOptIn: "1"})
	if err := Save(g.Path, Cache{CheckedAt: clk.t, Latest: "1.5.0"}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(g.Path)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory modes")
	}
	if line, ok := Hint(g); ok {
		t.Fatalf("hint %q printed although hinted_at could not be recorded", line)
	}
}
