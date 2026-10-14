package httpapi

// Purpose: wire-level adversarial cases: body size and shape, slow clients,
//   framing lies and path tricks (P7-SURF-21 unit B).
// Inputs: raw TCP bytes against a real listener, or hostile request bodies.
// Outputs: subtests of TestHTTPAdversarial.
// Constraints: short fake timeouts only; connection deadlines are failure guards.

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
)

// endlessReader yields 'a' forever and counts the bytes handed out.
type endlessReader struct{ n atomic.Int64 }

func (e *endlessReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	e.n.Add(int64(len(p)))
	return len(p), nil
}

// advOversized proves an over-limit body is refused after reading at most
// MaxRequestBytes+1 bytes, however much the client would keep sending.
func advOversized(t *testing.T) {
	s, inv, _ := newAdv(t, nil)
	for name, declared := range map[string]int64{"unknown length": -1, "lying short length": 10, "huge declared length": 1 << 40} {
		t.Run(name, func(t *testing.T) {
			src := &endlessReader{}
			w := call(t, s, "/v1/commands/config/get", "", func(r *http.Request) { r.Body = io.NopCloser(src); r.ContentLength = declared })
			expect(t, w, 413, "E430")
			if got := src.n.Load(); got > invoke.MaxRequestBytes+4096 {
				t.Fatalf("server pulled %d bytes, limit %d", got, invoke.MaxRequestBytes)
			}
		})
	}
	t.Run("exactly at the limit is a JSON error not a size error", func(t *testing.T) {
		pad := strings.Repeat(" ", invoke.MaxRequestBytes-2)
		w := call(t, s, "/v1/commands/config/get", "{}"+pad, nil)
		if w.Code == 413 {
			t.Fatalf("a body of exactly the limit was refused as too large")
		}
	})
	t.Run("one byte over the limit", func(t *testing.T) {
		before := inv.count()
		expect(t, call(t, s, "/v1/commands/config/get", "{}"+strings.Repeat(" ", invoke.MaxRequestBytes-1), nil), 413, "E430")
		if inv.count() != before {
			t.Fatal("oversized body reached the invoker")
		}
	})
}

// advNested sends deeply nested and pathological JSON through the real invoker
// with a child stub that would announce itself; none may spawn a child.
func advNested(t *testing.T) {
	pids := t.TempDir()
	s, _, _ := newAdv(t, func(o *Options) { o.Invoker = invoke.Invoke })
	t.Setenv("HTTPAPI_PID", pids) // after newAdv: server(t) resets the stub env
	t.Setenv("HTTPAPI_MODE", "block")
	nest := func(n int, open, close string) string { return strings.Repeat(open, n) + strings.Repeat(close, n) }
	bodies := map[string]string{
		"array depth 100000":         `{"args":` + nest(100000, "[", "]") + `}`,
		"array depth 9000":           `{"args":` + nest(9000, "[", "]") + `}`,
		"object depth 5000 in flags": `{"flags":` + strings.Repeat(`{"a":`, 5000) + `1` + strings.Repeat("}", 5000) + `}`,
		"object depth 100000":        strings.Repeat(`{"a":`, 100000) + `1` + strings.Repeat("}", 100000),
		"unterminated nesting":       `{"args":` + strings.Repeat("[", 500000),
		"top-level array":            `[` + nest(5000, "[", "]") + `]`,
		"duplicate keys":             `{"args":["a"],"args":["b"]}`,
		"nul in argument":            `{"args":["a\u0000b"]}`,
		"huge number":                `{"args":[1e999999999]}`,
		"BOM prefix":                 "\xef\xbb\xbf{}",
		"trailing garbage":           `{} {}`,
		"null body":                  `null`,
		"scalar body":                `"x"`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			w := call(t, s, "/v1/commands/config/get", body, nil)
			if w.Code < 400 || w.Code >= 500 {
				t.Fatalf("status %d: %.200s", w.Code, w.Body)
			}
			if entries, _ := os.ReadDir(pids); len(entries) != 0 {
				t.Fatal("hostile body spawned a child")
			}
		})
	}
}

