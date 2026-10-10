package commands

// mcp_transport.go — the transport switch of `nself mcp`.
//
// Purpose: serve an already-populated MCP server over stdio, SSE or
//   streamable HTTP. Split out of mcp.go (P7-SURF-25, the retired P7-SURF-01
//   split) so mcp.go has room for the bind and token policy.
// Inputs:  the MCP server, the transport name, the bind host and port.
// Outputs: blocks serving until the transport ends; an error for an unknown
//   transport or a listener failure.
// Constraints: sse/http wrap the handler in mcpBearerMiddleware (mcp_auth.go).
// SPORT: CLI-CMD-MCP-001

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/mark3labs/mcp-go/server"
)

// serveMCP serves s over transport, binding host:port for sse and http.
func serveMCP(s *server.MCPServer, transport, host string, port int) error {
	switch transport {
	case "stdio":
		return server.ServeStdio(s)
	case "sse":
		addr := net.JoinHostPort(host, strconv.Itoa(port))
		fmt.Fprintf(os.Stderr, "nSelf MCP server listening on %s (SSE)\n", addr)
		httpSrv := &http.Server{Addr: addr, Handler: mcpBearerMiddleware(server.NewSSEServer(s))}
		return httpSrv.ListenAndServe()
	case "http":
		addr := net.JoinHostPort(host, strconv.Itoa(port))
		fmt.Fprintf(os.Stderr, "nSelf MCP server listening on %s (streamable HTTP)\n", addr)
		httpSrv := &http.Server{Addr: addr, Handler: mcpBearerMiddleware(server.NewStreamableHTTPServer(s))}
		return httpSrv.ListenAndServe()
	default:
		return fmt.Errorf("unknown transport %q (use stdio, sse, or http)", transport)
	}
}
