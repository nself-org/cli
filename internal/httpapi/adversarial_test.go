package httpapi

// Purpose: adversarial suite for the HTTP machine surface (P7-SURF-21 unit B).
// Inputs: hostile Host, Origin, Authorization, body, method and path shapes.
// Outputs: TestHTTPAdversarial; every subtest asserts the SAFE outcome.
// Constraints: a refused request never reaches the invoker and never reads its
//   body; no CORS header is ever sent; no wall-clock sleeps. The subtests live
//   in adversarial_*_test.go files, one theme per function.

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestHTTPAdversarial runs every adversarial theme as a named subtest.
func TestHTTPAdversarial(t *testing.T) {
	t.Run("host", advHost)
	t.Run("origin", advOrigin)
	t.Run("auth", advAuth)
	t.Run("body-order", advBodyOrder)
	t.Run("methods", advMethods)
	t.Run("oversized-body", advOversized)
	t.Run("nested-json", advNested)
	t.Run("slow-clients", advSlow)
	t.Run("framing", advFraming)
	t.Run("paths", advPaths)
	t.Run("saturation", advSaturation)
	t.Run("token-in-log", advTokenLog)
	t.Run("ndjson-abuse", advNDJSON)
}

// advHost covers Host header shapes over a real parser (raw TCP). Go itself
// answers 400 to some malformed shapes; either 400 or 421 is a refusal.
func advHost(t *testing.T) {
	r := startRaw(t, func(o *Options) { o.AllowedHosts = []string{"fake.example"} })
	p := fmt.Sprint(r.port)
	cases := []struct {
		name, host string
		ok         bool
	}{
		{"empty", "", false},
		{"port only", ":" + p, false},
		{"uppercase accepted after normalisation", "LOCALHOST:" + p, true},
		{"IPv6 loopback", "[::1]:" + p, true},
		{"trailing dot", "localhost.:" + p, false},
		{"localhost prefix of evil", "localhost.evil.com:" + p, false},
		{"127.0.0.1 prefix of evil", "127.0.0.1.evil.com:" + p, false},
		{"IPv6 no port", "[::1]", false},
		{"IPv6 long form", "[0:0:0:0:0:0:0:1]:" + p, false},
		{"IPv6 unbracketed", "::1:" + p, false},
		{"IPv6 bracket suffix", "[::1].evil.com:" + p, false},
		{"userinfo before loopback", "a@127.0.0.1:" + p, false},
		{"loopback as userinfo", "127.0.0.1:" + p + "@evil.com", false},
		{"decimal IPv4", "2130706433:" + p, false},
		{"short IPv4", "127.1:" + p, false},
		{"rebinding name", "rebind.evil.example:" + p, false},
		{"allowed name wrong port", "fake.example:1", false},
		{"two hosts in one value", "localhost:" + p + ", evil.com:" + p, false},
		{"path in host", "localhost:" + p + "/x", false},
		{"allowed name", "fake.example:" + p, true},
	}
	want := 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := fmt.Sprintf("POST /v1/commands/config/get HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nContent-Length: 2\r\n\r\n{}", tc.host, advToken)
			before := r.inv.count()
			status, body := r.send(t, raw)
			if tc.ok {
				want++
				if status != 200 {
					t.Fatalf("status %d, want 200: %s", status, body)
				}
				return
			}
			if status != 421 && status != 400 {
				t.Fatalf("status %d, want 421 or 400: %s", status, body)
			}
			if status == 421 && !strings.Contains(body, "E428") {
				t.Fatalf("421 without E428: %s", body)
			}
			if r.inv.count() != before {
				t.Fatal("refused Host reached the invoker")
			}
		})
	}
	if r.inv.count() != want {
		t.Fatalf("invoker saw %d calls, want %d", r.inv.count(), want)
	}
	t.Run("no Host header", func(t *testing.T) {
		before := r.inv.count()
		status, _ := r.send(t, "POST /v1/commands/config/get HTTP/1.1\r\nAuthorization: Bearer "+advToken+"\r\nContent-Length: 2\r\n\r\n{}")
		if status != 400 && status != 421 || r.inv.count() != before {
			t.Fatalf("status %d, invoker %d", status, r.inv.count()-before)
		}
	})
	t.Run("HTTP/1.0 without Host", func(t *testing.T) {
		before := r.inv.count()
		status, _ := r.send(t, "POST /v1/commands/config/get HTTP/1.0\r\nAuthorization: Bearer "+advToken+"\r\nContent-Type: application/json\r\nContent-Length: 2\r\n\r\n{}")
		if status != 421 && status != 400 || r.inv.count() != before {
			t.Fatalf("status %d, invoker %d", status, r.inv.count()-before)
		}
	})
	t.Run("duplicate Host headers", func(t *testing.T) {
		before := r.inv.count()
		raw := fmt.Sprintf("POST /v1/commands/config/get HTTP/1.1\r\nHost: localhost:%s\r\nHost: evil.example:%s\r\nAuthorization: Bearer %s\r\nContent-Length: 2\r\n\r\n{}", p, p, advToken)
		for _, flip := range []bool{false, true} {
			if flip {
				raw = fmt.Sprintf("POST /v1/commands/config/get HTTP/1.1\r\nHost: evil.example:%s\r\nHost: localhost:%s\r\nAuthorization: Bearer %s\r\nContent-Length: 2\r\n\r\n{}", p, p, advToken)
			}
			if status, _ := r.send(t, raw); status != 400 && status != 421 {
				t.Fatalf("flip=%v status %d", flip, status)
			}
		}
		if r.inv.count() != before {
			t.Fatal("duplicate Host reached the invoker")
		}
	})
	t.Run("absolute-form target names another host", func(t *testing.T) {
		before := r.inv.count()
		raw := fmt.Sprintf("POST http://evil.example:%s/v1/commands/config/get HTTP/1.1\r\nHost: localhost:%s\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nContent-Length: 2\r\n\r\n{}", p, p, advToken)
		if status, _ := r.send(t, raw); status != 421 && status != 400 {
			t.Fatalf("status %d", status)
		}
		if r.inv.count() != before {
			t.Fatal("absolute-form foreign host reached the invoker")
		}
	})
	t.Run("forwarding headers do not replace Host", func(t *testing.T) {
		before := r.inv.count()
		for _, h := range []string{"X-Forwarded-Host: localhost:" + p, "Forwarded: host=localhost:" + p, "X-Original-Host: localhost:" + p, "X-Host: localhost:" + p} {
			raw := fmt.Sprintf("POST /v1/commands/config/get HTTP/1.1\r\nHost: evil.example:%s\r\n%s\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nContent-Length: 2\r\n\r\n{}", p, h, advToken)
			if status, _ := r.send(t, raw); status != 421 {
				t.Fatalf("%s: status %d", h, status)
			}
		}
		if r.inv.count() != before {
			t.Fatal("forwarded Host reached the invoker")
		}
	})
	t.Run("lowercase header name cannot dodge the check", func(t *testing.T) {
		raw := fmt.Sprintf("POST /v1/commands/config/get HTTP/1.1\r\nhOsT: evil.example:%s\r\nAuthorization: Bearer %s\r\nContent-Length: 2\r\n\r\n{}", p, advToken)
		if status, _ := r.send(t, raw); status != 421 {
			t.Fatalf("status %d", status)
		}
	})
}