// advSlow covers stalled headers and bodies against a server with short fake
// timeouts, and a slow body that never has to be read when auth fails.
func advSlow(t *testing.T) {
	short := func(o *Options) { o.ReadTimeout, o.ReadHeaderTimeout = 80*time.Millisecond, 80*time.Millisecond }
	t.Run("stalled headers are dropped", func(t *testing.T) {
		r := startRaw(t, short)
		c := r.dial(t)
		_, _ = fmt.Fprintf(c, "POST /v1/commands/config/get HTTP/1.1\r\nHost: localhost:%d\r\nX-Slow: a", r.port)
		buf := make([]byte, 1)
		if _, err := c.Read(buf); err == nil && buf[0] == 0 {
			t.Fatal("unexpected byte")
		}
		if r.inv.count() != 0 {
			t.Fatal("stalled headers reached the invoker")
		}
	})
	t.Run("stalled body is released with 400", func(t *testing.T) {
		r := startRaw(t, short)
		raw := r.req("/v1/commands/config/get", "{", "Content-Length: 100")
		status, _ := r.send(t, raw)
		if status != 400 || r.inv.count() != 0 {
			t.Fatalf("status %d, invoked %d", status, r.inv.count())
		}
		if status, _ := r.send(t, r.req("/v1/commands/config/get", "{}")); status != 200 {
			t.Fatalf("server wedged after a stalled body: %d", status)
		}
	})
	t.Run("slow body with a bad token is refused without waiting for it", func(t *testing.T) {
		r := startRaw(t, nil)
		raw := fmt.Sprintf("POST /v1/commands/config/get HTTP/1.1\r\nHost: localhost:%d\r\nAuthorization: Bearer wrong\r\nContent-Type: application/json\r\nContent-Length: 1000000\r\n\r\n{", r.port)
		if status, _ := r.send(t, raw); status != 401 {
			t.Fatalf("status %d", status)
		}
	})
	t.Run("many idle connections do not block a good request", func(t *testing.T) {
		r := startRaw(t, nil)
		for i := 0; i < 20; i++ {
			c := r.dial(t)
			_, _ = fmt.Fprintf(c, "POST /v1/commands/config/get HTTP/1.1\r\nHost: localhost:%d\r\n", r.port)
		}
		if status, _ := r.send(t, r.req("/v1/commands/config/get", "{}")); status != 200 {
			t.Fatalf("status %d", status)
		}
	})
}

