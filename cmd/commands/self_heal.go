package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/nself-org/cli/internal/auth"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

var selfHealCmd = &cobra.Command{
	Use:   "self-heal",
	Short: "Run targeted self-healing routines for nSelf components",
	Long: `Run targeted self-healing routines to repair or initialise nSelf components
without requiring a full stack rebuild.

Each flag selects a specific repair routine. Multiple flags can be combined.

Examples:
  nself self-heal --jwt                     # Rotate JWT key and record in rotation log
  nself self-heal --jwt --to-file /tmp/key  # Write new key to file (0600) instead of stdout
  nself self-heal --jwt --no-print          # Rotate without printing the new key to stdout
  nself self-heal --dry-run                 # Show what would be repaired without making changes`,
	RunE: runSelfHeal,
}

func init() {
	selfHealCmd.Flags().Bool("jwt", false, "Rotate the HASURA_GRAPHQL_JWT_SECRET and update the rotation log")
	selfHealCmd.Flags().Bool("dry-run", false, "Show what would be repaired without making any changes")
	selfHealCmd.Flags().String("to-file", "", "Write the new JWT key to this file path (mode 0600) instead of stdout")
	selfHealCmd.Flags().Bool("no-print", false, "Suppress printing the new JWT key to stdout")
	RootCmd.AddCommand(selfHealCmd)
}

func runSelfHeal(cmd *cobra.Command, args []string) error {
	jwtFlag, _ := cmd.Flags().GetBool("jwt")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	toFile, _ := cmd.Flags().GetString("to-file")
	jsonOn, err := jsonModeOf(cmd)
	if err != nil {
		return err
	}

	if !jwtFlag {
		// No routine selected — show help (stderr while a JSON document is due).
		if err := cmd.Help(); err != nil {
			return err
		}
		if jsonOn {
			return output.EmitData(pilotWriter(), "doctor heal", HealResult{Actions: []HealAction{}})
		}
		return nil
	}
	if jsonOn {
		return runSelfHealJWTJSON(dryRun, toFile)
	}

	ui.CommandHeader("nSelf Self-Heal", "Targeted repair routines")

	if jwtFlag {
		noPrint, _ := cmd.Flags().GetBool("no-print")
		if err := runSelfHealJWT(dryRun, toFile, noPrint); err != nil {
			return fmt.Errorf("self-heal --jwt: %w", err)
		}
	}
	return nil
}

// runSelfHealJWTJSON is the --json form of the --jwt routine (P7-SURF-11): the
// same rotation, one envelope, none of the human text. The new key is never
// placed in the document or on a stream, so a real rotation needs --to-file
// (mode 0600); the request is refused before anything is rotated.
func runSelfHealJWTJSON(dryRun bool, toFile string) error {
	res := HealResult{Actions: []HealAction{}, DryRun: dryRun, LogPath: auth.RotationLogPath()}
	if dryRun {
		res.Actions = append(res.Actions, HealAction{Name: "jwt", Status: "would-rotate",
			Detail: fmt.Sprintf("rotation window %d days", auth.RotationWindowDays())})
		return output.EmitData(pilotWriter(), "doctor heal", res)
	}
	if toFile == "" {
		return errs.New("E401", "doctor heal --jwt --json needs --to-file: the new key is never put in a JSON document").
			WithFix("add --to-file <path> (written with mode 0600)")
	}
	currentKey := os.Getenv("HASURA_GRAPHQL_JWT_SECRET")
	if currentKey == "" {
		return fmt.Errorf("self-heal --jwt: HASURA_GRAPHQL_JWT_SECRET is not set — cannot determine old key for rotation log. Set the variable and retry")
	}
	result, err := auth.RotateJWTKey(currentKey)
	if err != nil {
		return fmt.Errorf("self-heal --jwt: %w", err)
	}
	if err := os.WriteFile(toFile, []byte(result.NewKey+"\n"), 0o600); err != nil {
		return fmt.Errorf("write key to file %s: %w", toFile, err)
	}
	res.Rotated, res.KeyFile = true, toFile
	res.GraceUntil = result.GraceUntil.UTC().Format(time.RFC3339)
	res.Actions = append(res.Actions, HealAction{Name: "jwt", Status: "rotated", Detail: "new key written to --to-file"})
	return output.EmitData(pilotWriter(), "doctor heal", res)
}