// advOrigin covers Origin shapes: every Origin that is not an exact allowed
// http origin is refused with E429, and no response carries CORS headers.
func advOrigin(t *testing.T) {
	s, inv, _ := newAdv(t, nil)
	refused := []string{
		"null", "file://", "file:///etc/passwd", "http://localhost:3858", "http://localhost", "http://127.0.0.1:3858",
		"http://127.0.0.1.evil.com:3857", "http://localhost.evil.com:3857", "http://localhost:3857.evil.com",
		"http://localhost:3857@evil.com", "http://user@localhost:3857", "http://evil.example", "https://localhost:3857",
		"HTTP://localhost:3857", "http://localhost:3857/", "http://localhost:3857/x", "http://[::1]:3857/", "chrome-extension://abc",
		"http://", "http://:3857", "", " http://localhost:3857", "http://localhost:3857 ", "http://localhost:3857, http://evil.example",
	}
	for _, o := range refused {
		t.Run("refused/"+o, func(t *testing.T) {
			w := call(t, s, "/v1/commands/config/get", "{}", func(r *http.Request) { r.Header.Set("Origin", o) })
			expect(t, w, 403, "E429")
			if hasCORS(w.Header()) {
				t.Fatal("CORS header on refusal")
			}
		})
	}
	for _, o := range []string{"http://localhost:3857", "http://127.0.0.1:3857", "http://[::1]:3857", "http://LOCALHOST:3857"} {
		t.Run("same-host/"+o, func(t *testing.T) {
			w := call(t, s, "/v1/commands/config/get", "{}", func(r *http.Request) { r.Header.Set("Origin", o) })
			expect(t, w, 200, "")
		})
	}
	t.Run("duplicate Origin headers", func(t *testing.T) {
		for _, order := range [][2]string{{"http://localhost:3857", "http://evil.example"}, {"http://evil.example", "http://localhost:3857"}} {
			w := call(t, s, "/v1/commands/config/get", "{}", func(r *http.Request) {
				r.Header.Add("Origin", order[0])
				r.Header.Add("Origin", order[1])
			})
			expect(t, w, 403, "E429")
		}
	})
	t.Run("preflight is never answered with CORS", func(t *testing.T) {
		for _, origin := range []string{"http://evil.example", "http://localhost:3857", "null"} {
			w := call(t, s, "/v1/commands/config/get", "", func(r *http.Request) {
				r.Method = "OPTIONS"
				r.Header.Set("Origin", origin)
				r.Header.Set("Access-Control-Request-Method", "POST")
				r.Header.Set("Access-Control-Request-Headers", "authorization,content-type")
			})
			if w.Code != 403 && w.Code != 405 {
				t.Fatalf("origin %q: status %d", origin, w.Code)
			}
		}
	})
	t.Run("foreign origin on a mutating route never invokes", func(t *testing.T) {
		before := inv.count()
		call(t, s, "/v1/commands/destroy", `{"flags":{"yes":true}}`, func(r *http.Request) { r.Header.Set("Origin", "http://evil.example") })
		if inv.count() != before {
			t.Fatal("cross-origin write reached the invoker")
		}
	})
	t.Run("lowercase header name over the wire", func(t *testing.T) {
		r := startRaw(t, nil)
		for _, h := range []string{"origin: http://evil.example", "ORIGIN: null", "Origin: http://evil.example\r\nOrigin: http://localhost:" + fmt.Sprint(r.port)} {
			if status, body := r.send(t, r.req("/v1/commands/config/get", "{}", h)); status != 403 || !strings.Contains(body, "E429") {
				t.Fatalf("%q: status %d %s", h, status, body)
			}
		}
		if r.inv.count() != 0 {
			t.Fatal("hostile origin reached the invoker")
		}
	})
}