// advFraming covers lying lengths, chunked bodies and request smuggling shapes.
func advFraming(t *testing.T) {
	good := func(r *rawSrv) string { return r.req("/v1/commands/config/get", "{}") }
	noAuth := func(r *rawSrv, path string) string {
		return fmt.Sprintf("POST %s HTTP/1.1\r\nHost: localhost:%d\r\nContent-Type: application/json\r\nContent-Length: 2\r\n\r\n{}", path, r.port)
	}
	t.Run("content length shorter than the body", func(t *testing.T) {
		r := startRaw(t, nil)
		c := r.dial(t)
		_, _ = io.WriteString(c, r.req("/v1/commands/config/get", `{"args":["x"]}`, "Content-Length: 5"))
		rs := readResponses(t, c, 1)
		if len(rs) == 0 || rs[0].StatusCode != 400 || r.inv.count() != 0 {
			t.Fatalf("responses %d, invoked %d", len(rs), r.inv.count())
		}
	})
	t.Run("content length longer than the body then close", func(t *testing.T) {
		r := startRaw(t, nil)
		c := r.dial(t)
		_, _ = io.WriteString(c, r.req("/v1/commands/config/get", "{}", "Content-Length: 50"))
		c.(interface{ CloseWrite() error }).CloseWrite()
		rs := readResponses(t, c, 1)
		if r.inv.count() != 0 || len(rs) > 0 && rs[0].StatusCode != 400 {
			t.Fatalf("invoked %d", r.inv.count())
		}
	})
	t.Run("smuggled second request still needs its own auth", func(t *testing.T) {
		r := startRaw(t, nil)
		c := r.dial(t)
		_, _ = io.WriteString(c, good(r)+noAuth(r, "/v1/commands/destroy"))
		rs := readResponses(t, c, 2)
		if len(rs) != 2 || rs[0].StatusCode != 200 || rs[1].StatusCode != 401 {
			t.Fatalf("responses %d", len(rs))
		}
		if got := r.inv.seen(); len(got) != 1 || got[0] != "config get" {
			t.Fatalf("invoked %v", got)
		}
	})
	t.Run("transfer-encoding with content-length", func(t *testing.T) {
		r := startRaw(t, nil)
		c := r.dial(t)
		chunked := r.req("/v1/commands/config/get", "2\r\n{}\r\n0\r\n\r\n", "Transfer-Encoding: chunked", "Content-Length: 3")
		_, _ = io.WriteString(c, chunked+noAuth(r, "/v1/commands/destroy"))
		rs := readResponses(t, c, 2)
		for _, resp := range rs[min(1, len(rs)):] {
			if resp.StatusCode != 401 && resp.StatusCode != 400 {
				t.Fatalf("smuggled request answered %d", resp.StatusCode)
			}
		}
		for _, p := range r.inv.seen() {
			if p != "config get" {
				t.Fatalf("smuggled command ran: %s", p)
			}
		}
	})
	for name, header := range map[string]string{
		"unknown transfer coding":   "Transfer-Encoding: gzip, chunked",
		"identity transfer coding":  "Transfer-Encoding: identity",
		"obfuscated chunked":        "Transfer-Encoding: xchunked",
		"duplicate content length":  "Content-Length: 2\r\nContent-Length: 3",
		"negative content length":   "Content-Length: -1",
		"signed content length":     "Content-Length: +2",
		"content length with comma": "Content-Length: 2, 2",
	} {
		t.Run(name, func(t *testing.T) {
			r := startRaw(t, nil)
			raw := r.req("/v1/commands/config/get", "{}", header)
			status, _ := r.send(t, raw)
			if status != 400 && status != 501 && status != 0 {
				t.Fatalf("status %d", status)
			}
			if r.inv.count() != 0 {
				t.Fatal("ambiguous framing reached the invoker")
			}
		})
	}
	t.Run("giant chunk size with tiny body", func(t *testing.T) {
		r := startRaw(t, nil)
		c := r.dial(t)
		_, _ = io.WriteString(c, r.req("/v1/commands/config/get", "7fffffff\r\n{}", "Transfer-Encoding: chunked"))
		c.(interface{ CloseWrite() error }).CloseWrite()
		readResponses(t, c, 1)
		if r.inv.count() != 0 {
			t.Fatal("truncated chunked body reached the invoker")
		}
	})
	t.Run("chunked body past the limit", func(t *testing.T) {
		r := startRaw(t, nil)
		c := r.dial(t)
		chunk := fmt.Sprintf("%x\r\n%s\r\n", 1<<16, strings.Repeat(" ", 1<<16))
		go func() {
			_, _ = io.WriteString(c, r.req("/v1/commands/config/get", "", "Transfer-Encoding: chunked"))
			for i := 0; i < 2*(invoke.MaxRequestBytes>>16)+2; i++ {
				if _, err := io.WriteString(c, chunk); err != nil {
					return
				}
			}
		}()
		rs := readResponses(t, c, 1)
		if len(rs) == 0 || rs[0].StatusCode != 413 || r.inv.count() != 0 {
			t.Fatalf("responses %d, invoked %d", len(rs), r.inv.count())
		}
	})
}

