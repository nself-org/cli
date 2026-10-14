package commands

// Purpose: D-0739 proves token-less v1.4 SSE/HTTP rejects rebinding Hosts
// before reading a body or reaching legacy tools, without widening the
// loopback allowlist via --allowed-host.
// Inputs: isolated token lookup and requests carrying the listener address.
// Outputs: failures for an unsafe transport policy.
// Constraints: no network or real tools; configured-token compatibility is
// covered by TestMCPHostCheckV15 and TestMCPTokenSetBothModes.
// SPORT: CLI-CMD-MCP-001

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

type tokenlessProbeBody struct{ reads int }

func (b *tokenlessProbeBody) Read([]byte) (int, error) {
	b.reads++
	return 0, io.EOF
}

func (*tokenlessProbeBody) Close() error { return nil }

func TestMCPTokenlessV14Host(t *testing.T) {
	compattest.Set(t, false)
	policyHome(t)
	for _, transport := range []string{"sse", "http"} {
		t.Run(transport, func(t *testing.T) {
			wrap, err := prepareMCPHTTP(transport, "127.0.0.1", []string{"evil.example"}, &bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				host    string
				local   bool
				allowed bool
			}{
				{"localhost:8090", true, true},
				{"LoCaLhOsT:8090", true, true},
				{"127.0.0.1:8090", true, true},
				{"[::1]:8090", true, true},
				{"evil.example:8090", true, false},
				{"localhost:8091", true, false},
				{"localhost", true, false},
				{"localhost.:8090", true, false},
				{"locaſhost:8090", true, false},
				{"127.0.0.2:8090", true, false},
				{"localhost:8090", false, false},
			} {
				t.Run(tc.host+"/local="+strconv.FormatBool(tc.local), func(t *testing.T) {
					body := &tokenlessProbeBody{}
					ran := false
					inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						ran = true
						_, _ = io.ReadAll(r.Body)
						w.WriteHeader(http.StatusOK)
					})
					req := httptest.NewRequest(http.MethodPost, "http://localhost:8090/", nil)
					req.Host, req.Body = tc.host, body
					if tc.local {
						addr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8090}
						req = req.WithContext(context.WithValue(req.Context(), http.LocalAddrContextKey, addr))
					}
					rec := httptest.NewRecorder()
					mcpHandler(wrap, inner).ServeHTTP(rec, req)
					if tc.allowed {
						if rec.Code != http.StatusOK || !ran {
							t.Fatalf("loopback: status=%d tool ran=%v", rec.Code, ran)
						}
						return
					}
					if rec.Code != http.StatusMisdirectedRequest || ran || body.reads != 0 {
						t.Fatalf("refusal: status=%d tool ran=%v body reads=%d", rec.Code, ran, body.reads)
					}
					if rec.Body.String() != mcpHostRefusal || rec.Header().Get("Access-Control-Allow-Origin") != "" {
						t.Fatalf("unsafe refusal: body=%q headers=%v", rec.Body.String(), rec.Header())
					}
				})
			}
		})
	}
}
