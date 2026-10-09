package commands

// Purpose: Plugin lifecycle mutation commands split out of plugin.go
// (CLI-R12 Batch B mechanical file-size split). Holds the RunE handlers for
// `nself plugin remove/update/updates/refresh/start/stop/disable/enable` —
// every subcommand that changes installed-plugin state rather than only
// reporting on it.
// Inputs: cobra command flags and positional plugin name args, plus the
// shared resolvePluginDir()/loadConfig()/plugin package helpers defined in
// plugin.go.
// Outputs: stderr/stdout progress messages; errors wrap the underlying
// plugin package failures.
// Constraints: pure move, no behavior change — cobra.Command var
// declarations and RunE wiring remain in plugin.go.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/reconcile"

	"github.com/spf13/cobra"
)

func runPluginRemove(cmd *cobra.Command, args []string) error {
	name := args[0]
	keepData, _ := cmd.Flags().GetBool("keep-data")
	force, _ := cmd.Flags().GetBool("force")

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	pluginDir := resolvePluginDir()
	ctx := context.Background()

	fmt.Fprintf(os.Stderr, "Removing plugin %q...\n", name)
	if err := plugin.Remove(ctx, cfg, name, pluginDir, keepData, force); err != nil {
		return fmt.Errorf("removing plugin %q: %w", name, err)
	}

	fmt.Fprintf(os.Stderr, "Plugin %q removed successfully.\n", name)
	// compat.V15(P7-LIVE-06): plugin removal leaves generated state for a later build -> reconcile now
	if compat.V15() {
		return reconcileAfterExtension(cmd, reconcile.Trigger{Kind: reconcile.TriggerPlugin, Subject: name})
	}
	return nil
}

func runPluginUpdate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	registryURL := os.Getenv("NSELF_PLUGIN_REGISTRY")
	if err := plugin.ValidateNetworkAccess(ctx, registryURL); err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	pluginDir := resolvePluginDir()

	if len(args) == 1 {
		name := args[0]
		fmt.Fprintf(os.Stderr, "Updating plugin %q...\n", name)
		if err := plugin.Update(ctx, cfg, name, pluginDir); err != nil {
			return fmt.Errorf("updating plugin %q: %w", name, err)
		}
		fmt.Fprintf(os.Stderr, "Plugin %q updated successfully.\n", name)
		return nil
	}

	// Update all installed plugins.
	plugins, err := plugin.List(pluginDir, true)
	if err != nil {
		return fmt.Errorf("listing installed plugins: %w", err)
	}
	if len(plugins) == 0 {
		fmt.Println("No plugins installed.")
		return nil
	}

	var failures []string
	for _, p := range plugins {
		fmt.Fprintf(os.Stderr, "Updating %s...\n", p.Name)
		if err := plugin.Update(ctx, cfg, p.Name, pluginDir); err != nil {
			fmt.Fprintf(os.Stderr, "  Failed to update %s: %v\n", p.Name, err)
			failures = append(failures, p.Name)
			continue
		}
		fmt.Fprintf(os.Stderr, "  %s updated.\n", p.Name)
	}

	if len(failures) > 0 {
		return fmt.Errorf("failed to update: %s", strings.Join(failures, ", "))
	}
	return nil
}

func runPluginUpdates(cmd *cobra.Command, args []string) error {
	registryURL := os.Getenv("NSELF_PLUGIN_REGISTRY")
	if err := plugin.ValidateNetworkAccess(cmd.Context(), registryURL); err != nil {
		return err
	}

	pluginDir := resolvePluginDir()

	// Get installed plugins.
	installed, err := plugin.List(pluginDir, true)
	if err != nil {
		return fmt.Errorf("listing installed plugins: %w", err)
	}
	if len(installed) == 0 {
		fmt.Println("No plugins installed.")
		return nil
	}

	// Get registry plugins for version comparison.
	registry, err := plugin.List(pluginDir, false)
	if err != nil {
		return fmt.Errorf("fetching registry: %w", err)
	}

	// Build a map of registry versions.
	regVersions := make(map[string]string)
	for _, p := range registry {
		regVersions[p.Name] = p.Version
	}

	hasUpdates := false
	for _, p := range installed {
		regVer, found := regVersions[p.Name]
		if !found {
			continue
		}
		if regVer != p.Version {
			fmt.Printf("%-20s %s -> %s\n", p.Name, p.Version, regVer)
			hasUpdates = true
		}
	}

	if !hasUpdates {
		fmt.Println("All plugins are up to date.")
	}

	return nil
}

func runPluginRefresh(cmd *cobra.Command, args []string) error {
	registryURL := os.Getenv("NSELF_PLUGIN_REGISTRY")
	if err := plugin.ValidateNetworkAccess(cmd.Context(), registryURL); err != nil {
		return err
	}

	ctx := context.Background()
	cacheDir := os.Getenv("NSELF_PLUGIN_CACHE")
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			cacheDir = filepath.Join("/tmp", ".nself", "cache", "plugins")
		} else {
			cacheDir = filepath.Join(home, ".nself", "cache", "plugins")
		}
	}

	// Remove cached registry file to force a fresh fetch.
	cachePath := filepath.Join(cacheDir, "registry.json")
	_ = os.Remove(cachePath)

	fmt.Fprintln(os.Stderr, "Refreshing plugin registry...")
	_, err := plugin.FetchRegistry(ctx, "", cacheDir)
	if err != nil {
		return fmt.Errorf("refreshing registry: %w", err)
	}

	fmt.Fprintln(os.Stderr, "Registry cache refreshed.")
	return nil
}

func runPluginStart(cmd *cobra.Command, args []string) error {
	name := args[0]
	pluginDir := resolvePluginDir()
	ctx := context.Background()

	pluginPath := filepath.Join(pluginDir, name)
	fmt.Fprintf(os.Stderr, "Starting plugin %q...\n", name)
	if err := plugin.Start(ctx, pluginPath, name); err != nil {
		return fmt.Errorf("starting plugin %q: %w", name, err)
	}

	fmt.Fprintf(os.Stderr, "Plugin %q started.\n", name)
	return nil
}

func runPluginStop(cmd *cobra.Command, args []string) error {
	name := args[0]
	ctx := context.Background()

	fmt.Fprintf(os.Stderr, "Stopping plugin %q...\n", name)
	if err := plugin.Stop(ctx, name); err != nil {
		return fmt.Errorf("stopping plugin %q: %w", name, err)
	}

	fmt.Fprintf(os.Stderr, "Plugin %q stopped.\n", name)
	return nil
}

func runPluginDisable(cmd *cobra.Command, args []string) error {
	name := args[0]
	pluginDir := resolvePluginDir()

	fmt.Fprintf(os.Stderr, "Disabling plugin %q...\n", name)
	if err := plugin.DisablePlugin(name, pluginDir); err != nil {
		return fmt.Errorf("disabling plugin %q: %w", name, err)
	}

	fmt.Fprintf(os.Stderr, "Plugin %q disabled. Run 'nself build' to update your stack.\n", name)
	return nil
}

func runPluginEnable(cmd *cobra.Command, args []string) error {
	name := args[0]
	pluginDir := resolvePluginDir()

	fmt.Fprintf(os.Stderr, "Enabling plugin %q...\n", name)
	if err := plugin.EnablePlugin(name, pluginDir); err != nil {
		return fmt.Errorf("enabling plugin %q: %w", name, err)
	}

	fmt.Fprintf(os.Stderr, "Plugin %q enabled. Run 'nself build' to update your stack.\n", name)
	return nil
}
