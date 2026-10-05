package commands

// Purpose: RunE implementations for "nself config list" and "nself config
// validate". Inputs are the cobra command/args; outputs are printed config
// listings/validation results or an error.
// Constraints: split out of config.go (CLI-R12) as a pure move, no behavior change.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/output"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

func runConfigList(cmd *cobra.Command, args []string) error {
	envFlag, _ := cmd.Flags().GetString("env")

	projectDir, err := resolveProjectDir()
	if err != nil {
		return err
	}

	envFile := envFileName(projectDir, envFlag)

	// Read the env file if it exists. Missing file is not an error for list.
	var pairs map[string]string
	if _, statErr := os.Stat(envFile); statErr == nil {
		pairs, err = godotenv.Read(envFile)
		if err != nil {
			return fmt.Errorf("reading %s: %w", envFile, err)
		}
	} else {
		pairs = make(map[string]string)
	}

	// compat.V15(P7-REG-09): --json ignored -> envelope
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut && compat.V15() {
		return output.EmitData(pilotWriter(), "config list", configListJSON(envFile, pairs))
	}

	// Print header.
	fmt.Printf("%-45s %-30s %s\n", "KEY", "VALUE", "SOURCE")
	fmt.Println(strings.Repeat("-", 90))

	for _, row := range configListRows(pairs) {
		displayVal := row.Value
		switch row.Source {
		case configSourceDefault:
			displayVal = "(default: " + row.Value + ")"
		case configSourceUnset:
			displayVal = "(unset)"
		}
		source := row.Source
		if source == configSourceFile {
			source = filepath.Base(envFile)
		}
		fmt.Printf("%-45s %-30s %s\n", row.Key, displayVal, source)
	}
	return nil
}

// configListRows resolves every known key against the env file: its value
// (masked like the table; never revealed), and its source (file, default or
// unset). The table and `config list --json` both render these rows.
func configListRows(pairs map[string]string) []configListKey {
	known := config.KnownEnvVars()
	rows := make([]configListKey, 0, len(known))
	for _, k := range known {
		val, ok := pairs[k]
		switch {
		case ok && val != "":
			rows = append(rows, configListKey{Key: k, Value: maskValue(k, val, false), Source: configSourceFile})
		case config.DefaultFor(k) != "":
			rows = append(rows, configListKey{Key: k, Value: config.DefaultFor(k), Source: configSourceDefault})
		default:
			rows = append(rows, configListKey{Key: k, Source: configSourceUnset})
		}
	}
	return rows
}

// configListJSON is the data of `config list --json`.
func configListJSON(envFile string, pairs map[string]string) configListData {
	return configListData{File: filepath.Base(envFile), Keys: configListRows(pairs)}
}

// --- S4-T05: config validate ---

func runConfigValidate(cmd *cobra.Command, args []string) error {
	envFlag, _ := cmd.Flags().GetString("env")

	projectDir, err := resolveProjectDir()
	if err != nil {
		return err
	}

	// For validate, use the cascade loader so all defaults apply.
	// Temporarily honour --env by setting ENV if provided.
	if envFlag != "" {
		_ = os.Setenv("ENV", envFlag)
	}

	cfg, err := config.Load(projectDir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Run validators individually to report per-validator pass/fail.
	results := config.RunAllWithResults(cfg)
	errorCount := 0
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "✗ %s: %s\n", r.Name, r.Err.Error())
			errorCount++
		} else {
			fmt.Printf("[PASS] %s\n", r.Name)
		}
	}

	errorCount += reportNselfYAML(cmd.ErrOrStderr(), projectDir)

	if errorCount == 0 {
		fmt.Println("config OK")
		return nil
	}

	fmt.Fprintf(os.Stderr, "\n%d error(s) found\n", errorCount)
	return fmt.Errorf("one or more validators failed")
}

// reportNselfYAML validates nself.yaml / nself.yml when the project has one and
// prints each finding to w (stderr) as "file:line:col: severity: path: [code] ...".
// It returns the number of findings that are errors: none in v1.4 mode (the
// findings are warnings), all of them in v1.5 mode. A project without a
// manifest reports nothing.
func reportNselfYAML(w io.Writer, projectDir string) int {
	path := build.ManifestPath(projectDir)
	if path == "" {
		return 0
	}
	errorCount := 0
	for _, f := range build.ValidateManifestFile(path) {
		_, _ = fmt.Fprintln(w, f.String())
		if f.Severity == build.SeverityError {
			errorCount++
		}
	}
	return errorCount
}

// --- S4-T06: config export ---