// advAuth covers missing, malformed, mis-scheme'd and near-miss credentials.
func advAuth(t *testing.T) {
	s, inv, _ := newAdv(t, nil)
	deny := func(t *testing.T, alter func(*http.Request)) {
		t.Helper()
		before := inv.count()
		w := call(t, s, "/v1/commands/config/get", "{}", func(r *http.Request) {
			r.Header.Del("Authorization")
			alter(r)
		})
		expect(t, w, 401, "E426")
		if headerValue(w.Header(), "WWW-Authenticate") != `Bearer realm="nself"` {
			t.Fatalf("challenge %v", w.Header())
		}
		if inv.count() != before {
			t.Fatal("unauthenticated request reached the invoker")
		}
	}
	auth := func(v string) func(*http.Request) { return func(r *http.Request) { r.Header.Set("Authorization", v) } }
	for name, alter := range map[string]func(*http.Request){
		"missing":               func(*http.Request) {},
		"empty value":           auth(""),
		"scheme only":           auth("Bearer"),
		"scheme and space only": auth("Bearer "),
		"basic scheme":          auth("Basic " + advToken),
		"token scheme":          auth("Token " + advToken),
		"no scheme":             auth(advToken),
		"lowercase scheme":      auth("bearer " + advToken),
		"uppercase scheme":      auth("BEARER " + advToken),
		"double space":          auth("Bearer  " + advToken),
		"trailing space":        auth("Bearer " + advToken + " "),
		"tab separator":         auth("Bearer\t" + advToken),
		"token twice":           auth("Bearer " + advToken + " " + advToken),
		"token in query":        func(r *http.Request) { r.URL.RawQuery = "token=" + advToken + "&access_token=" + advToken },
		"token in cookie":       func(r *http.Request) { r.Header.Set("Cookie", "token="+advToken+"; Authorization=Bearer "+advToken) },
		"token in second header": func(r *http.Request) {
			for _, h := range []string{"X-Api-Key", "X-Auth-Token", "X-Nself-Token", "X-Access-Token", "Proxy-Authorization"} {
				r.Header.Set(h, advToken)
			}
		},
		"token in body": func(r *http.Request) { r.Body = io.NopCloser(strings.NewReader(`{"token":"` + advToken + `"}`)) },
	} {
		t.Run(name, func(t *testing.T) { deny(t, alter) })
	}
	t.Run("wrong token with the real one in the query", func(t *testing.T) {
		deny(t, func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer wrong")
			r.URL.RawQuery = "token=" + advToken
		})
	})
	t.Run("multiple Authorization headers", func(t *testing.T) {
		for _, order := range [][]string{{"Bearer wrong", "Bearer " + advToken}, {"Bearer " + advToken, "Bearer wrong"}, {"Bearer " + advToken, "Bearer " + advToken}} {
			deny(t, func(r *http.Request) {
				for _, v := range order {
					r.Header.Add("Authorization", v)
				}
			})
		}
	})
	t.Run("near-miss tokens of every length", func(t *testing.T) {
		for n := 0; n <= len(advToken)+3; n++ {
			cand := (advToken + "xyz")[:n]
			if cand == advToken {
				continue
			}
			deny(t, auth("Bearer "+cand))
		}
		for i := range advToken {
			b := []byte(advToken)
			b[i]++
			deny(t, auth("Bearer "+string(b)))
		}
		deny(t, auth("Bearer "+strings.Repeat(advToken, 4096)))
	})
	t.Run("the real token still works", func(t *testing.T) {
		expect(t, call(t, s, "/v1/commands/config/get", "{}", nil), 200, "")
	})
	t.Run("empty configured token fails closed", func(t *testing.T) {
		closed, cinv, _ := newAdv(t, func(o *Options) { o.Auth = BearerAuth("") })
		for _, v := range []string{"", "Bearer", "Bearer ", "Bearer " + advToken} {
			expect(t, call(t, closed, "/v1/commands/config/get", "{}", auth(v)), 401, "E426")
		}
		if cinv.count() != 0 {
			t.Fatal("empty token admitted a request")
		}
	})
	t.Run("constant-time compare is structural", func(t *testing.T) {
		assertConstantTimeCompare(t, "server.go")
	})
}

