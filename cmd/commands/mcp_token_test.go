package commands

// mcp_token_test.go — the P7-SURF-25 bind and token policy of `nself mcp`.
// The older env-only middleware tests (TestMCPBearerMiddleware*) stay in
// mcp_test.go; these tests cover the policy built in mcp_auth.go.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/machineauth"
)

// policyHome isolates HOME and the token env var for a policy test.
func policyHome(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv(mcpTokenEnv, "")
	return h
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func statusOf(t *testing.T, url, auth string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// TestMCPTransportAuthPolicy builds the handler the way runMCPServe does and
// checks the loopback default in both compat modes.
func TestMCPTransportAuthPolicy(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		policyHome(t)
		v15 := os.Getenv("NSELF_V15") == "1"
		var notice bytes.Buffer
		wrap, warns, err := mcpAuthPolicy("127.0.0.1", nil, &notice)
		if err != nil {
			t.Fatalf("policy: %v", err)
		}
		srv := httptest.NewServer(wrap(okHandler()))
		defer srv.Close()

		got := statusOf(t, srv.URL, "")
		if v15 {
			if got != http.StatusUnauthorized {
				t.Fatalf("v1.5 without Authorization: got %d, want 401", got)
			}
			if len(warns) != 0 {
				t.Errorf("v1.5 loopback warnings = %q, want none", warns)
			}
			path, _ := machineauth.TokenPath()
			if !strings.Contains(notice.String(), path) {
				t.Errorf("notice %q lacks the token path", notice.String())
			}
			tok, src, _ := machineauth.Lookup()
			if src != "file" || tok == "" {
				t.Fatalf("generated token not reusable: %q", src)
			}
			if strings.Contains(notice.String(), tok) {
				t.Error("the token value appeared in the notice")
			}
			if c := statusOf(t, srv.URL, "Bearer "+tok); c != http.StatusOK {
				t.Errorf("v1.5 with the generated token: got %d, want 200", c)
			}
			return
		}
		if got != http.StatusOK {
			t.Fatalf("v1.4 with no token: got %d, want 200 (open loopback)", got)
		}
		if len(warns) != 1 {
			t.Fatalf("v1.4 open start: %d warnings %q, want exactly 1", len(warns), warns)
		}
		if _, _, err := machineauth.Lookup(); err != nil {
			t.Errorf("lookup: %v", err)
		}
		if p, _ := machineauth.TokenPath(); fileExists(p) {
			t.Error("v1.4 must not create a token file")
		}
	})
}

// TestMCPTokenSetBothModes: with NSELF_MCP_TOKEN set the behaviour is the
// same 401 / 200 in both modes with no warning.
func TestMCPTokenSetBothModes(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		policyHome(t)
		t.Setenv(mcpTokenEnv, "env-secret")
		wrap, warns, err := mcpAuthPolicy("127.0.0.1", nil, &bytes.Buffer{})
		if err != nil || len(warns) != 0 {
			t.Fatalf("policy: %v warns=%q", err, warns)
		}
		srv := httptest.NewServer(wrap(okHandler()))
		defer srv.Close()
		if c := statusOf(t, srv.URL, ""); c != 401 {
			t.Errorf("no auth: %d", c)
		}
		if c := statusOf(t, srv.URL, "Bearer env-secret"); c != 200 {
			t.Errorf("right token: %d", c)
		}
	})
}

// TestMCPHostFlag covers the --host and --allowed-host flags and the
// non-loopback refusals in both compat modes.
func TestMCPHostFlag(t *testing.T) {
	host := mcpCmd.Flags().Lookup("host")
	if host == nil || host.DefValue != "127.0.0.1" {
		t.Fatalf("--host flag missing or wrong default: %+v", host)
	}
	if mcpCmd.Flags().Lookup("allowed-host") == nil {
		t.Fatal("--allowed-host flag missing")
	}

	compattest.Both(t, func(t *testing.T) {
		home := policyHome(t)
		isE426 := func(err error) {
			t.Helper()
			if err == nil || !strings.Contains(err.Error(), "E426") || !strings.Contains(err.Error(), "--allowed-host") {
				t.Fatalf("want an E426 naming --allowed-host, got %v", err)
			}
		}

		// No allowed-host, no token: refused, no file written.
		_, _, err := mcpAuthPolicy("0.0.0.0", nil, &bytes.Buffer{})
		isE426(err)
		if fileExists(home + "/.nself/mcp-token") {
			t.Fatal("a refused start created the token file")
		}
		// allowed-host but no configured token: refused, still no file.
		_, _, err = mcpAuthPolicy("0.0.0.0", []string{"nself.example.com"}, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "E426") {
			t.Fatalf("want E426 without a token, got %v", err)
		}
		if fileExists(home + "/.nself/mcp-token") {
			t.Fatal("a refused start created the token file")
		}
		// A token without allowed-host: refused.
		t.Setenv(mcpTokenEnv, "tok")
		_, _, err = mcpAuthPolicy("0.0.0.0", nil, &bytes.Buffer{})
		isE426(err)

		// Both given: accepted with exactly one cleartext warning.
		var out bytes.Buffer
		wrap, err := prepareMCPHTTP("http", "0.0.0.0", []string{"nself.example.com"}, &out)
		if err != nil || wrap == nil {
			t.Fatalf("accepted case: %v", err)
		}
		if n := strings.Count(out.String(), "cleartext HTTP"); n != 1 {
			t.Errorf("cleartext warnings = %d in %q, want 1", n, out.String())
		}

		// stdio and unknown transports skip the policy entirely.
		out.Reset()
		if _, err := prepareMCPHTTP("stdio", "0.0.0.0", nil, &out); err != nil || out.Len() != 0 {
			t.Errorf("stdio must ignore the policy: %v %q", err, out.String())
		}
	})
}

// TestMCPHostFlagSecondStartAfterGenerated: once a loopback start wrote the
// token file (v1.5), a non-loopback start without --allowed-host is still
// refused, and with it the file counts as a configured token.
func TestMCPHostFlagSecondStartAfterGenerated(t *testing.T) {
	compattest.Set(t, true)
	policyHome(t)
	if _, _, err := mcpAuthPolicy("127.0.0.1", nil, &bytes.Buffer{}); err != nil {
		t.Fatalf("first start: %v", err)
	}
	if _, _, err := mcpAuthPolicy("0.0.0.0", nil, &bytes.Buffer{}); err == nil {
		t.Fatal("second start without --allowed-host must be refused")
	}
	_, warns, err := mcpAuthPolicy("0.0.0.0", []string{"nself.example.com"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("with --allowed-host and the file: %v", err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "cleartext HTTP") {
		t.Errorf("warnings = %q", warns)
	}
}
