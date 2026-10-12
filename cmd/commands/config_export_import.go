package commands

// Purpose: RunE implementations for "nself config export" and "nself config
// import". Inputs are the cobra command/args; outputs are an exported file or
// an imported/updated env file, or an error.
// Constraints: split out of config.go (CLI-R12) as a pure move, no behavior change.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

func runConfigExport(cmd *cobra.Command, args []string) error {
	format, _ := cmd.Flags().GetString("format")
	outputFile, _ := cmd.Flags().GetString("output")
	if outputFile == "" && len(args) > 0 {
		outputFile = args[0]
	}
	reveal, _ := cmd.Flags().GetBool("reveal")
	envFlag, _ := cmd.Flags().GetString("env")

	projectDir, err := resolveProjectDir()
	if err != nil {
		return err
	}

	envFile := envFileName(projectDir, envFlag)
	pairs, err := config.ReadEnvFile(envFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("env file not found: %s", envFile)
		}
		return config.EnvReadError(envFile, err)
	}

	keys := make([]string, 0, len(pairs))
	for k := range pairs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder

	switch format {
	case "json":
		m := make(map[string]string, len(pairs))
		for _, k := range keys {
			m[k] = maskValue(k, pairs[k], reveal)
		}
		data, jsonErr := json.MarshalIndent(m, "", "  ")
		if jsonErr != nil {
			return fmt.Errorf("encoding json: %w", jsonErr)
		}
		sb.Write(data)
		sb.WriteString("\n")
	case "yaml":
		for _, k := range keys {
			v := maskValue(k, pairs[k], reveal)
			if strings.ContainsAny(v, ": \t#\"'\\") || v == "" {
				fmt.Fprintf(&sb, "%s: %q\n", k, v)
			} else {
				fmt.Fprintf(&sb, "%s: %s\n", k, v)
			}
		}
	default: // env
		sb.WriteString("# Exported by nself config export\n")
		for _, k := range keys {
			sb.WriteString(k + "=" + maskValue(k, pairs[k], reveal) + "\n")
		}
	}

	output := sb.String()

	// --json (or --format json) answers with the v1 envelope (the map; bare with NSELF_JSON_LEGACY=1)
	asEnvelope := jsonEnvelopeOn(cmd)
	if outputFile != "" {
		if writeErr := os.WriteFile(outputFile, []byte(output), 0600); writeErr != nil {
			return fmt.Errorf("writing export file %s: %w", outputFile, writeErr)
		}
		ui.Success(fmt.Sprintf("Config exported to %s (%d keys)", outputFile, len(keys)))
	} else if !asEnvelope {
		fmt.Print(output)
	}
	if asEnvelope {
		return emitEnv(cmd, maskedPairs(pairs, keys, reveal), format == "json")
	}

	return nil
}

// --- config import ---

func runConfigImport(cmd *cobra.Command, args []string) error {
	srcFile := args[0]
	envFlag, _ := cmd.Flags().GetString("env")
	force, _ := cmd.Flags().GetBool("force")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	projectDir, err := resolveProjectDir()
	if err != nil {
		return err
	}

	// Read source file.
	incoming, err := config.ReadEnvFile(srcFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("import file not found: %s", srcFile)
		}
		return config.EnvReadError(srcFile, err)
	}

	envFile := envFileName(projectDir, envFlag)

	// Read current env file (may not exist yet).
	current := make(map[string]string)
	if _, statErr := os.Stat(envFile); statErr == nil {
		current, err = config.ReadEnvFile(envFile)
		if err != nil {
			return config.EnvReadError(envFile, err)
		}
	}

	if dryRun {
		// Sort keys for deterministic output.
		incomingKeys := make([]string, 0, len(incoming))
		for k := range incoming {
			incomingKeys = append(incomingKeys, k)
		}
		sort.Strings(incomingKeys)

		for _, k := range incomingKeys {
			newVal := incoming[k]
			if existing, ok := current[k]; ok {
				if existing != newVal {
					fmt.Printf("update %s: %s -> %s\n", k, maskValue(k, existing, false), maskValue(k, newVal, false))
				}
			} else {
				fmt.Printf("add %s=%s\n", k, maskValue(k, newVal, false))
			}
		}
		fmt.Println("(dry run - no changes written)")
		return nil
	}

	// Find keys that would be overwritten with a different value.
	var conflicts []string
	for k, newVal := range incoming {
		if existing, ok := current[k]; ok && existing != newVal {
			conflicts = append(conflicts, k)
		}
	}
	sort.Strings(conflicts)

	if len(conflicts) > 0 && !force {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "The following keys in %s will be overwritten:\n", filepath.Base(envFile))
		for _, k := range conflicts {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  %s: %s -> %s\n", k, maskValue(k, current[k], false), maskValue(k, incoming[k], false))
		}
		_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Continue? [y/N] ")
		var response string
		if _, scanErr := fmt.Scanln(&response); scanErr != nil || strings.ToLower(strings.TrimSpace(response)) != "y" {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Import cancelled.")
			if jsonEnvelopeOn(cmd) {
				setData(cmd, dataResult{OK: false, Target: srcFile}) // nothing was written
			}
			return nil
		}
	}

	// Apply all incoming keys into the target file.
	newCount := 0
	updateCount := 0
	for k, v := range incoming {
		updated, setErr := setEnvFileLine(envFile, k, v)
		if setErr != nil {
			return setErr
		}
		if updated {
			updateCount++
		} else {
			newCount++
		}
	}

	ui.Success(fmt.Sprintf("Import complete: %d updated, %d added to %s", updateCount, newCount, filepath.Base(envFile)))
	return nil
}
