// Package invoke is the one execution core that MCP and HTTP share
// (contract:cli.machine-request v1, EPIC P7-SURF D2).
//
// Purpose:     turn a machine request (JSON object) for a registry command into
//
//	argv, run `nself <path> --json` as a child of this binary in v1.5
//	mode, and return the child's single JSON document and exit code.
//
// Inputs:      a *cmdregistry.Registry (the machine registry), a command path, a
//
//	Request, Options (transport, project dir, timeout).
//
// Outputs:     Result: the document bytes and exit code. Every refusal is itself
//
//	a v1 error envelope: E420 invalid request, E421 not exposed, E422 no
//	single JSON document, E423 timed out.
//
// Constraints: no shell, ever: one request value is exactly one argv element
//
//	and positional values sit after "--". The child is never run
//	in-process. The child environment drops NSELF_MCP_TOKEN. Remote and
//	destructive commands are refused until the confirmation gate
//	(P7-SURF-25) exists. This package imports no cmd/ package; layer L2.
package invoke
