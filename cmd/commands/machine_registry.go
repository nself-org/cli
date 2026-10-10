package commands

// The machine registry: the v1.5 registry of the running binary, with the
// installed plugin commands mounted, that every machine surface generates from
// (EPIC P7-SURF D2).
//
// Purpose: MCP tools, HTTP routes and the OpenAPI document are generated from
// one registry whatever compat mode the server process runs in, so tool names
// and routes are the committed v1.5 shape in 1.4.x too, and the child that
// internal/invoke starts always prints envelopes.
//
// Inputs: RootCmd, the embedded canon, the installed plugin directory.
//
// Outputs: a memoised *cmdregistry.Registry (machineRegistry).
//
// Constraints: machineRegistry mutates the process cobra tree (v1.5 canon
// relocation, plugin mount) and restores it before it returns, so it is for
// long-lived server commands (mcp, admin serve) that never execute a command
// in-process: they run every command by self-exec. It flips NSELF_V15 for the
// duration of the build, because the mount and the engine read the mode from
// the environment; call it once at server start, before request goroutines
// exist. The plugin set is read at the first call; a plugin installed later
// needs a server restart. Plugin nodes the process already mounted (builtin
// families and installed plugins, mounted under the process's own mode) are lifted
// off, mounted afresh under v1.5 and put back as the same nodes. The mount is called directly instead of through
// prepareTree's installed hook because that hook skips mounting when the first
// argv word is a flag, and a server started as `nself --x mcp` must still list
// its plugins.

import (
	"fmt"
	"os"
	"sync"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/plugin/mount"
	"github.com/spf13/cobra"
)

var machineCache struct {
	mu   sync.Mutex
	done bool
	reg  *cmdregistry.Registry
	err  error
}

// machineRegistry returns the v1.5 registry with installed plugin mounts,
// building it on first use.
func machineRegistry() (*cmdregistry.Registry, error) {
	machineCache.mu.Lock()
	defer machineCache.mu.Unlock()
	if !machineCache.done {
		machineCache.reg, machineCache.err = buildMachineRegistry()
		machineCache.done = true
	}
	return machineCache.reg, machineCache.err
}

// resetMachineRegistry drops the memoised registry (tests only).
func resetMachineRegistry() {
	machineCache.mu.Lock()
	defer machineCache.mu.Unlock()
	machineCache.done, machineCache.reg, machineCache.err = false, nil, nil
}

// buildMachineRegistry prepares the tree as v1.5 with plugins mounted, builds
// the registry and puts the tree back.
func buildMachineRegistry() (*cmdregistry.Registry, error) {
	// The mount and the engine read the mode from the environment, so the build
	// runs with NSELF_V15=1 whatever the process mode is, and puts it back.
	prev, had := os.LookupEnv(compat.EnvVar)
	_ = os.Setenv(compat.EnvVar, "1")
	defer func() {
		if had {
			_ = os.Setenv(compat.EnvVar, prev)
		} else {
			_ = os.Unsetenv(compat.EnvVar)
		}
	}()
	ApplyCommandGroups()
	RootCmd.InitDefaultHelpCmd()
	c, err := canonLoad(true)
	if err != nil {
		return nil, fmt.Errorf("command canon: %w", err)
	}
	// Plugin nodes the invocation already mounted (builtin families and
	// installed plugins) were mounted under the process's own mode, which sets
	// their group; lift them off and mount afresh under v1.5.
	lifted := detachMounts(RootCmd)
	undo := prepareTree(RootCmd, true, false)
	specs, problems := mount.Discover(resolvePluginDir(), canonTable.Verbs)
	mountInstalled(RootCmd, specs, problems)
	reg, buildErr := cmdregistry.Build(RootCmd, c, jsonDataTypes, cmdregistry.BuildOptions{
		V15:             true,
		V15OnlyEnvelope: jsonV15OnlyEnvelope,
	})
	undo()
	detachMounts(RootCmd)
	RootCmd.AddCommand(lifted...)
	if buildErr != nil {
		return nil, fmt.Errorf("machine registry: %w", buildErr)
	}
	return reg, nil
}

// detachMounts removes the plugin-mounted nodes (builtin and installed) from
// root and returns them.
func detachMounts(root *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	for _, n := range root.Commands() {
		if n.Annotations[mount.AnnSource] != "" {
			out = append(out, n)
		}
	}
	root.RemoveCommand(out...)
	return out
}
