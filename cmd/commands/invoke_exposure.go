package commands

// Purpose: recheck machine exposure against the child's freshly mounted tree.
// Inputs: the prepared root, pinned command path, argv and invocation transport.
// Outputs: nil when exposed, otherwise E421 before any command handler runs.
// Constraints: no cached parent registry, tree mutation or plugin execution.
// SPORT: CLI-CMD-MCP-001

import (
	"os"
	"strings"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/spf13/cobra"
)

// checkInvokeExposure checks the same rules as the parent using the actual
// child tree, including installed-plugin attributes and named flag overrides.
func checkInvokeExposure(root *cobra.Command, argv []string, path string) error {
	transport := os.Getenv(invoke.TransportEnv)
	_ = os.Unsetenv(invoke.TransportEnv)
	by := os.Getenv(invoke.InvokedByEnv)
	if by == "" {
		return nil
	}
	if transport == "" {
		transport = by // compatibility with invokers predating the transport marker
	}
	refuse := func(why string) error {
		return errs.New("E421", "command "+quoteInvokePath(path)+" is not exposed").WithWhy(why)
	}
	if transport != invoke.TransportMCP && transport != invoke.TransportHTTP && transport != invoke.TransportHTTPStream {
		return refuse("unknown child invocation transport")
	}
	// A disabled builtin family keeps its canon entries and data types, so the
	// child registry must be built with every family mounted, exactly as the
	// parent does (D-0863). Without this, a pinned MCP/HTTP child whose path
	// lives under a disabled builtin family fails E421 even though the parent
	// classified it as exposed.
	undoFamilies := mountDisabledBuiltinFamilies(root)
	defer undoFamilies()
	c, err := canonLoad(true)
	if err != nil {
		return refuse("child command canon: " + err.Error())
	}
	reg, err := cmdregistry.Build(root, c, jsonDataTypes, cmdregistry.BuildOptions{
		V15: true, V15OnlyEnvelope: jsonV15OnlyEnvelope,
	})
	if err != nil {
		return refuse("child command registry: " + err.Error())
	}
	row, _ := reg.Lookup(path)
	if ok, why := invoke.Exposure(row, transport, invokeArgvFlags(argv)); !ok {
		return refuse("child exposure check: " + why)
	}
	return nil
}

// invokeArgvFlags recovers named flags from the invoker's canonical argv:
// --name=value, with positional values following --. Presence counts even
// for --flag=false, exactly as invoke.SetFlags does in the parent.
func invokeArgvFlags(argv []string) map[string]bool {
	set := map[string]bool{}
	for _, word := range argv {
		if word == "--" {
			break
		}
		if strings.HasPrefix(word, "--") {
			name, _, _ := strings.Cut(strings.TrimPrefix(word, "--"), "=")
			set[name] = true
		}
	}
	return set
}
