package commands

import (
	"fmt"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// `nself install` / `nself remove` — the short way to extend the CLI.
//
// Purpose: the product line is "the core does 95% of what you need, everything
// else is one `nself install X` away". That sentence was not true: the actual
// spelling was `nself plugin install X`, and a user who typed a command the
// core does not have got a plugin-proxy error rather than a way forward.
//
// Inputs: one or more names, each resolved as a bundle first and a plugin
// second (bundle names are a small curated set; plugin names are the registry).
//
// Outputs: delegates to the existing bundle and plugin install paths. This file
// adds no installation logic of its own — a second implementation would drift
// from the licence and checksum handling in internal/plugin.
//
// Constraints: `install` and `remove` must stay thin. Anything that belongs to
// installation belongs in internal/plugin or internal/bundle, not here.

var installCmd = &cobra.Command{
	Use:   "install <name> [name...]",
	Short: "Install a plugin or bundle",
	Long: `Install a plugin or bundle by name.

This is the short form of ` + "`nself plugin install`" + `, with bundle awareness: if
the name is a bundle (` + "`nchat`, `nclaw`, `ntv`, `nfamily`, `clawde`, `nsentry`" + `)
the whole bundle is installed, otherwise the name is resolved as a plugin.

Third-party plugins install by URL rather than by name:

  nself install https://example.com/my-plugin.tar.gz

Once installed, the plugin's commands are available directly:

  nself install waf
  nself waf status

Examples:
  nself install waf                  # one plugin
  nself install waf cdn analytics    # several at once
  nself install nchat                # a whole bundle`,
	Args:    cobra.MinimumNArgs(1),
	Aliases: []string{"add"},
	RunE:    runInstall,
}

var removeCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an installed plugin or bundle",
	Long: `Remove an installed plugin or bundle by name.

This is the short form of ` + "`nself plugin remove`" + `. As with install, a bundle
name removes the whole bundle.

Examples:
  nself remove waf
  nself remove nchat`,
	Args:    cobra.ExactArgs(1),
	Aliases: []string{"rm"},
	RunE:    runRemove,
}

func init() {
	// Flags are forwarded to the underlying plugin/bundle commands, so the two
	// surfaces cannot drift apart in what they accept.
	installCmd.Flags().Bool("yes", false, "Skip confirmation prompts (required for third-party URL installs in CI)")
	installCmd.Flags().Bool("force", false, "Reinstall even when the plugin is already present")
	installCmd.Flags().String("key", "", "License key for Licensed plugins")
	installCmd.Flags().String("tier", "", `Force "free" or "licensed" for a slug served as both (e.g. cron, notify); default resolves by license entitlement`)
	installCmd.Flags().String("version", "", "Install a specific version")
	installCmd.Flags().Bool("allow-eol", false, "Allow installing an EOL plugin (not recommended)")
	installCmd.Flags().Bool("preview", false, "Preview the dependency tree without installing")
	installCmd.Flags().Bool("with-optional", false, "Include optional dependencies in --preview output")
	installCmd.Flags().Bool("skip-sbom-check", false, "Skip SBOM verification (air-gapped installs only — sets NSELF_SKIP_SBOM_CHECK=1)")
	installCmd.Flags().Bool("dry-run", false, "Show what would be installed without making changes")
	installCmd.Flags().Bool("show-graph", false, "Show dependency graph with topological sort order")
	installCmd.Flags().String("checksum", "", "Expected SHA-256 checksum of the downloaded archive (third-party URL installs only)")
	installCmd.Flags().Bool("strict", false, "Fail if any plugin in the bundle is missing from the registry")
	installCmd.Flags().String("channel", "stable", "Release channel: stable | beta | canary")

	removeCmd.Flags().Bool("yes", false, "Skip confirmation prompts")
	removeCmd.Flags().Bool("keep-data", false, "Preserve database data on remove")
	removeCmd.Flags().Bool("force", false, "Remove even if other plugins depend on this one")
	removeCmd.Flags().Bool("dry-run", false, "Print the planned actions without removing")

	RootCmd.AddCommand(installCmd)
	RootCmd.AddCommand(removeCmd)
}

// resolvesToBundle reports whether name is one of the curated bundles rather
// than a plugin. Bundles are checked first because a bundle and a plugin can
// share a name (the `nsentry` bundle contains an `nself-sentry-*` plugin set),
// and installing the bundle is what the user meant.
func resolvesToBundle(name string) bool {
	b, ok := bundle.Get(name)
	return ok && b.IsInstallable()
}

func runInstall(cmd *cobra.Command, args []string) error {
	if handled, err := toggleBuiltin(args, true); handled {
		return err
	}
	var bundles, plugins []string
	for _, name := range args {
		if resolvesToBundle(name) {
			bundles = append(bundles, name)
			continue
		}
		plugins = append(plugins, name)
	}

	for _, name := range bundles {
		ui.Info(fmt.Sprintf("%q is a bundle — installing every plugin it contains.", name))
		if err := runBundleInstall(cmd, []string{name}); err != nil {
			return fmt.Errorf("install bundle %q: %w", name, err)
		}
	}

	if len(plugins) > 0 {
		if err := runPluginInstall(cmd, plugins); err != nil {
			return err
		}
		preview, _ := cmd.Flags().GetBool("preview")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		showGraph, _ := cmd.Flags().GetBool("show-graph")
		if !preview && !dryRun && !showGraph {
			printAddedCommandHints(plugins)
		}
	}
	return nil
}

func runRemove(cmd *cobra.Command, args []string) error {
	if handled, err := toggleBuiltin(args, false); handled {
		return err
	}
	name := args[0]

	if resolvesToBundle(name) {
		return runBundleRemove(cmd, []string{name})
	}
	return runPluginRemove(cmd, []string{name})
}
