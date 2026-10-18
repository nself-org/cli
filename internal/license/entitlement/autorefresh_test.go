package entitlement

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/license"
)

// arPing is a synthetic ping.nself.org that issues 24 h JWTs on a controllable clock.
type arPing struct {
	mu      sync.Mutex
	mode    string // ok, 5xx, revoked, invalid, badjwt
	now     time.Time
	key     string
	pub     ed25519.PublicKey
	priv    ed25519.PrivateKey
	other   ed25519.PrivateKey
	plugins []string
	hits    int
	server  *httptest.Server
}

func (p *arPing) set(mode string, now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.mode, p.now = mode, now
}

func (p *arPing) jwt(priv ed25519.PrivateKey) string {
	enc := base64.RawURLEncoding
	hdr := enc.EncodeToString([]byte(`{"alg":"EdDSA","kid":"synthetic"}`))
	claims, _ := json.Marshal(map[string]any{"sub": license.HashKey(p.key), "tier": "plus", "plugins": p.plugins,
		"iss": "ping.nself.org", "aud": "nself-cli", "iat": p.now.Unix(), "exp": p.now.Add(24 * time.Hour).Unix()})
	unsigned := hdr + "." + enc.EncodeToString(claims)
	return unsigned + "." + enc.EncodeToString(ed25519.Sign(priv, []byte(unsigned)))
}

func (p *arPing) handle(w http.ResponseWriter, _ *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.hits++
	var body []byte
	switch p.mode {
	case "5xx":
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	case "revoked":
		body, _ = json.Marshal(map[string]any{"valid": false, "reason": "license revoked"})
	case "invalid":
		body, _ = json.Marshal(map[string]any{"valid": false, "reason": "unknown license key"})
	case "badjwt":
		body, _ = json.Marshal(map[string]any{"valid": true, "tier": "plus", "plugins_allowed": p.plugins, "jwt": p.jwt(p.other)})
	default:
		body, _ = json.Marshal(map[string]any{"valid": true, "tier": "plus", "plugins_allowed": p.plugins, "jwt": p.jwt(p.priv)})
	}
	w.Header().Set("X-NSelf-License-Sig", hex.EncodeToString(ed25519.Sign(p.priv, body)))
	_, _ = w.Write(body)
}

