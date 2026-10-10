package commands

// mcp_auth.go — bind and bearer-token policy for the SSE/HTTP MCP transports.
//
// Purpose: P7-SURF-25 (EPIC SURF D4). Decide, before the listener opens,
//   whether the requested bind is allowed and which bearer token (if any)
//   guards it. The token rules live in internal/machineauth; this file only
//   wires them to `nself mcp`.
// Inputs:  --host, --allowed-host, NSELF_MCP_TOKEN, ~/.nself/mcp-token, and
//   the compat mode (NSELF_V15).
// Outputs: a handler wrapper, the warnings to print on stderr (one cleartext
//   warning for an accepted non-loopback bind, one open-server warning in
//   v1.4 mode with no token), or an E426 refusal.
// Constraints: stdio never calls this. A refused start creates no token file:
//   only the v1.5 loopback path with no configured token generates one. The
//   v1.4 behaviour with a token set is byte-identical (same 401 body and
//   header). No token value is ever printed or logged.
// SPORT: CLI-CMD-MCP-001

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/machineauth"
)

// mcpTokenEnv is the env var that carries the bearer token for sse/http.
const mcpTokenEnv = machineauth.EnvVar

// mcpRealm is the WWW-Authenticate realm of the MCP bearer check.
const mcpRealm = "nself-mcp"

// mcpPassthrough leaves a handler unwrapped.
func mcpPassthrough(h http.Handler) http.Handler { return h }

// mcpBearerWith returns the bearer wrapper for tok, or a passthrough when tok
// is empty.
func mcpBearerWith(tok string) func(http.Handler) http.Handler {
	if tok == "" {
		return mcpPassthrough
	}
	return machineauth.Middleware(tok, mcpRealm)
}

// mcpBearerMiddleware wraps h with a bearer-token check driven by
// NSELF_MCP_TOKEN. Returns h unchanged when the env var is empty.
func mcpBearerMiddleware(h http.Handler) http.Handler {
	return mcpBearerWith(os.Getenv(mcpTokenEnv))(h)
}

// mcpAuthPolicy applies the bind and token policy for an sse/http start. It
// returns the wrapper to put around the transport handler plus the warning
// lines the caller prints to stderr. notice receives the generated-token
// path message (v1.5 first start only).
func mcpAuthPolicy(host string, allowed []string, notice io.Writer) (func(http.Handler) http.Handler, []string, error) {
	tok, src, err := machineauth.Lookup()
	if err != nil {
		return nil, nil, err
	}
	if err := machineauth.CheckBind(host, allowed, src); err != nil {
		return nil, nil, err
	}
	var warns []string
	if !machineauth.IsLoopback(host) {
		warns = append(warns, fmt.Sprintf("warning: serving the MCP bearer token over cleartext HTTP on host %s; the token crosses the network unencrypted, so put a TLS terminator in front", host))
	}
	// compat.V15(P7-SURF-25): no token = open loopback with a warning -> token required (generated on first start)
	if compat.V15() {
		if tok == "" {
			if tok, _, _, err = machineauth.ResolveTo(notice); err != nil {
				return nil, nil, err
			}
		}
	} else if tok == "" {
		warns = append(warns, "warning: nself mcp is serving without a bearer token on loopback; any local process can use it. Set NSELF_MCP_TOKEN to require one")
	}
	return mcpBearerWith(tok), warns, nil
}

// prepareMCPHTTP runs mcpAuthPolicy for the sse and http transports and
// prints its warnings; stdio and unknown transports get a passthrough.
func prepareMCPHTTP(transport, host string, allowed []string, errw io.Writer) (func(http.Handler) http.Handler, error) {
	if transport != "sse" && transport != "http" {
		return mcpPassthrough, nil
	}
	wrap, warns, err := mcpAuthPolicy(host, allowed, errw)
	if err != nil {
		return nil, err
	}
	for _, w := range warns {
		fmt.Fprintln(errw, w)
	}
	return wrap, nil
}
