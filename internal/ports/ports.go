// Package ports provides canonical port constants for nSelf services and
// port-holder identification utilities for the nSelf CLI.
//
// Purpose: Single source of truth for all nSelf service ports used by the CLI.
//
//	These constants must match F10-PORT-REGISTRY.md in the canonical SPORT.
//
// Inputs: None (compile-time constants).
// Outputs: Typed int constants for use in CLI commands.
// Constraints: Never hardcode these values elsewhere in cli/; always import.
// SPORT: F10-PORT-REGISTRY.md — update both files together.
package ports

// AdminAPIPort is the API-only admin listener reserved by SPORT F10-PORT-REGISTRY.
const AdminAPIPort = 3857

// AI gateway ports (E6 canonical trio).
const (
	// AICCPort is the port for nself-ai-cc (Claude Code PTY session relay).
	// SPORT F10: nself-ai-cc | HTTP/WS | 3760
	AICCPort = 3760

	// AIGatewayPort is the port for nself-ai-gateway (AI provider key vault + router).
	// SPORT F10: nself-ai-gateway | HTTP | 3761
	AIGatewayPort = 3761

	// AIMCPPort is the port for nself-ai-mcp (MCP tool server).
	// SPORT F10: nself-ai-mcp | HTTP | 3762
	AIMCPPort = 3762
)
