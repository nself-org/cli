package commands

// mcp_auth_host_test.go — the debt D-0327 Host check of `nself mcp` sse/http.
//
// Purpose: prove that v1.5 serves only localhost, 127.0.0.1, [::1] and
//   --allowed-host names with the listener port, answers anything else 421
//   E428 before reading a body, runs the Host check before the bearer check,
//   and that v1.4 keeps serving any Host.
// Inputs:  httptest servers around mcpHandler(wrap, inner), fake tokens only.
// Outputs: test failures.

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

const hostTestToken = "fake-secret-value-0001"

// hostServer starts the handler chain built by mcpAuthPolicy and returns the
// server, its port and a counter of inner-handler runs.
func hostServer(t *testing.T, allowed []string) (*httptest.Server, string, *int) {
	t.Helper()
	policyHome(t)
	t.Setenv(mcpTokenEnv, hostTestToken)
	wrap, _, err := mcpAuthPolicy("127.0.0.1", allowed, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("policy: %v", err)
	}
	ran := 0
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ran++
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mcpHandler(wrap, inner))
	t.Cleanup(srv.Close)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	return srv, port, &ran
}

// hostGet sends one POST with the given Host and bearer token.
func hostGet(t *testing.T, srv *httptest.Server, host, auth string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(`{"fake":"body"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = host
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Error("response carries a CORS header")
	}
	return resp.StatusCode, string(b)
}

func TestMCPHostCheckV15(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		v15 := os.Getenv("NSELF_V15") == "1"
		srv, port, ran := hostServer(t, []string{"my.box"})
		bearer := "Bearer " + hostTestToken
		other := strconv.Itoa(mustAtoi(t, port) + 1)

		for _, h := range []string{"127.0.0.1:" + port, "localhost:" + port, "[::1]:" + port, "LOCALHOST:" + port, "my.box:" + port, "My.Box:" + port} {
			*ran = 0
			if c, _ := hostGet(t, srv, h, bearer); c != http.StatusOK || *ran != 1 {
				t.Errorf("Host %q: status %d ran %d, want 200 and 1", h, c, *ran)
			}
		}
		for _, h := range []string{"evil.example:" + port, "127.0.0.1:" + other, "localhost", "my.box", "evil.example"} {
			*ran = 0
			c, body := hostGet(t, srv, h, bearer)
			if !v15 {
				if c != http.StatusOK || *ran != 1 {
					t.Errorf("v1.4 Host %q: status %d ran %d, want unchanged 200 and 1", h, c, *ran)
				}
				continue
			}
			if c != http.StatusMisdirectedRequest {
				t.Errorf("Host %q: status %d, want 421", h, c)
			}
			if !strings.Contains(body, "E428") {
				t.Errorf("Host %q: body %q lacks E428", h, body)
			}
			if strings.Contains(body, h) {
				t.Errorf("Host %q: body echoes the Host: %q", h, body)
			}
			if *ran != 0 {
				t.Errorf("Host %q: inner handler ran after a refusal", h)
			}
		}
	})
}

// TestMCPHostCheckOrder: Host first, then bearer.
func TestMCPHostCheckOrder(t *testing.T) {
	only15(t, func(t *testing.T) {
		srv, port, ran := hostServer(t, nil)
		if c, _ := hostGet(t, srv, "evil.example:"+port, "Bearer "+hostTestToken); c != http.StatusMisdirectedRequest {
			t.Errorf("wrong Host with a valid token: %d, want 421", c)
		}
		if c, _ := hostGet(t, srv, "127.0.0.1:"+port, ""); c != http.StatusUnauthorized {
			t.Errorf("right Host without a token: %d, want 401", c)
		}
		if c, _ := hostGet(t, srv, "evil.example:"+port, ""); c != http.StatusMisdirectedRequest {
			t.Errorf("wrong Host without a token: %d, want 421", c)
		}
		if *ran != 0 {
			t.Error("inner handler ran")
		}
	})
}

// TestMCPHostCheckFailClosed: a request with no local address is refused.
func TestMCPHostCheckFailClosed(t *testing.T) {
	only15(t, func(t *testing.T) {
		ran := false
		h := mcpHostCheck(nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { ran = true }))
		req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:1/", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusMisdirectedRequest || ran {
			t.Errorf("no local address: status %d ran %v, want 421 and false", rec.Code, ran)
		}
	})
}

// TestMCPAllowedHostValidation: v1.5 validates --allowed-host before any
// listener opens.
func TestMCPAllowedHostValidation(t *testing.T) {
	only15(t, func(t *testing.T) {
		policyHome(t)
		for _, bad := range []string{"foo:8080", "http://foo", "::1"} {
			_, _, err := mcpAuthPolicy("127.0.0.1", []string{bad}, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), "E430") {
				t.Errorf("--allowed-host %q: err = %v, want E430", bad, err)
			}
		}
		if _, err := prepareMCPHTTP("http", "127.0.0.1", []string{"foo:8080"}, &bytes.Buffer{}); err == nil {
			t.Error("prepareMCPHTTP accepted foo:8080")
		}
	})
}

// only15 runs fn once with v1.5 behaviour on.
func only15(t *testing.T, fn func(t *testing.T)) {
	t.Helper()
	compattest.Set(t, true)
	fn(t)
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
