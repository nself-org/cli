package commands

// mcp_transport.go — the transport switch of `nself mcp`.
//
// Purpose: serve an already-populated MCP server over stdio, SSE or
//   streamable HTTP. Split out of mcp.go (P7-SURF-25, the retired P7-SURF-01
//   split) so mcp.go has room for the bind and token policy.
// Inputs:  the MCP server, the transport name, the bind host and port, and
//   the handler wrapper chosen by mcpAuthPolicy (mcp_auth.go).
// Outputs: blocks serving until the transport ends; an error for an unknown
//   transport or a listener failure.
// Constraints: sse/http wrap the handler in wrap. The HTTP server sets read
//   timeouts and no write timeout (SSE streams stay open). Nothing from a
//   request (headers, bodies, tokens) is logged here.
// SPORT: CLI-CMD-MCP-001

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

// Read limits of the sse/http listener (EPIC SURF D4). Variables so a test
// can shorten them.
var (
	mcpReadHeaderTimeout = 10 * time.Second
	mcpReadTimeout       = 30 * time.Second
)

// mcpMaxBodyBytes bounds one MCP request body read under the read deadline.
const mcpMaxBodyBytes = 32 << 20

// newMCPHTTPServer builds the http.Server for the sse/http transports.
func newMCPHTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           mcpBodyThenStream(h),
		ReadHeaderTimeout: mcpReadHeaderTimeout,
		ReadTimeout:       mcpReadTimeout,
	}
}

// mcpBodyThenStream reads the request body under the server's ReadTimeout,
// then clears the read deadline before h runs. Without that, ReadTimeout
// keeps running while an SSE stream or a long tool call writes, and net/http
// cancels the request context when it expires (a slow body still meets the
// deadline; headers are bounded by ReadHeaderTimeout).
func mcpBodyThenStream(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.Body != http.NoBody {
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, mcpMaxBodyBytes))
			if err != nil {
				http.Error(w, "request body unreadable or too large", http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		_ = http.NewResponseController(w).SetReadDeadline(time.Time{})
		h.ServeHTTP(w, r)
	})
}

// serveMCP serves s over transport, binding host:port for sse and http. wrap
// guards the HTTP handler (nil means none); stdio ignores it.
func serveMCP(s *server.MCPServer, transport, host string, port int, wrap func(http.Handler) http.Handler) error {
	if wrap == nil {
		wrap = mcpPassthrough
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]") // JoinHostPort adds the brackets
	switch transport {
	case "stdio":
		return server.ServeStdio(s)
	case "sse":
		addr := net.JoinHostPort(host, strconv.Itoa(port))
		fmt.Fprintf(os.Stderr, "nSelf MCP server listening on %s (SSE)\n", addr)
		return newMCPHTTPServer(addr, wrap(server.NewSSEServer(s))).ListenAndServe()
	case "http":
		addr := net.JoinHostPort(host, strconv.Itoa(port))
		fmt.Fprintf(os.Stderr, "nSelf MCP server listening on %s (streamable HTTP)\n", addr)
		return newMCPHTTPServer(addr, wrap(server.NewStreamableHTTPServer(s))).ListenAndServe()
	default:
		return fmt.Errorf("unknown transport %q (use stdio, sse, or http)", transport)
	}
}
