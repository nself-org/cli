package mcpgen

// Purpose: the server name and version, and the server options.
// Inputs: none. Outputs: the initialize identity and server options.
// Constraints: no mode branch and no experimental flag; the generated surface
// is the contract (P7-SURF-23 removed the flag v1.4 mode advertised).

import "github.com/mark3labs/mcp-go/server"

// ServerName and ServerVersion identify the server in the initialize result.
const (
	ServerName    = "nSelf MCP Server"
	ServerVersion = "2.0.0"
)

// ServerOptions are the extra options for server.NewMCPServer.
func ServerOptions() []server.ServerOption {
	return []server.ServerOption{
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithPromptCapabilities(true),
	}
}
