package httpapi

// Purpose: stateful adversarial cases: in-flight saturation, token leakage into
//   logs and error bodies, NDJSON route abuse, and the structural
//   constant-time check (P7-SURF-21 unit B).
// Inputs: a recording invoker, a log buffer, a stream-capable fixture registry.
// Outputs: subtests of TestHTTPAdversarial.
// Constraints: goroutine accounting uses Gosched polling, never sleeps.

import (
	"bytes"
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/spf13/cobra"
)

// post sends one authenticated POST without touching *testing.T, so it is safe
// to run from helper goroutines.
func post(s *Server, path, body string, alter func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "http://localhost:3857"+path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+advToken)
	r.Header.Set("Content-Type", "application/json")
	if alter != nil {
		alter(r)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

// settleGoroutines waits (yielding, not sleeping) until the goroutine count is
// back at base or two seconds pass.
func settleGoroutines(t *testing.T, base int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > base {
		if time.Now().After(deadline) {
			t.Fatalf("goroutine leak: %d now, %d before", runtime.NumGoroutine(), base)
		}
		runtime.Gosched()
	}
}

// advSaturation fills every in-flight slot and checks E431 handling, that
// validation failures do not need a slot, and that slots and goroutines return.
func advSaturation(t *testing.T) {
	const slots = 2
	t.Run("full slots answer 503 and release", func(t *testing.T) {
		s, inv, _ := newAdv(t, func(o *Options) { o.MaxInFlight = slots })
		inv.gate, inv.entered = make(chan struct{}), make(chan struct{}, slots)
		base := runtime.NumGoroutine()
		var wg sync.WaitGroup
		for i := 0; i < slots; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); post(s, "/v1/commands/config/get", "{}", nil) }()
		}
		for i := 0; i < slots; i++ {
			<-inv.entered
		}
		w := call(t, s, "/v1/commands/config/get", "{}", nil)
		expect(t, w, 503, "E431")
		if headerValue(w.Header(), "Retry-After") != "1" {
			t.Fatal(w.Header())
		}
		for name, tc := range map[string]struct {
			path, body string
			alter      func(*http.Request)
			status     int
		}{
			"bad json":     {"/v1/commands/config/get", "not json", nil, 400},
			"bad field":    {"/v1/commands/config/get", `{"wat":1}`, nil, 400},
			"unknown":      {"/v1/commands/nope", "{}", nil, 404},
			"wrong type":   {"/v1/commands/config/get", "{}", func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, 415},
			"unauthorised": {"/v1/commands/config/get", "{}", func(r *http.Request) { r.Header.Del("Authorization") }, 401},
		} {
			if got := post(s, tc.path, tc.body, tc.alter).Code; got != tc.status {
				t.Fatalf("%s while saturated: status %d, want %d", name, got, tc.status)
			}
		}
		if inv.count() != slots {
			t.Fatalf("invoker calls %d while saturated, want %d", inv.count(), slots)
		}
		close(inv.gate)
		wg.Wait()
		expect(t, call(t, s, "/v1/commands/config/get", "{}", nil), 200, "")
		settleGoroutines(t, base)
	})
	t.Run("a canceled request frees its slot", func(t *testing.T) {
		s, inv, _ := newAdv(t, func(o *Options) { o.MaxInFlight = 1 })
		inv.gate, inv.entered = make(chan struct{}), make(chan struct{}, 1)
		base := runtime.NumGoroutine()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			post(s, "/v1/commands/config/get", "{}", func(r *http.Request) { *r = *r.WithContext(ctx) })
			close(done)
		}()
		<-inv.entered
		expect(t, call(t, s, "/v1/commands/config/get", "{}", nil), 503, "E431")
		cancel()
		<-done
		inv.mu.Lock()
		inv.gate = nil
		inv.mu.Unlock()
		expect(t, call(t, s, "/v1/commands/config/get", "{}", nil), 200, "")
		settleGoroutines(t, base)
	})
	t.Run("hammering yields only 200 or 503", func(t *testing.T) {
		s, _, _ := newAdv(t, func(o *Options) { o.MaxInFlight = slots })
		base := runtime.NumGoroutine()
		var wg sync.WaitGroup
		codes := make(chan int, 128)
		for i := 0; i < 128; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); codes <- post(s, "/v1/commands/config/get", "{}", nil).Code }()
		}
		wg.Wait()
		close(codes)
		for c := range codes {
			if c != 200 && c != 503 {
				t.Fatalf("status %d", c)
			}
		}
		settleGoroutines(t, base)
	})
}