// newArPing builds the ping, a private HOME with one licensed plugin in the default plugin
// directory, a cache seeded with a day-old token, and switches v1.5 on.
func newArPing(t *testing.T, start time.Time, plugins ...string) *arPing {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("NSELF_PLUGIN_DIR", "")
	t.Setenv("NSELF_ENTITLEMENT_AUTOREFRESH", "")
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "synthetic-license-key")
	for i := 1; i <= 10; i++ {
		t.Setenv("NSELF_LICENSE_KEY_"+strconv.Itoa(i), "")
	}
	t.Setenv("LICENSE_CACHE_PATH", filepath.Join(t.TempDir(), "license.json"))
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	old := license.PingKeys
	license.PingKeys = []license.PingKey{{ID: "synthetic", Public: pub}}
	t.Cleanup(func() { license.PingKeys = old })
	p := &arPing{mode: "ok", now: start, key: "synthetic-license-key", pub: pub, priv: priv, other: other, plugins: plugins}
	p.server = httptest.NewServer(http.HandlerFunc(p.handle))
	t.Cleanup(p.server.Close)
	t.Setenv("NSELF_PING_API_URL", p.server.URL)
	for _, name := range plugins {
		dir := filepath.Join(home, ".nself", "plugins", name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		manifest := `{"manifest_version":2,"license":"licensed","name":"` + name + `"}`
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// seedCache primes the licence cache with the token the ping issues at p.now.
func (p *arPing) seedCache(t *testing.T) {
	t.Helper()
	p.mu.Lock()
	token := p.jwt(p.priv)
	p.mu.Unlock()
	body, _ := json.Marshal(map[string]any{"valid": true, "tier": "plus", "plugins_allowed": p.plugins, "jwt": token})
	entry := &license.CacheEntry{KeyHash: license.HashKey(p.key), Tier: "plus", PluginsAllowed: p.plugins, JWT: token,
		RawBody: string(body), BodySig: hex.EncodeToString(ed25519.Sign(p.priv, body)), FetchedAt: time.Now().Unix()}
	if err := license.WriteCache(entry); err != nil {
		t.Fatal(err)
	}
}

// arTimer replaces the host timer API and the notice sink with recorders.
type arTimer struct {
	ensured, removed []string
	changed          bool
	rootRequired     bool
	notes            bytes.Buffer
}

func fakeTimer(t *testing.T) *arTimer {
	t.Helper()
	f := &arTimer{changed: true}
	oldH, oldB, oldN := HostTimers, selfBinary, timerNotes
	t.Cleanup(func() { HostTimers, selfBinary, timerNotes = oldH, oldB, oldN })
	HostTimers = Timers{
		Ensure: func(dir, bin string) (TimerInstall, error) {
			f.ensured = append(f.ensured, dir+"|"+bin)
			res := TimerInstall{Backend: "cron", Changed: f.changed}
			f.changed = false
			return res, nil
		},
		Remove: func(dir string) (TimerRemoval, error) {
			f.removed = append(f.removed, dir)
			return TimerRemoval{Removed: !f.rootRequired, RootRequired: f.rootRequired}, nil
		},
	}
	selfBinary = func() (string, error) { return "/usr/local/bin/nself", nil }
	timerNotes = &f.notes
	return f
}

func TestAutorefreshEnsuresTimer(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		p := newArPing(t, time.Now(), "chat")
		p.seedCache(t)
		timer := fakeTimer(t)
		project := t.TempDir()
		for i := 0; i < 2; i++ {
			if err := Refresh(context.Background(), project); err != nil {
				t.Fatal(err)
			}
		}
		if !compat.V15() {
			if len(timer.ensured)+len(timer.removed) != 0 || timer.notes.Len() != 0 {
				t.Fatalf("v1.4 touched the timer: %v %v %q", timer.ensured, timer.removed, timer.notes.String())
			}
			return
		}
		want := project + "|/usr/local/bin/nself"
		if len(timer.ensured) != 2 || timer.ensured[0] != want || len(timer.removed) != 0 {
			t.Fatalf("ensured %v, removed %v", timer.ensured, timer.removed)
		}
		if lines := strings.Count(timer.notes.String(), "\n"); lines != 1 || !strings.Contains(timer.notes.String(), "installed") {
			t.Fatalf("want exactly one install line (the second call changed nothing), got %q", timer.notes.String())
		}
		// A failed refresh (ping unreachable, no usable token) still keeps the timer: it is the repair path.
		_ = os.Remove(os.Getenv("LICENSE_CACHE_PATH"))
		_ = Refresh(context.Background(), project)
		if len(timer.ensured) != 3 {
			t.Fatalf("failed refresh did not ensure the timer: %v", timer.ensured)
		}
	})
}

func TestAutorefreshRemovesWhenNoLicensed(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		p := newArPing(t, time.Now(), "chat")
		p.seedCache(t)
		timer := fakeTimer(t)
		if err := os.RemoveAll(filepath.Join(os.Getenv("HOME"), ".nself", "plugins", "chat")); err != nil {
			t.Fatal(err)
		}
		project := t.TempDir()
		if err := Refresh(context.Background(), project); err != nil {
			t.Fatal(err)
		}
		out, err := RunScheduled(context.Background(), project)
		if !compat.V15() {
			if err != nil || !out.Skipped || len(timer.removed) != 0 {
				t.Fatalf("v1.4: %+v %v %v", out, err, timer.removed)
			}
			return
		}
		if err != nil || !out.Removed || out.RootRequired || len(timer.ensured) != 0 || len(timer.removed) != 2 || p.hits != 0 {
			t.Fatalf("outcome %+v err %v ensured %v removed %v pings %d", out, err, timer.ensured, timer.removed, p.hits)
		}
		timer.rootRequired = true
		if out, _ = RunScheduled(context.Background(), project); !out.RootRequired || out.Removed {
			t.Fatalf("root systemd units must ask for exit 78: %+v", out)
		}
	})
}