// advBodyOrder proves refusals happen before any body byte is read: a reader
// that panics would crash the handler.
func advBodyOrder(t *testing.T) {
	s, _, _ := newAdv(t, nil)
	if err := s.Mount("/mounted", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.ReadAll(r.Body) })); err != nil {
		t.Fatal(err)
	}
	panicBody := func(r *http.Request) { r.Body = io.NopCloser(panicReader{}); r.ContentLength = 1 << 20 }
	for _, tc := range []struct {
		name   string
		path   string
		alter  func(*http.Request)
		status int
	}{
		{"bad host", "/v1/commands/config/get", func(r *http.Request) { r.Host = "evil.example"; panicBody(r) }, 421},
		{"bad origin", "/v1/commands/config/get", func(r *http.Request) { r.Header.Set("Origin", "http://evil.example"); panicBody(r) }, 403},
		{"no auth", "/v1/commands/config/get", func(r *http.Request) { r.Header.Del("Authorization"); panicBody(r) }, 401},
		{"wrong auth", "/v1/commands/config/get", func(r *http.Request) { r.Header.Set("Authorization", "Bearer wrong"); panicBody(r) }, 401},
		{"no auth on openapi", "/openapi.json", func(r *http.Request) { r.Header.Del("Authorization"); r.Method = "GET"; panicBody(r) }, 401},
		{"no auth on mount", "/mounted", func(r *http.Request) { r.Header.Del("Authorization"); panicBody(r) }, 401},
		{"no auth on unknown route", "/nope", func(r *http.Request) { r.Header.Del("Authorization"); panicBody(r) }, 401},
		{"bad content type", "/v1/commands/config/get", func(r *http.Request) { r.Header.Set("Content-Type", "text/plain"); panicBody(r) }, 415},
		{"wrong method", "/v1/commands/config/get", func(r *http.Request) { r.Method = "PUT"; panicBody(r) }, 405},
		{"unknown route", "/v1/commands/nope", panicBody, 404},
	} {
		t.Run(tc.name, func(t *testing.T) { expect(t, call(t, s, tc.path, "", tc.alter), tc.status, "") })
	}
	t.Run("Expect 100-continue is not honoured before auth", func(t *testing.T) {
		r := startRaw(t, nil)
		raw := fmt.Sprintf("POST /v1/commands/config/get HTTP/1.1\r\nHost: localhost:%d\r\nAuthorization: Bearer wrong\r\nExpect: 100-continue\r\nContent-Type: application/json\r\nContent-Length: 1048576\r\n\r\n", r.port)
		status, _ := r.send(t, raw)
		if status != 401 {
			t.Fatalf("status %d, want 401 and no interim 100", status)
		}
	})
}