// advTokenLog sends many hostile requests and proves neither the access log nor
// any response (headers or body) contains the real or any presented secret.
func advTokenLog(t *testing.T) {
	secrets := []string{advToken, "fake-token-0001", "nself_test_FAKE_query_0003", "nself_test_FAKE_cookie_0004", "nself_test_FAKE_body_0005", "nself_test_FAKE_origin_0006", "nself_test_FAKE_host_0007", "nself_test_FAKE_invoker_0008"}
	s, inv, lg := newAdv(t, nil)
	var seen strings.Builder
	record := func(w *httptest.ResponseRecorder) {
		seen.WriteString(w.Body.String())
		for k, v := range w.Header() {
			seen.WriteString(k + ": " + strings.Join(v, ",") + "\n")
		}
	}
	do := func(path, body string, alter func(*http.Request)) {
		record(call(t, s, path, body, alter))
	}
	do("/v1/commands/config/get", "{}", func(r *http.Request) { r.Header.Set("Authorization", "Bearer fake-token-0001") })
	do("/v1/commands/config/get?token=nself_test_FAKE_query_0003", "{}", func(r *http.Request) {
		r.Header.Del("Authorization")
		r.Header.Set("Cookie", "token=nself_test_FAKE_cookie_0004")
	})
	do("/v1/commands/config/get", `{"args":["nself_test_FAKE_body_0005"]}`, nil)
	do("/v1/commands/config/get", "{}", func(r *http.Request) { r.Header.Set("Origin", "http://nself_test_FAKE_origin_0006.example") })
	do("/v1/commands/config/get", "{}", func(r *http.Request) { r.Host = "nself_test_FAKE_host_0007.example" })
	do("/v1/commands/nope", "{}", nil)
	do("/v1/commands/config/get", "not json nself_test_FAKE_body_0005", nil)
	inv.mu.Lock()
	inv.err = errors.New("failed using " + advToken + " and nself_test_FAKE_invoker_0008")
	inv.mu.Unlock()
	do("/v1/commands/config/get", "{}", nil)
	for _, secret := range secrets {
		if strings.Contains(seen.String(), secret) {
			t.Errorf("response leaks %q", secret)
		}
		if strings.Contains(lg.String(), secret) {
			t.Errorf("log leaks %q", secret)
		}
	}
	line := regexp.MustCompile(`^("[A-Z]+" "[^"]*" [0-9]{3} [0-9]+ms|invoke error: \*[a-zA-Z.]+)$`)
	lines := strings.Split(strings.TrimSpace(lg.String()), "\n")
	if len(lines) < 8 {
		t.Fatalf("log has %d lines", len(lines))
	}
	for _, l := range lines {
		if !line.MatchString(l) {
			t.Errorf("unexpected log line %q", l)
		}
	}
	t.Run("bearer auth error is generic", func(t *testing.T) {
		_, err := BearerAuth(advToken).Authenticate(httptest.NewRequest("POST", "/", nil))
		if err == nil || strings.Contains(err.Error(), advToken) {
			t.Fatalf("err %v", err)
		}
		r := httptest.NewRequest("POST", "/", nil)
		r.Header.Set("Authorization", "Bearer nself_test_FAKE_query_0003")
		if _, err = BearerAuth(advToken).Authenticate(r); err == nil || strings.Contains(err.Error(), "nself_test_FAKE_query_0003") || strings.Contains(err.Error(), advToken) {
			t.Fatalf("err %v", err)
		}
	})
}

