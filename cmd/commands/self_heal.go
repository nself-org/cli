package commands

import (
	"fmt"
	"os"
	"path/filepath"
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
	result, err := rotateJWTToFile(toFile)
	if err != nil {
		return fmt.Errorf("self-heal --jwt: %w", err)
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

	result, err := rotateJWTToFile(toFile)
	if err != nil {
		return err
	}

	// Deliver the new key: file (already written), stdout, or suppressed.
	if toFile != "" {
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
	// --to-file and --no-print promise the key stays off stdout, so the
	// instruction names where the key is instead of repeating it.
	shown := result.NewKey
	if toFile != "" {
		shown = "<the key in " + toFile + ">"
	} else if noPrint {
		shown = "<the new key>"
	}
	fmt.Printf("  1. Add the new key to .env.secrets:\n")
	fmt.Printf("       HASURA_GRAPHQL_JWT_SECRET={\"type\":\"HS256\",\"key\":\"%s\"}\n", shown)
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

// rotateJWTToFile is the rotate-and-deliver step shared by the human and the
// --json forms of --jwt. It reads the current key from the environment
// (HASURA_GRAPHQL_JWT_SECRET; RotateJWTKey stores it as OldKey, and the
// operator builds the JSON envelope for the new key), rotates it, and when
// toFile is set writes the new key there through writeKeyFile. The target is
// opened before the rotation, so a bad path fails with E401 and leaves no
// rotation-log entry for a key nobody received.
func rotateJWTToFile(toFile string) (*auth.RotateResult, error) {
	currentKey := os.Getenv("HASURA_GRAPHQL_JWT_SECRET")
	if currentKey == "" {
		return nil, fmt.Errorf("HASURA_GRAPHQL_JWT_SECRET is not set — cannot determine old key for rotation log. Set the variable and retry")
	}
	var kf *keyFile
	if toFile != "" {
		var err error
		if kf, err = openKeyFile(toFile); err != nil {
			return nil, errs.New("E401", fmt.Sprintf("--to-file %s cannot be written: %v", toFile, err)).
				WithFix("pass --to-file a path in an existing, writable directory")
		}
		defer kf.discard()
	}
	result, err := auth.RotateJWTKey(currentKey)
	if err != nil {
		return nil, err
	}
	if kf != nil {
		if err := kf.commit(result.NewKey + "\n"); err != nil {
			return nil, fmt.Errorf("write key to file %s: %w (the rotation was logged but the key was not delivered; run the rotation again)", toFile, err)
		}
	}
	return result, nil
}

// keyFile is a private (0600) temp file in the directory of a --to-file
// target. commit renames it over the target, so the key file ends 0600 even
// when the target already existed with a wider mode. A symlink target is
// replaced by a regular file; the file it pointed to is left untouched.
type keyFile struct {
	tmp    *os.File
	target string
	done   bool
}

// openKeyFile creates the temp file next to target; it fails when the
// directory is missing or not writable, or when target is a directory.
func openKeyFile(target string) (*keyFile, error) {
	if fi, err := os.Stat(target); err == nil && fi.IsDir() {
		return nil, fmt.Errorf("%s is a directory", target)
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return nil, err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, err
	}
	return &keyFile{tmp: tmp, target: target}, nil
}

// commit writes data, syncs, and renames the temp file over the target.
func (k *keyFile) commit(data string) error {
	if _, err := k.tmp.WriteString(data); err != nil {
		return err
	}
	if err := k.tmp.Sync(); err != nil {
		return err
	}
	if err := k.tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(k.tmp.Name(), k.target); err != nil {
		return err
	}
	k.done = true
	return nil
}

// discard removes the temp file unless commit renamed it into place.
func (k *keyFile) discard() {
	if k.done {
		return
	}
	_ = k.tmp.Close()
	_ = os.Remove(k.tmp.Name())
}
