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
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

// newMCPHTTPServer builds the http.Server for the sse/http transports.
func newMCPHTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
	}
}

// serveMCP serves s over transport, binding host:port for sse and http. wrap
// guards the HTTP handler (nil means none); stdio ignores it.
func serveMCP(s *server.MCPServer, transport, host string, port int, wrap func(http.Handler) http.Handler) error {
	if wrap == nil {
		wrap = mcpPassthrough
	}
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
