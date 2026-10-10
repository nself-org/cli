package invoke

// Purpose: decide whether a command may run over a machine surface.
// Inputs: a registry command, the transport and the flags the request names.
// Outputs: ok and, when refused, a reason that becomes the E421 cause.
// Constraints: the rules are EPIC D2's "Exposed =" list. Remote and destructive
// commands are refused until the confirmation gate (P7-SURF-25) exists; that
// Ticket replaces the "no gate" rule below and nothing else.

import (
	"fmt"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
)

// Exposure reports whether cmd may be invoked on transport with the flags in
// set (the names the request carries). A refusal names the rule in reason.
func Exposure(cmd *cmdregistry.Command, transport string, set map[string]bool) (bool, string) {
	switch {
	case cmd == nil:
		return false, "no such command"
	case cmd.Surface == "cli-only":
		return false, "surface cli-only: this command never crosses MCP or HTTP"
	case cmd.Hidden:
		return false, "hidden command"
	case cmd.Deprecated != nil:
		return false, "deprecated command"
	case !cmd.Runnable:
		return false, "not a runnable command"
	}
	switch cmd.Canon {
	case canon.CanonCore, canon.CanonSubcommand, canon.CanonPlugin:
	case canon.CanonPending:
		return false, "pending command (not yet in the canon)"
	case canon.CanonShim:
		return false, "deprecated shim: call its target"
	case canon.CanonBuiltin:
		return false, "builtin framework command"
	default:
		return false, fmt.Sprintf("canon %q is not exposed", cmd.Canon)
	}
	if cmd.JSON != canon.JSONEnvelope {
		return false, "no JSON envelope"
	}
	side := cmdregistry.EffectiveSideEffect(cmd, set)
	switch cmdregistry.EffectiveOutput(cmd, set) {
	case canon.OutputDocument:
		if transport == TransportHTTPStream {
			return false, "not a stream command"
		}
	case canon.OutputStream:
		if transport != TransportHTTPStream {
			return false, "stream output is served only as HTTP NDJSON"
		}
		if side != canon.SideEffectRead {
			return false, "stream output with a " + side + " side effect"
		}
	default:
		return false, "interactive command"
	}
	switch side {
	case canon.SideEffectRead, canon.SideEffectWrite:
		return true, ""
	case canon.SideEffectRemote, canon.SideEffectDestructive:
		return false, "no gate: " + side + " commands wait for the confirmation gate (P7-SURF-25)"
	}
	return false, fmt.Sprintf("unknown side effect %q", side)
}
