package commands

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/httptimeout"
)

// Tests for the doctor ai client's local-only token rule (D15, P7-PLUG-73).
// Every secret here is a fake. Servers record X-Internal-Token themselves.

const (
	fakeAISecret = "fake-plug73-secret-not-real"
	aiChildEnv   = "AI_PROXY_CHILD"
	aiChildDone  = "PLUG73-CHILD-LEGS-DONE"
)

// tokenRecorder is an httptest handler that stores the token header it saw.
type tokenRecorder struct {
	mu   sync.Mutex
	toks []string
}

func (r *tokenRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	r.toks = append(r.toks, req.Header.Get("X-Internal-Token"))
	r.mu.Unlock()
	_, _ = w.Write([]byte("ok"))
}

func (r *tokenRecorder) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.toks...)
}

// dialTo returns a dial hook that sends every connection to addr.
func dialTo(addr string) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}
}

// stubAISeams injects the resolver and dial hook and restores them afterwards.
func stubAISeams(t *testing.T, lookup func(context.Context, string) ([]net.IPAddr, error), dial func(context.Context, string, string) (net.Conn, error)) {
	t.Helper()
	oldLookup, oldDial := aiLookupIP, aiDialContext
	aiLookupIP, aiDialContext = lookup, dial
	t.Cleanup(func() { aiLookupIP, aiDialContext = oldLookup, oldDial })
}

func lookupReturning(addrs ...string) func(context.Context, string) ([]net.IPAddr, error) {
	return func(context.Context, string) ([]net.IPAddr, error) {
		var out []net.IPAddr
		for _, a := range addrs {
			out = append(out, net.IPAddr{IP: net.ParseIP(a)})
		}
		return out, nil
	}
}

// recordingRT records the token header of each request, then forwards it.
type recordingRT struct {
	inner http.RoundTripper
	mu    sync.Mutex
	toks  []string
}

func (r *recordingRT) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.toks = append(r.toks, req.Header.Get("X-Internal-Token"))
	r.mu.Unlock()
	return r.inner.RoundTrip(req)
}

// swapDefaultTransport puts a recording transport into httptimeout.Default for
// the test (Recheck D1) so token-less legs observe the header on the wire side
// of the client. Requests are forwarded to addr.
func swapDefaultTransport(t *testing.T, addr string) *recordingRT {
	t.Helper()
	rec := &recordingRT{inner: &http.Transport{DialContext: dialTo(addr)}}
	old := httptimeout.Default.Transport
	httptimeout.Default.Transport = rec
	t.Cleanup(func() { httptimeout.Default.Transport = old })
	return rec
}

func TestAIPluginTokenLocalOnly(t *testing.T) {
	t.Setenv("PLUGIN_INTERNAL_SECRET", fakeAISecret)
	oldOut := aiWarnOut
	aiWarnOut = &bytes.Buffer{}
	t.Cleanup(func() { aiWarnOut = oldOut })

	t.Run("loopback carries token", func(t *testing.T) {
		rec := &tokenRecorder{}
		srv := httptest.NewServer(rec)
		defer srv.Close()
		t.Setenv("PLUGIN_AI_INTERNAL_URL", srv.URL)
		if _, st, err := aiPluginRequest(context.Background(), "GET", "/health", nil); err != nil || st != 200 {
			t.Fatalf("request: status=%d err=%v", st, err)
		}
		if got := rec.seen(); len(got) != 1 || got[0] != fakeAISecret {
			t.Fatalf("server saw tokens %q, want exactly the fake secret", got)
		}
	})

	t.Run("non-local carries no token", func(t *testing.T) {
		rec := &tokenRecorder{}
		srv := httptest.NewServer(rec)
		defer srv.Close()
		wire := swapDefaultTransport(t, srv.Listener.Addr().String())
		t.Setenv("PLUGIN_AI_INTERNAL_URL", "http://example.invalid:3709")
		if _, st, err := aiPluginRequest(context.Background(), "GET", "/health", nil); err != nil || st != 200 {
			t.Fatalf("request: status=%d err=%v", st, err)
		}
		for name, got := range map[string][]string{"transport": wire.toks, "server": rec.seen()} {
			if len(got) != 1 || got[0] != "" {
				t.Fatalf("%s saw tokens %q, want one empty token", name, got)
			}
		}
	})

	t.Run("redirect never carries token to another host", func(t *testing.T) {
		second := &tokenRecorder{}
		srv2 := httptest.NewServer(second)
		defer srv2.Close()
		first := &tokenRecorder{}
		srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			first.ServeHTTP(httptest.NewRecorder(), r)
			http.Redirect(w, r, srv2.URL+"/landed", http.StatusFound)
		}))
		defer srv1.Close()
		t.Setenv("PLUGIN_AI_INTERNAL_URL", srv1.URL)
		_, st, err := aiPluginRequest(context.Background(), "GET", "/health", nil)
		if err != nil {
			t.Fatalf("request: %v", err)
		}
		if st != http.StatusFound {
			t.Errorf("status = %d, want the 302 returned unfollowed", st)
		}
		if got := first.seen(); len(got) != 1 || got[0] != fakeAISecret {
			t.Errorf("first server saw %q, want the fake secret once", got)
		}
		for _, tok := range second.seen() {
			if tok != "" {
				t.Errorf("second server received a token")
			}
		}
	})
}

