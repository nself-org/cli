package commands

import (
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/version"

	"github.com/spf13/cobra"
)

// pluginCompatCheckCmd is the `nself plugin compat-check` command. (S58-T06)
// It walks installed plugins and flags any whose maxNselfVersion is less than
// the running CLI version, then prints an install-method-aware upgrade hint.
// Exit code 0 = all plugins compatible; non-zero = at least one incompatible.
var pluginCompatCheckCmd = &cobra.Command{
	Use:   "compat-check",
	Short: "Check installed plugins against the current CLI version",
	Long: `Walk every installed plugin and verify it is compatible with the current
CLI version. A plugin declares compatibility via minNselfVersion and
maxNselfVersion in its manifest. If maxNselfVersion is older than the current
CLI, the plugin is flagged as incompatible and an upgrade hint is printed.

This command runs automatically after 'nself self update' completes.

Exit codes:
  0  All installed plugins are compatible
  1  One or more plugins are incompatible (details printed to stderr)
`,
	RunE: runPluginCompatCheck,
}

func init() {
	pluginCompatCheckCmd.Flags().Bool("json", false, "Output results as JSON")
	pluginCmd.AddCommand(pluginCompatCheckCmd)
}

// compatResult holds compat-check findings for a single plugin.
type compatResult struct {
	Name       string
	Installed  string
	Status     string
	Compatible bool
	Reason     string
	Hint       string
}

func runPluginCompatCheck(cmd *cobra.Command, args []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")
	pluginDir := resolvePluginDir()
	cliVer := version.GetVersion()

	ctx := cmd.Context()
	cacheDir := plugin.DefaultCacheDir()
	reg, regErr := plugin.FetchRegistry(ctx, "", cacheDir)
	if regErr != nil {
		// Offline mode: emit a warning and check only installed manifests.
		fmt.Fprintf(os.Stderr, "warning: registry unavailable (offline mode) — checking local manifests only\n")
	}

	installed, err := plugin.ListInstalled(pluginDir)
	if err != nil {
		return fmt.Errorf("listing installed plugins: %w", err)
	}

	if len(installed) == 0 {
		if jsonEnvelopeOn(cmd) {
			return emitEnv(cmd, newCompatCheckData(nil), true) // P7-SURF-13
		}
		if !jsonOut {
			fmt.Println("No plugins installed.")
		}
		return nil
	}

	var results []compatResult
	incompatCount := 0

	for _, p := range installed {
		res := compatResult{
			Name:       p.Name,
			Installed:  p.Version,
			Status:     p.Status,
			Compatible: true,
		}

		// Look up maxNselfVersion from registry if available.
		if reg != nil {
			if manifest, found := plugin.FindPluginByName(reg, p.Name); found {
				if manifest.MaxNselfVersion != "" {
					if plugin.CompareVersions(cliVer, manifest.MaxNselfVersion) > 0 {
						res.Compatible = false
						res.Reason = fmt.Sprintf("plugin max CLI version %s < current CLI %s",
							manifest.MaxNselfVersion, cliVer)
						// Hint: install-method-aware upgrade suggestion.
						// detectInstallMethod is defined in upgrade.go (same package).
						switch detectInstallMethod() {
						case "homebrew":
							res.Hint = fmt.Sprintf("brew upgrade nself && nself plugin update %s", p.Name)
						case "direct":
							res.Hint = fmt.Sprintf("curl -sSL https://install.nself.org | bash && nself plugin update %s", p.Name)
						default:
							res.Hint = fmt.Sprintf("nself plugin update %s", p.Name)
						}
					}
				}
			}
		}

		if !res.Compatible {
			incompatCount++
		}
		results = append(results, res)
	}

	if jsonEnvelopeOn(cmd) {
		if err := emitEnv(cmd, newCompatCheckData(results), true); err != nil { // P7-SURF-13
			return err
		}
		if incompatCount > 0 {
			// The envelope is the answer; the human message goes to stderr and the exit stays 1.
			fmt.Fprintf(os.Stderr, "%d incompatible plugin(s) found\n", incompatCount)
			return errs.Exit(1)
		}
		return nil
	}
	if jsonOut {
		printCompatJSON(results, cliVer)
	} else {
		printCompatTable(results, cliVer, incompatCount)
	}

	if incompatCount > 0 {
		return fmt.Errorf("%d incompatible plugin(s) found", incompatCount)
	}
	return nil
}

func printCompatTable(results []compatResult, cliVer string, incompatCount int) {
	fmt.Printf("CLI version: %s\n\n", cliVer)
	fmt.Printf("%-22s %-9s %-12s %s\n", "Plugin", "Version", "Status", "Compat")
	fmt.Printf("%-22s %-9s %-12s %s\n", "------", "-------", "------", "------")
	for _, r := range results {
		compat := "OK"
		if !r.Compatible {
			compat = "INCOMPATIBLE"
		}
		fmt.Printf("%-22s %-9s %-12s %s\n", r.Name, r.Installed, r.Status, compat)
		if !r.Compatible {
			fmt.Printf("  reason: %s\n", r.Reason)
			if r.Hint != "" {
				fmt.Printf("  fix:    %s\n", r.Hint)
			}
		}
	}
	fmt.Println()
	if incompatCount == 0 {
		fmt.Println("All installed plugins are compatible.")
	} else {
		fmt.Printf("%d incompatible plugin(s). Run the fix commands above to resolve.\n", incompatCount)
	}
}

func printCompatJSON(results []compatResult, cliVer string) {
	fmt.Printf(`{"cliVersion":%q,"plugins":[`, cliVer)
	for i, r := range results {
		if i > 0 {
			fmt.Print(",")
		}
		fmt.Printf(`{"name":%q,"version":%q,"status":%q,"compatible":%v,"reason":%q,"hint":%q}`,
			r.Name, r.Installed, r.Status, r.Compatible, r.Reason, r.Hint)
	}
	fmt.Println("]}")
}