func TestAutorefreshDisabledInstallsNothing(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		p := newArPing(t, time.Now(), "chat")
		p.seedCache(t)
		timer := fakeTimer(t)
		t.Setenv("NSELF_ENTITLEMENT_AUTOREFRESH", "false")
		project := t.TempDir()
		if err := Refresh(context.Background(), project); err != nil {
			t.Fatal(err)
		}
		if len(timer.ensured) != 0 {
			t.Fatalf("disabled autorefresh installed a timer: %v", timer.ensured)
		}
		if out, err := RunScheduled(context.Background(), project); compat.V15() && (err != nil || !out.Removed || out.Reason != "autorefresh disabled") {
			t.Fatalf("scheduled run with autorefresh disabled: %+v %v", out, err)
		}
		// A custom plugin directory is invisible to the timer: no timer, no warning.
		t.Setenv("NSELF_ENTITLEMENT_AUTOREFRESH", "")
		t.Setenv("NSELF_PLUGIN_DIR", filepath.Join(os.Getenv("HOME"), ".nself", "plugins")+"-custom")
		timer.ensured = nil
		timer.notes.Reset()
		_ = Refresh(context.Background(), project)
		if len(timer.ensured) != 0 || timer.notes.Len() != 0 {
			t.Fatalf("custom plugin dir: ensured %v notes %q", timer.ensured, timer.notes.String())
		}
	})
}

func TestScheduledRefreshFailureClasses(t *testing.T) {
	for _, tc := range []struct{ mode, class string }{
		{"5xx", ClassPingUnavailable}, {"invalid", ClassRejected}, {"revoked", ClassRejected}, {"badjwt", ClassTokenVerify},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			compattest.Set(t, true)
			p := newArPing(t, time.Now(), "chat")
			p.seedCache(t)
			timer := fakeTimer(t)
			oldClock := refreshClock
			refreshClock = arClock{p}
			t.Cleanup(func() { refreshClock = oldClock })
			project := t.TempDir()
			start := time.Now()
			p.set("ok", start.Add(time.Hour))
			if _, err := RunScheduled(context.Background(), project); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(Path(project))
			// The cache now holds an OLDER token than the file: a failed run must not write it.
			p.set("ok", start)
			p.seedCache(t)
			p.set(tc.mode, start.Add(2*time.Hour))
			_, err := RunScheduled(context.Background(), project)
			if len(timer.removed) != 0 || len(timer.ensured) != 0 {
				t.Fatalf("a failed refresh touched the timer: ensured %v removed %v", timer.ensured, timer.removed)
			}
			var failed *RefreshError
			if !errors.As(err, &failed) || failed.Class != tc.class {
				t.Fatalf("err = %v, want class %s", err, tc.class)
			}
			after, _ := os.ReadFile(Path(project))
			if len(before) == 0 || !bytes.Equal(before, after) {
				t.Fatal("a failed refresh must leave the old entitlement file intact")
			}
			if strings.Contains(err.Error(), "eyJ") || strings.Contains(err.Error(), p.key) {
				t.Fatalf("error text leaks: %v", err)
			}
		})
	}
}

