package invoke

// Purpose: decide whether a command may run over a machine surface.
// Inputs: a registry command, the transport and the flags the request names.
// Outputs: ok and, when refused, a reason that becomes the E421 cause.
// Constraints: the rules are EPIC D2's "Exposed =" list. Remote and destructive
// commands are refused until the confirmation gate (P7-SURF-25) exists; that
// Ticket replaces the "no gate" rule below and nothing else.

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/errs"
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

// IsLeaf reports whether no registry command sits below cmd. A nil registry
// is taken as a registry of leaves.
func IsLeaf(reg *cmdregistry.Registry, cmd *cmdregistry.Command) bool {
	if reg == nil || cmd == nil {
		return true
	}
	prefix := cmd.Path + " "
	for i := range reg.Commands {
		if strings.HasPrefix(reg.Commands[i].Path, prefix) {
			return false
		}
	}
	return true
}

// FreeFormArgv reports whether cmd takes the free-form argv member: a plugin
// command that declares no args and no flags and has no subcommand. A mounted
// plugin node parses no flags, so the cobra tree in the child reads the first
// word of a free-form argv as a subcommand name; on a parent that would run a
// node the exposure rules never checked.
func FreeFormArgv(reg *cmdregistry.Registry, cmd *cmdregistry.Command) bool {
	return freeFormArgv(cmd) && IsLeaf(reg, cmd)
}

// rootFlagWords are the argv words the child's plugin proxy strips (root
// persistent flags) or the invoker owns; free-form argv cannot carry them
// because it would not reach the plugin as sent.
func rootFlagWords(reg *cmdregistry.Registry) map[string]bool {
	words := map[string]bool{}
	for name := range invokerFlags {
		words["--"+name] = true
	}
	if reg != nil {
		for _, f := range reg.Root.Flags {
			if !f.Persistent {
				continue
			}
			words["--"+f.Name] = true
			if f.Shorthand != nil && *f.Shorthand != "" {
				words["-"+*f.Shorthand] = true
			}
		}
	}
	return words
}

// CheckFreeForm vets a request for a command that takes (or may be given)
// free-form argv. A non-empty Argv on a command that is a plugin parent is
// refused (E421); an item before "--" naming a root persistent flag is
// refused (E420).
func CheckFreeForm(reg *cmdregistry.Registry, cmd *cmdregistry.Command, r Request) error {
	if len(r.Argv) == 0 || cmd == nil {
		return nil
	}
	if freeFormArgv(cmd) && !IsLeaf(reg, cmd) {
		return errs.New("E421", "command "+quoteName(barePath(cmd.Path))+" is not exposed").
			WithWhy("free-form argv is accepted only on a leaf command: this one has subcommands, and the child would run the one named by the first word")
	}
	words := rootFlagWords(reg)
	for _, a := range r.Argv {
		if a == "--" {
			break
		}
		name, _, _ := strings.Cut(a, "=")
		if words[name] {
			return badRequest("argv item %s is a root flag that the plugin proxy would drop", quoteName(name))
		}
	}
	return nil
}
