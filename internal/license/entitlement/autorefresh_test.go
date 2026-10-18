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
	"github.com/nself-org/cli/internal/maintenance"
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
	oldE, oldR, oldB, oldN := timerEnsure, timerRemove, selfBinary, timerNotes
	t.Cleanup(func() { timerEnsure, timerRemove, selfBinary, timerNotes = oldE, oldR, oldB, oldN })
	timerEnsure = func(dir, bin string) (maintenance.Result, error) {
		f.ensured = append(f.ensured, dir+"|"+bin)
		res := maintenance.Result{Backend: "cron", Changed: f.changed}
		f.changed = false
		return res, nil
	}
	timerRemove = func(dir string) (maintenance.RemoveResult, error) {
		f.removed = append(f.removed, dir)
		return maintenance.RemoveResult{Removed: !f.rootRequired, RootRequired: f.rootRequired}, nil
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
			fakeTimer(t)
			project := t.TempDir()
			if _, err := RunScheduled(context.Background(), project); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(Path(project))
			p.set(tc.mode, time.Now().Add(time.Hour))
			_, err := RunScheduled(context.Background(), project)
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
