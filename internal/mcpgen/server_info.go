package mcpgen

// Purpose: the server name and version, and the experimental marker.
// Inputs: the compat mode. Outputs: the initialize identity and server options.
// Constraints: v1.4 mode advertises `experimental: true` (generated tools are
// additive until the v1.5 flip); P7-SURF-23 removes the flag.

import "github.com/mark3labs/mcp-go/server"

// ServerName and ServerVersion identify the server in the initialize result.
const (
	ServerName    = "nSelf MCP Server"
	ServerVersion = "2.0.0"
)

// ServerOptions are the extra options for server.NewMCPServer.
func ServerOptions(v15 bool) []server.ServerOption {
	opts := []server.ServerOption{
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
		server.WithPromptCapabilities(true),
	}
	// compat.V15(P7-SURF-03): the initialize result carries experimental:true -> the flag is gone (P7-SURF-23)
	if !v15 {
		opts = append(opts, server.WithExperimental(map[string]any{"experimental": true}))
	}
	return opts
}