// advPaths covers path tricks over a real parser. Whatever the status, a
// command runs only when its own route equals the decoded request path.
func advPaths(t *testing.T) {
	r := startRaw(t, nil)
	const base = "/v1/commands/config/get"
	long := "/v1/commands/" + strings.Repeat("a", 100000)
	cases := []struct {
		name, target string
		ran          string // "" means nothing may run
	}{
		{"canonical", base, "config get"},
		{"percent-encoded letter", "/v1/commands/%63onfig/get", "config get"},
		{"query is not path", base + "?x=../..", "config get"},
		{"dot dot", "/v1/commands/../commands/config/get", ""},
		{"encoded dot dot", "/v1/commands/%2e%2e/commands/config/get", ""},
		{"encoded dot dot upper", "/v1/commands/%2E%2E/commands/config/get", ""},
		{"inner dot dot", "/v1/commands/config/../config/get", ""},
		{"single dot", "/v1/commands/./config/get", ""},
		{"leading double slash", "//v1/commands/config/get", ""},
		{"inner double slash", "/v1/commands/config//get", ""},
		{"trailing slash", base + "/", ""},
		{"encoded NUL tail", base + "%00", ""},
		{"encoded NUL inside", "/v1/commands/config%00/get", ""},
		{"uppercase prefix", "/V1/commands/config/get", ""},
		{"uppercase word", "/v1/commands/Config/get", ""},
		{"uppercase leaf", "/v1/commands/config/GET", ""},
		{"trailing space", base + "%20", ""},
		{"semicolon parameter", base + ";x=1", ""},
		{"encoded backslash", "/v1/commands/config%5Cget", ""},
		{"encoded slash dot dot", "/v1/commands/config%2f..%2fdestroy", ""},
		{"overlong slash", "/v1/commands/%c0%afconfig/get", ""},
		{"fullwidth slash", "/v1/commands/%ef%bc%8fconfig/get", ""},
		{"division slash", "/v1/commands/config%e2%88%95get", ""},
		{"cyrillic c", "/v1/commands/%d1%81onfig/get", ""},
		{"latin script g", "/v1/commands/config/%c9%a1et", ""},
		{"zero width space", base + "%e2%80%8b", ""},
		{"raw utf8", "/v1/commands/\u0441onfig/get", ""},
		{"very long path", long, ""},
		{"destroy via dot dot", "/v1/commands/config/../destroy", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := r.inv.count()
			status, _ := r.send(t, r.req(tc.target, "{}"))
			ran := r.inv.seen()[before:]
			if tc.ran == "" {
				if len(ran) != 0 {
					t.Fatalf("status %d: ran %v", status, ran)
				}
				if status != 301 && status != 307 && status != 308 && status != 400 && status != 404 && status != 414 && status != 431 && status != 0 {
					t.Fatalf("status %d", status)
				}
				return
			}
			if status != 200 || len(ran) != 1 || ran[0] != tc.ran {
				t.Fatalf("status %d ran %v", status, ran)
			}
		})
	}
	t.Run("encoded slash maps only to its decoded route", func(t *testing.T) {
		before := r.inv.count()
		r.send(t, r.req("/v1/commands/config%2Fget", "{}"))
		for _, p := range r.inv.seen()[before:] {
			if cmdregistry.RoutePath("nself "+p) != "/v1/commands/config/get" {
				t.Fatalf("ran %q", p)
			}
		}
	})
	t.Run("long path is truncated in the log", func(t *testing.T) {
		for _, line := range strings.Split(strings.TrimSpace(r.log.String()), "\n") {
			if len(line) > 400 {
				t.Fatalf("log line of %d bytes", len(line))
			}
		}
	})
	t.Run("log lines cannot be forged by path newlines", func(t *testing.T) {
		before := strings.Count(r.log.String(), "\n")
		r.send(t, r.req("/v1/commands/x%0a%22POST%22%20%22/v1/commands/destroy%22%20200%200ms", "{}"))
		if got := strings.Count(r.log.String(), "\n") - before; got != 1 {
			t.Fatalf("one request wrote %d log lines", got)
		}
	})
	t.Run("unknown route does not echo the path", func(t *testing.T) {
		_, body := r.send(t, r.req("/v1/commands/<script>alert(1)</script>", "{}"))
		if strings.Contains(body, "<script>") {
			t.Fatalf("path echoed: %s", body)
		}
	})
}