func TestAIPluginTokenHostLocal(t *testing.T) {
	lookups := 0
	var addrs []net.IPAddr
	var lookupErr error
	stubAISeams(t, func(context.Context, string) ([]net.IPAddr, error) {
		lookups++
		return addrs, lookupErr
	}, nil)
	ips := func(s ...string) []net.IPAddr {
		var out []net.IPAddr
		for _, a := range s {
			out = append(out, net.IPAddr{IP: net.ParseIP(a)})
		}
		return out
	}
	cases := []struct {
		name, url string
		addrs     []net.IPAddr
		err       error
		want      bool
	}{
		{"localhost", "http://localhost:3709", nil, nil, true},
		{"127.0.0.1", "http://127.0.0.1:3709", nil, nil, true},
		{"10.0.0.5", "http://10.0.0.5:3709", nil, nil, true},
		{"172.16.0.1", "http://172.16.0.1:3709", nil, nil, true},
		{"172.32.0.1", "http://172.32.0.1:3709", nil, nil, false},
		{"192.168.1.1", "http://192.168.1.1:3709", nil, nil, true},
		{"::1", "http://[::1]:3709", nil, nil, true},
		{"fd00::5", "http://[fd00::5]:3709", nil, nil, true},
		{"fe80::1", "http://[fe80::1]:3709", nil, nil, false},
		{"169.254.169.254", "http://169.254.169.254", nil, nil, false},
		{"0.0.0.0", "http://0.0.0.0:3709", nil, nil, false},
		{"example.invalid", "http://example.invalid:3709", nil, nil, false},
		{"foo.localhost", "http://foo.localhost:3709", nil, nil, false},
		{"ftp scheme", "ftp://127.0.0.1", nil, nil, false},
		{"userinfo trick", "http://127.0.0.1@evil.example", nil, nil, false},
		{"plugin-ai private", "http://plugin-ai:3709", ips("10.0.0.5"), nil, true},
		{"plugin-ai mixed", "http://plugin-ai:3709", ips("10.0.0.5", "8.8.8.8"), nil, false},
		{"plugin-ai lookup error", "http://plugin-ai:3709", nil, errors.New("no such host"), false},
		{"plugin-ai empty", "http://plugin-ai:3709", ips(), nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			addrs, lookupErr, lookups = c.addrs, c.err, 0
			if got := aiTokenHostLocal(context.Background(), c.url); got != c.want {
				t.Errorf("aiTokenHostLocal(%q) = %v, want %v", c.url, got, c.want)
			}
			if wantLookup := strings.Contains(c.url, "plugin-ai"); (lookups > 0) != wantLookup {
				t.Errorf("lookups = %d, want a lookup only for plugin-ai", lookups)
			}
		})
	}
}

// TestAIPluginTokenNotProxied proves the token client ignores proxy variables.
// net/http reads the proxy environment once per process, so setting it with
// t.Setenv here would prove nothing: the parent re-executes the test binary
// with the proxy variables already set, and requires that the child passes,
// printed its sentinel, and that the proxy listener never accepted a connection.
func TestAIPluginTokenNotProxied(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	var accepts atomic.Int64
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepts.Add(1)
			_ = c.Close()
		}
	}()

	proxy := "http://" + ln.Addr().String()
	var env []string
	for _, kv := range os.Environ() {
		k := strings.ToUpper(strings.SplitN(kv, "=", 2)[0])
		switch k {
		case "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY":
			continue
		}
		env = append(env, kv)
	}
	env = append(env, aiChildEnv+"=1",
		"HTTP_PROXY="+proxy, "HTTPS_PROXY="+proxy, "http_proxy="+proxy, "https_proxy="+proxy)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run", "^TestAIPluginTokenNotProxiedChild$", "-test.v")
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "--- PASS: TestAIPluginTokenNotProxiedChild") {
		t.Fatalf("child did not report PASS:\n%s", out)
	}
	if !strings.Contains(string(out), aiChildDone) {
		t.Fatalf("child did not reach its sentinel %q:\n%s", aiChildDone, out)
	}
	if n := accepts.Load(); n != 0 {
		t.Fatalf("proxy listener accepted %d connection(s); the token client used the proxy", n)
	}
}