func TestStableBinaryPrefersThePathEntry(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	real := filepath.Join(root, "Cellar", "1.5.0", "bin", "nself-stable-fixture")
	link := filepath.Join(root, "bin", "nself-stable-fixture")
	for _, d := range []string{filepath.Dir(real), filepath.Dir(link)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(real, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(link))
	if got := stableBinary(real); got != link {
		t.Fatalf("stableBinary = %q, want the PATH entry %q", got, link)
	}
	t.Setenv("PATH", t.TempDir())
	if got := stableBinary(real); got != real {
		t.Fatalf("not on PATH: stableBinary = %q, want %q", got, real)
	}
}

func TestScheduledRemoveFailureIsReported(t *testing.T) {
	compattest.Set(t, true)
	p := newArPing(t, time.Now(), "chat")
	p.seedCache(t)
	fakeTimer(t)
	if err := os.RemoveAll(filepath.Join(os.Getenv("HOME"), ".nself", "plugins", "chat")); err != nil {
		t.Fatal(err)
	}
	HostTimers.Remove = func(string) (TimerRemoval, error) { return TimerRemoval{}, errors.New("crontab rejected") }
	_, err := RunScheduled(context.Background(), t.TempDir())
	var failed *RefreshError
	if !errors.As(err, &failed) || failed.Class != ClassTimerRemove {
		t.Fatalf("err = %v, want class %s", err, ClassTimerRemove)
	}
}

// TestScheduledRefreshStaysQuietOnStaleCache: the fail-open warning of license.Validate must not
// reach stderr (--quiet allows one line per run).
func TestScheduledRefreshStaysQuietOnStaleCache(t *testing.T) {
	compattest.Set(t, true)
	p := newArPing(t, time.Now(), "chat")
	p.seedCache(t)
	entry, err := license.ReadCache()
	if err != nil || entry == nil {
		t.Fatal(err)
	}
	entry.FetchedAt = time.Now().Add(-3 * 24 * time.Hour).Unix()
	if err := license.WriteCache(entry); err != nil {
		t.Fatal(err)
	}
	fakeTimer(t)
	p.set("5xx", time.Now())
	r, w, _ := os.Pipe()
	old := os.Stderr
	os.Stderr = w
	_, runErr := RunScheduled(context.Background(), t.TempDir())
	os.Stderr = old
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	var failed *RefreshError
	if !errors.As(runErr, &failed) || failed.Class != ClassPingUnavailable {
		t.Fatalf("err = %v", runErr)
	}
	if buf.Len() != 0 {
		t.Fatalf("RunScheduled wrote to stderr: %q", buf.String())
	}
}

// TestScheduledRefreshNeedsAPersistedReply: when the licence cache cannot be written the reply is
// lost and the cache still holds the old token, so the run must fail instead of reporting it.
func TestScheduledRefreshNeedsAPersistedReply(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	compattest.Set(t, true)
	p := newArPing(t, time.Now(), "chat")
	p.seedCache(t)
	fakeTimer(t)
	project := t.TempDir()
	if _, err := RunScheduled(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(Path(project))
	entry, _ := license.ReadCache()
	entry.FetchedAt = time.Now().Add(-time.Hour).Unix() // the cache the run will fail to replace
	if err := license.WriteCache(entry); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(os.Getenv("LICENSE_CACHE_PATH"))
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	p.set("ok", time.Now().Add(time.Minute))
	_, err := RunScheduled(context.Background(), project)
	var failed *RefreshError
	if !errors.As(err, &failed) || failed.Class != ClassCacheWrite {
		t.Fatalf("err = %v, want class %s", err, ClassCacheWrite)
	}
	if after, _ := os.ReadFile(Path(project)); !bytes.Equal(before, after) {
		t.Fatal("entitlement file changed")
	}
}

// TestScheduledRemovalWithRootUnitsStillExits78: a failing crontab step must not hide RootRequired.
func TestScheduledRemovalWithRootUnitsStillExits78(t *testing.T) {
	compattest.Set(t, true)
	p := newArPing(t, time.Now(), "chat")
	p.seedCache(t)
	fakeTimer(t)
	if err := os.RemoveAll(filepath.Join(os.Getenv("HOME"), ".nself", "plugins", "chat")); err != nil {
		t.Fatal(err)
	}
	HostTimers.Remove = func(string) (TimerRemoval, error) {
		return TimerRemoval{RootRequired: true}, errors.New("crontab -l: you are not allowed to use this program")
	}
	out, err := RunScheduled(context.Background(), t.TempDir())
	if err != nil || !out.RootRequired {
		t.Fatalf("outcome %+v err %v", out, err)
	}
}
