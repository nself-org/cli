package commands

// Purpose: The `nself secrets edit`, `rotate`, and `retire` subcommands —
// opening the decrypted store in $EDITOR, rotating a secret's value (with
// optional dual-window overlap), and retiring an old key. Split out of
// secrets.go (CLI-R12); see secrets_core.go for the file-splitting
// rationale shared by every secrets_*.go file in this split.
// Inputs: cobra.Command args/flags per subcommand.
// Outputs: an updated internal/secrets store; printed confirmation.
// Constraints: pure move — no behavior changes.

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/nself-org/cli/internal/secrets"

	"github.com/spf13/cobra"
)

var secretsEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open decrypted secrets in $EDITOR, re-encrypt on save",
	RunE: func(cmd *cobra.Command, args []string) error {
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}

		// Decrypt to temp file.
		keys, entries, err := secrets.List(cwd, secretsEnvFlag)
		if err != nil {
			return err
		}
		var lines []string
		for _, k := range keys {
			lines = append(lines, fmt.Sprintf("%s=%s", k, entries[k].Value))
		}

		tmpFile, err := os.CreateTemp("", "nself-secrets-*.env")
		if err != nil {
			return err
		}
		tmpPath := tmpFile.Name()
		defer func() { _ = os.Remove(tmpPath) }()

		if _, err := tmpFile.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
			return err
		}
		_ = tmpFile.Close()

		// Open editor.
		c := exec.Command(editor, tmpPath)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("editor failed: %w", err)
		}

		// Re-read and save.
		data, err := os.ReadFile(tmpPath)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			if err := secrets.Set(cwd, secretsEnvFlag, parts[0], parts[1]); err != nil {
				return fmt.Errorf("setting %s: %w", parts[0], err)
			}
		}
		fmt.Println("Secrets updated.")
		return nil
	},
}

var secretsRotateCmd = &cobra.Command{
	Use:   "rotate <KEY>",
	Short: "Rotate a secret to a new value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		dualWindow, _ := cmd.Flags().GetBool("dual-window")
		if dualWindow {
			if err := secrets.RotateDualWindow(cwd, secretsEnvFlag, args[0]); err != nil {
				return err
			}
			fmt.Printf("Secret %s rotated with dual-key window. Both _CURRENT and _PREVIOUS are active.\n", args[0])
			return nil
		}
		newValue, err := secrets.Rotate(cwd, secretsEnvFlag, args[0])
		if err != nil {
			return err
		}
		switch {
		case newValue == "":
			fmt.Println("Secret requires manual rotation through the provider.")
			if jsonEnvelopeOn(cmd) {
				setData(cmd, dataResult{OK: false, Target: args[0]}) // nothing was rotated
			}
		case jsonEnvelopeOn(cmd):
			// The new value is never printed in envelope mode: stderr is the
			// machine E422 tail channel (P7-SURF-12).
			fmt.Printf("Secret %s rotated.\n", args[0])
		default:
			fmt.Printf("Secret %s rotated. New value: %s\n", args[0], newValue)
		}
		return nil
	},
}

var secretsRetireCmd = &cobra.Command{
	Use:   "retire <KEY>",
	Short: "Retire the _PREVIOUS variant of a dual-window rotated secret",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		if err := secrets.RetireOldKey(cwd, secretsEnvFlag, args[0]); err != nil {
			return err
		}
		fmt.Printf("Retired %s_PREVIOUS. Only the new key remains.\n", args[0])
		return nil
	},
}