// TestAIPluginTokenNotProxiedChild is the child half; it passes at once unless
// the parent set AI_PROXY_CHILD=1 and the proxy environment.
func TestAIPluginTokenNotProxiedChild(t *testing.T) {
	if os.Getenv(aiChildEnv) != "1" {
		return
	}
	if tr, ok := aiTokenClient().Transport.(*http.Transport); !ok || tr.Proxy != nil {
		t.Fatal("aiTokenClient transport must be an *http.Transport with Proxy nil")
	}
	// Sanity: in this process a non-loopback host would have been proxied.
	probe := &http.Request{URL: &url.URL{Scheme: "http", Host: "10.9.9.9:3709", Path: "/"}}
	if p, err := http.ProxyFromEnvironment(probe); err != nil || p == nil {
		t.Fatalf("sanity: ProxyFromEnvironment = %v, %v; the proxy variables are not in effect", p, err)
	}

	rec := &tokenRecorder{}
	srv := httptest.NewServer(rec)
	defer srv.Close()
	stubAISeams(t, lookupReturning("10.9.9.9"), dialTo(srv.Listener.Addr().String()))
	t.Setenv("PLUGIN_INTERNAL_SECRET", fakeAISecret)
	t.Setenv("PLUGIN_AI_INTERNAL_URL", "http://plugin-ai:3709")
	if _, st, err := aiPluginRequest(context.Background(), "GET", "/health", nil); err != nil || st != 200 {
		t.Fatalf("request: status=%d err=%v", st, err)
	}
	if got := rec.seen(); len(got) != 1 || got[0] != fakeAISecret {
		t.Fatalf("server saw tokens %q, want the fake secret once", got)
	}
	t.Log(aiChildDone)
}

func TestAIPluginTokenWarnNonLocal(t *testing.T) {
	rec := &tokenRecorder{}
	srv := httptest.NewServer(rec)
	defer srv.Close()
	addr := srv.Listener.Addr().String()
	stubAISeams(t, lookupReturning("8.8.8.8"), dialTo(addr))
	wire := swapDefaultTransport(t, addr)
	t.Setenv("PLUGIN_AI_INTERNAL_URL", "http://plugin-ai:3709")

	var buf bytes.Buffer
	oldOut := aiWarnOut
	aiWarnOut = &buf
	t.Cleanup(func() { aiWarnOut = oldOut; aiWarnOnce = sync.Once{} })
	aiWarnOnce = sync.Once{}

	t.Setenv("PLUGIN_INTERNAL_SECRET", fakeAISecret)
	for i := 0; i < 2; i++ {
		if _, st, err := aiPluginRequest(context.Background(), "GET", "/health", nil); err != nil || st != 200 {
			t.Fatalf("request %d: status=%d err=%v", i, st, err)
		}
	}
	out := buf.String()
	if n := strings.Count(out, "\n"); n != 1 {
		t.Fatalf("warn output has %d lines, want exactly 1: %q", n, out)
	}
	if !strings.Contains(out, "PLUGIN_AI_INTERNAL_URL") {
		t.Errorf("warn line does not name PLUGIN_AI_INTERNAL_URL: %q", out)
	}
	if strings.Contains(out, fakeAISecret) {
		t.Error("warn line contains the secret value")
	}
	for name, got := range map[string][]string{"transport": wire.toks, "server": rec.seen()} {
		if len(got) != 2 || got[0] != "" || got[1] != "" {
			t.Errorf("%s saw tokens %q, want two empty tokens", name, got)
		}
	}

	// Second leg: no secret configured, so nothing is printed.
	buf.Reset()
	aiWarnOnce = sync.Once{}
	t.Setenv("PLUGIN_INTERNAL_SECRET", "")
	if _, _, err := aiPluginRequest(context.Background(), "GET", "/health", nil); err != nil {
		t.Fatalf("request without secret: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("warn output with no secret = %q, want empty", buf.String())
	}
}