// streamRegistry has a write stream command, a write command with a stream
// flag override, a read stream command whose flag can raise it to write, and a
// plain document command.
func streamRegistry(t *testing.T) *cmdregistry.Registry {
	t.Helper()
	root := &cobra.Command{Use: "nself"}
	for _, name := range []string{"wstream", "wlogs", "rlogs", "rdoc"} {
		c := &cobra.Command{Use: name, Run: func(*cobra.Command, []string) {}}
		if name == "wlogs" || name == "rlogs" {
			c.Flags().Bool("follow", false, "")
		}
		if name == "rlogs" {
			c.Flags().Bool("wipe", false, "")
		}
		root.AddCommand(c)
	}
	f, err := canon.Parse([]byte(`schema_version: 1
verbs: [wstream, wlogs, rlogs, rdoc]
commands:
  wstream: {canon: core, side_effect: write, output: stream}
  wlogs: {canon: core, side_effect: write, flags: {follow: {output: stream}}}
  rlogs: {canon: core, side_effect: read, flags: {follow: {output: stream}, wipe: {side_effect: write}}}
  rdoc: {canon: core, side_effect: read}
`))
	if err != nil {
		t.Fatal(err)
	}
	empty := map[string]any{}
	r, err := cmdregistry.Build(root, f, map[string]any{"wstream": empty, "wlogs": empty, "rlogs": empty, "rdoc": empty}, cmdregistry.BuildOptions{V15: true})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// advNDJSON abuses the stream route: write commands never stream, the stream
// flag is not advertised, and ndjson Accept cannot turn a document into a stream.
func advNDJSON(t *testing.T) {
	pids := t.TempDir()
	// build sets the stub env after newAdv, because server(t) resets it. The
	// default mode "block" makes any spawned child leave a marker in pids.
	mode := "block"
	build := func(t *testing.T) *Server {
		s, _, _ := newAdv(t, func(o *Options) { o.Registry, o.Invoker = streamRegistry(t), invoke.Invoke })
		t.Setenv("HTTPAPI_PID", pids)
		t.Setenv("HTTPAPI_MODE", mode)
		return s
	}
	nd := func(r *http.Request) { r.Header.Set("Accept", "application/x-ndjson") }
	spawned := func() int {
		entries, _ := os.ReadDir(pids)
		return len(entries)
	}
	for _, tc := range []struct {
		name, path, body string
		alter            func(*http.Request)
	}{
		{"write stream command over ndjson", "/v1/commands/wstream", "{}", nd},
		{"write stream command over json", "/v1/commands/wstream", "{}", nil},
		{"write command with stream flag over ndjson", "/v1/commands/wlogs", `{"flags":{"follow":true}}`, nd},
		{"write command with stream flag over json", "/v1/commands/wlogs", `{"flags":{"follow":true}}`, nil},
		{"read stream raised to write by a flag", "/v1/commands/rlogs", `{"flags":{"follow":true,"wipe":true}}`, nd},
		{"stream flag without ndjson accept", "/v1/commands/rlogs", `{"flags":{"follow":true}}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := call(t, build(t), tc.path, tc.body, tc.alter)
			expect(t, w, 404, "E421")
			if headerValue(w.Header(), "Content-Type") == "application/x-ndjson" || spawned() != 0 {
				t.Fatalf("stream started or a child spawned: %v", w.Header())
			}
		})
	}
	t.Run("ndjson-only accept on a document command", func(t *testing.T) {
		expect(t, call(t, build(t), "/v1/commands/rdoc", "{}", nd), 406, "E430")
		if spawned() != 0 {
			t.Fatal("child spawned")
		}
	})
	for _, q := range []string{"0", "abc", "-1", "2", "NaN"} {
		t.Run("q="+q+" never selects a stream", func(t *testing.T) {
			w := call(t, build(t), "/v1/commands/rlogs", `{"flags":{"follow":true}}`, func(r *http.Request) { r.Header.Set("Accept", "application/x-ndjson;q="+q) })
			expect(t, w, 406, "E430")
			if spawned() != 0 {
				t.Fatal("child spawned")
			}
		})
	}
	t.Run("stream flag spelled as an argument does not stream", func(t *testing.T) {
		mode = ""
		defer func() { mode = "block" }()
		w := call(t, build(t), "/v1/commands/rlogs", `{"args":["--follow"]}`, nd)
		if headerValue(w.Header(), "Content-Type") == "application/x-ndjson" {
			t.Fatalf("argument selected a stream: %d", w.Code)
		}
	})
	t.Run("stream flag is not advertised", func(t *testing.T) {
		b, err := OpenAPI(streamRegistry(t), nil)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(b, []byte(`"follow"`)) {
			t.Fatal("OpenAPI lists the stream override flag")
		}
	})
	t.Run("a read stream still streams", func(t *testing.T) {
		mode = "stream"
		defer func() { mode = "block" }()
		w := call(t, build(t), "/v1/commands/rlogs", `{"flags":{"follow":true}}`, nd)
		expect(t, w, 200, "")
		if headerValue(w.Header(), "Content-Type") != "application/x-ndjson" {
			t.Fatal(w.Header())
		}
	})
}

// assertConstantTimeCompare parses a package source file and fails unless it
// hashes both sides, compares with crypto/subtle, never imports math/rand and
// never compares a secret-named identifier with == or !=. This is the
// structural stand-in for a timing measurement.
func assertConstantTimeCompare(t *testing.T, file string) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Clean(file), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	imports := map[string]bool{}
	for _, imp := range f.Imports {
		imports[strings.Trim(imp.Path.Value, `"`)] = true
	}
	for _, want := range []string{"crypto/subtle", "crypto/sha256"} {
		if !imports[want] {
			t.Errorf("%s does not import %s", file, want)
		}
	}
	if imports["math/rand"] {
		t.Errorf("%s imports math/rand", file)
	}
	called := false
	secretish := map[string]bool{"tok": true, "got": true, "want": true, "value": true, "token": true, "sum": true}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "ConstantTimeCompare" {
				called = true
			}
		case *ast.BinaryExpr:
			if x.Op != token.EQL && x.Op != token.NEQ {
				return true
			}
			for _, side := range []ast.Expr{x.X, x.Y} {
				id, ok := side.(*ast.Ident)
				other := x.Y
				if side == x.Y {
					other = x.X
				}
				lit, isLit := other.(*ast.BasicLit)
				if ok && secretish[id.Name] && !(isLit && lit.Value == `""`) {
					t.Errorf("%s: %s compares %q with %s", file, fset.Position(x.Pos()), id.Name, x.Op)
				}
			}
		}
		return true
	})
	if !called {
		t.Errorf("%s never calls subtle.ConstantTimeCompare", file)
	}
}