// runSelfHealJWT is the entry point for the --jwt routine.
// It generates a new JWT key, writes the rotation log entry, and prints
// instructions for applying the new key.
//
// When toFile is non-empty, the new key is written to that path at mode 0600
// instead of stdout. When noPrint is true, the key is not printed to stdout
// (useful in automated pipelines where the key is captured via --to-file).
//
// It does NOT write to .env.secrets or restart Hasura automatically — those
// steps are destructive and require explicit operator action.
func runSelfHealJWT(dryRun bool, toFile string, noPrint bool) error {
	logPath := auth.RotationLogPath()
	windowDays := auth.RotationWindowDays()

	if dryRun {
		ui.Info("Would rotate HASURA_GRAPHQL_JWT_SECRET")
		ui.Info(fmt.Sprintf("  Rotation log: %s", logPath))
		ui.Info(fmt.Sprintf("  Rotation window: %d days", windowDays))
		ui.Info("No changes made (--dry-run)")
		return nil
	}

	// Read the existing raw key from the environment.
	// HASURA_GRAPHQL_JWT_SECRET holds the JSON envelope: {"type":"HS256","key":"<rawHex>"}
	// We pass the env var value as currentKey; RotateJWTKey stores it as OldKey (raw hex).
	// The operator is responsible for constructing the JSON envelope for the new key.
	currentKey := os.Getenv("HASURA_GRAPHQL_JWT_SECRET")
	if currentKey == "" {
		return fmt.Errorf("HASURA_GRAPHQL_JWT_SECRET is not set — cannot determine old key for rotation log. Set the variable and retry")
	}

	result, err := auth.RotateJWTKey(currentKey)
	if err != nil {
		return err
	}

	// Deliver the new key: file, stdout, or suppressed.
	if toFile != "" {
		if err := os.WriteFile(toFile, []byte(result.NewKey+"\n"), 0o600); err != nil {
			return fmt.Errorf("write key to file %s: %w", toFile, err)
		}
		ui.Success(fmt.Sprintf("New JWT key written to %s (mode 0600)", toFile))
	} else if !noPrint {
		ui.Success(fmt.Sprintf("JWT key rotated. Rotation log updated: %s", logPath))
		fmt.Println()
		fmt.Printf("  New key: %s\n", result.NewKey)
	} else {
		ui.Success(fmt.Sprintf("JWT key rotated. Rotation log updated: %s", logPath))
	}

	fmt.Println()
	fmt.Println("Next steps (manual — apply before grace period expires):")
	fmt.Printf("  1. Add the new key to .env.secrets:\n")
	fmt.Printf("       HASURA_GRAPHQL_JWT_SECRET={\"type\":\"HS256\",\"key\":\"%s\"}\n", result.NewKey)
	fmt.Println()
	fmt.Printf("  2. Restart all services that cache JWT keys:\n")
	fmt.Println("       nself build && nself restart")
	fmt.Println("     Services that hold cached keys (Hasura, auth-server, every plugin) will")
	fmt.Println("     disconnect WebSocket clients with active subscriptions on restart.")
	fmt.Println("     See jwt-rotation.md for the recommended restart sequence.")
	fmt.Println()
	fmt.Printf("  3. During grace period (until %s) keep the old key in\n", result.GraceUntil.Format("2006-01-02 15:04 UTC"))
	fmt.Println("     HASURA_GRAPHQL_JWT_SECRET_KEY_PREVIOUS so running tokens continue to")
	fmt.Println("     validate against Hasura's dual-key support.")
	fmt.Println()
	fmt.Printf("  4. After grace period: remove HASURA_GRAPHQL_JWT_SECRET_KEY_PREVIOUS.\n")
	fmt.Println()
	fmt.Printf("Log entry: %s\n", result.LogEntry)

	return nil
}