// advMethods covers method confusion and override headers.
func advMethods(t *testing.T) {
	s, inv, _ := newAdv(t, nil)
	for _, m := range []string{"GET", "PUT", "HEAD", "OPTIONS", "TRACE", "DELETE", "PATCH", "CONNECT", "post", "Post", "PROPFIND"} {
		t.Run(m, func(t *testing.T) {
			before := inv.count()
			w := call(t, s, "/v1/commands/config/get", "{}", func(r *http.Request) { r.Method = m })
			expect(t, w, 405, "")
			if headerValue(w.Header(), "Allow") != "POST" || inv.count() != before {
				t.Fatalf("Allow %q, invoked %d", headerValue(w.Header(), "Allow"), inv.count()-before)
			}
		})
	}
	for _, h := range []string{"X-HTTP-Method-Override", "X-Method-Override", "X-HTTP-Method", "X-Original-Method"} {
		t.Run("override "+h+" on GET", func(t *testing.T) {
			before := inv.count()
			expect(t, call(t, s, "/v1/commands/config/get", "{}", func(r *http.Request) { r.Method = "GET"; r.Header.Set(h, "POST") }), 405, "")
			if inv.count() != before {
				t.Fatal("override header turned a GET into an invocation")
			}
		})
		t.Run("override "+h+" on POST is ignored", func(t *testing.T) {
			before := inv.count()
			expect(t, call(t, s, "/v1/commands/config/get", "{}", func(r *http.Request) { r.Header.Set(h, "DELETE") }), 200, "")
			if got := inv.seen(); len(got) != before+1 || got[before] != "config get" {
				t.Fatalf("invoked %v", got)
			}
		})
	}
	t.Run("method in query is ignored", func(t *testing.T) {
		before := inv.count()
		expect(t, call(t, s, "/v1/commands/config/get?_method=DELETE", "{}", func(r *http.Request) { r.Method = "GET" }), 405, "")
		if inv.count() != before {
			t.Fatal("query method reached the invoker")
		}
	})
	t.Run("openapi accepts GET only", func(t *testing.T) {
		for _, m := range []string{"POST", "PUT", "DELETE", "OPTIONS"} {
			w := call(t, s, "/openapi.json", "{}", func(r *http.Request) { r.Method = m })
			expect(t, w, 405, "E430")
		}
	})
}
