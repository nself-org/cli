package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// submitManifest is the minimal manifest shape we validate for submission.
type submitManifest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Category    string `json:"category"`
	License     string `json:"license"`
	Author      string `json:"author"`
	Port        int    `json:"port"`
}

var pluginSubmitCmd = &cobra.Command{
	Use:   "submit [path]",
	Short: "Validate a plugin for submission to the registry",
	Long: `Validate your plugin directory against the submission requirements.
Checks for required files and manifest fields before you open a PR.

  nself plugin submit              # validate current directory
  nself plugin submit ./my-plugin  # validate a specific path`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPluginSubmit,
}

func init() {
	pluginSubmitCmd.Flags().Bool("strict", false, "Enable strict validation (screenshots, tests required)")
	pluginCmd.AddCommand(pluginSubmitCmd)
}

func runPluginSubmit(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}

	strict, _ := cmd.Flags().GetBool("strict")

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	ui.Info(fmt.Sprintf("Validating plugin at %s...", absDir))

	var errors []string
	var warnings []string

	// 1. Check required files exist.
	requiredFiles := []string{"plugin.yaml", "README.md", "Dockerfile"}
	for _, f := range requiredFiles {
		if _, err := os.Stat(filepath.Join(absDir, f)); os.IsNotExist(err) {
			// Also accept plugin.json as alternative to plugin.yaml.
			if f == "plugin.yaml" {
				if _, err2 := os.Stat(filepath.Join(absDir, "plugin.json")); err2 == nil {
					continue
				}
			}
			errors = append(errors, fmt.Sprintf("missing required file: %s", f))
		}
	}

	// 2. Check optional but recommended files.
	recommendedFiles := []string{"LICENSE", "CHANGELOG.md"}
	for _, f := range recommendedFiles {
		if _, err := os.Stat(filepath.Join(absDir, f)); os.IsNotExist(err) {
			warnings = append(warnings, fmt.Sprintf("missing recommended file: %s", f))
		}
	}

	if strict {
		// Tests directory
		if _, err := os.Stat(filepath.Join(absDir, "tests")); os.IsNotExist(err) {
			errors = append(errors, "missing tests/ directory (required in strict mode)")
		}
	}

	// 3. Parse and validate the manifest.
	manifestPath := filepath.Join(absDir, "plugin.json")
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		manifestPath = filepath.Join(absDir, "plugin.yaml")
	}

	if _, err := os.Stat(manifestPath); err == nil {
		data, readErr := os.ReadFile(manifestPath)
		if readErr != nil {
			errors = append(errors, fmt.Sprintf("cannot read manifest: %v", readErr))
		} else {
			var m submitManifest
			if jsonErr := json.Unmarshal(data, &m); jsonErr != nil {
				errors = append(errors, fmt.Sprintf("invalid manifest JSON: %v", jsonErr))
			} else {
				if m.Name == "" {
					errors = append(errors, "manifest: missing 'name' field")
				}
				if m.Version == "" {
					errors = append(errors, "manifest: missing 'version' field")
				}
				if m.Description == "" {
					errors = append(errors, "manifest: missing 'description' field")
				}
				if m.Category == "" {
					errors = append(errors, "manifest: missing 'category' field")
				}
				if m.License == "" {
					errors = append(errors, "manifest: missing 'license' field")
				}
				if m.Author == "" {
					warnings = append(warnings, "manifest: missing 'author' field (recommended)")
				}
			}
		}
	}

	// 4. Check for secrets (basic scan).
	secretPatterns := []string{".env", ".env.local", ".env.production", "credentials.json"}
	for _, pat := range secretPatterns {
		if _, err := os.Stat(filepath.Join(absDir, pat)); err == nil {
			errors = append(errors, fmt.Sprintf("potential secret file found: %s (must not be included)", pat))
		}
	}

	// 5. Report results.
	if len(warnings) > 0 {
		for _, w := range warnings {
			ui.Warn("  " + w)
		}
	}

	if len(errors) > 0 {
		for _, e := range errors {
			ui.Error("  " + e)
		}
		return fmt.Errorf("validation failed with %d error(s)", len(errors))
	}

	ui.Success("Plugin validation passed!")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Fork https://github.com/nself-org/plugins")
	fmt.Println("  2. Copy your plugin directory into the fork")
	fmt.Println("  3. Open a Pull Request using the plugin submission template")
	fmt.Printf("  4. PR link: https://github.com/nself-org/plugins/compare/main...your-branch\n")

	return nil
}
