package httpapi

// Purpose: fuzz the request path router and the Host allowlist (P7-SURF-21).
// Inputs: arbitrary path and Host strings. Outputs: FuzzRoutePath, FuzzHostHeader.
// Constraints: a recording invoker stands in for the child; seeds live in
//   testdata/fuzz/<Name>/. The server is built once per fuzz process.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
)

var (
	fuzzOnce sync.Once
	fuzzMu   sync.Mutex
	fuzzSrv  *Server
	fuzzInv  *advInvoker
)

// fuzzServer builds the shared server on first use. HOME is set only while New
// runs, because New reads it for the audit path and nothing else does.
func fuzzServer(t *testing.T) (*Server, *advInvoker) {
	fuzzMu.Lock()
	t.Cleanup(fuzzMu.Unlock)
	fuzzOnce.Do(func() {
		fuzzSrv, fuzzInv, _ = newAdv(t, func(o *Options) {
			o.AllowedHosts = []string{"fake.example"}
			o.Log = io.Discard
		})
	})
	fuzzInv.mu.Lock()
	fuzzInv.paths = fuzzInv.paths[:0]
	fuzzInv.mu.Unlock()
	return fuzzSrv, fuzzInv
}

// hasDotSegment reports a "." or ".." path segment.
func hasDotSegment(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "." {
			return true
		}
	}
	return false
}

// FuzzRoutePath fuzzes the request path through the whole handler. It must
// never panic, must invoke only the command whose route equals the decoded
// path, and must never invoke for a path with a dot segment.
func FuzzRoutePath(f *testing.F) {
	for _, seed := range []string{
		"/v1/commands/config/get", "/v1/commands/config/../config/get", "/v1/commands/%2e%2e/config/get", "//v1/commands/config/get",
		"/v1/commands/config/get/", "/v1/commands/config%2fget", "/v1/commands/config%00", "/openapi.json", "/", "/v1/commands/destroy",
		"/v1/commands/%63onfig/get", "/v1/commands/config/get;x", "/V1/commands/config/get", "/v1/commands/config/./get",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, p string) {
		s, inv := fuzzServer(t)
		u, err := url.ParseRequestURI(p)
		if err != nil || !strings.HasPrefix(p, "/") {
			return
		}
		r := &http.Request{Method: "POST", URL: u, RequestURI: p, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1, Host: "localhost:3857",
			Header: http.Header{"Authorization": {"Bearer " + advToken}, "Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader("{}")), ContentLength: 2}
		before := inv.count()
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		ran := inv.seen()[before:]
		if len(ran) == 0 {
			return
		}
		if hasDotSegment(u.Path) {
			t.Fatalf("dot-segment path %q reached %v", p, ran)
		}
		for _, cmd := range ran {
			if cmdregistry.RoutePath("nself "+cmd) != u.Path {
				t.Fatalf("path %q (decoded %q) ran command %q", p, u.Path, cmd)
			}
		}
	})
}

// FuzzHostHeader fuzzes the Host check: no panic, an accepted Host is exactly
// an allowed name and the listener port after ASCII lowercasing, and every
// allowed Host is accepted.
func FuzzHostHeader(f *testing.F) {
	for _, seed := range []string{
		"localhost:3857", "LOCALHOST:3857", "127.0.0.1:3857", "[::1]:3857", "fake.example:3857", "FAKE.example:3857", "fa\u212Ae.example:3857", "FA\u212AE.EXAMPLE:3857",
		"", ":3857", "localhost", "localhost.:3857", "localhost.evil.com:3857", "127.0.0.1.evil.com:3857", "a@127.0.0.1:3857",
		"[::1]", "[0:0:0:0:0:0:0:1]:3857", "evil.example:3857", "localhost:3857 ", " localhost:3857", "localhost:03857", "\x00",
	} {
		f.Add(seed)
	}
	allowed := map[string]bool{"localhost:3857": true, "127.0.0.1:3857": true, "[::1]:3857": true, "fake.example:3857": true}
	f.Fuzz(func(t *testing.T, host string) {
		s, inv := fuzzServer(t)
		before := inv.count()
		r := httptest.NewRequest("POST", "http://localhost:3857/v1/commands/config/get", strings.NewReader("{}"))
		r.Host = host
		r.Header.Set("Authorization", "Bearer "+advToken)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		accepted := inv.count() > before
		if accepted && w.Code != 200 {
			t.Fatalf("host %q invoked but answered %d", host, w.Code)
		}
		if want := allowed[asciiLower(host)]; accepted != want {
			t.Fatalf("host %q: accepted=%v, on allowed list after ASCII lowercase=%v (status %d)", host, accepted, want, w.Code)
		}
		if !accepted && w.Code != 421 {
			t.Fatalf("host %q refused with %d, want 421", host, w.Code)
		}
	})
}

// asciiLower lowercases A-Z only, the normalisation the allowlist promises.
func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}
