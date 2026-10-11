package commands

// Purpose: deliver D5 secret values only to a FIFO consumer.
// Inputs: refs, project and attempt flags; never stdin.
// Outputs: additive v1 envelope in either compatibility mode.
// Constraints: check real stdout before store I/O; audit all refusals once.
// SPORT: P7-TRUST-03, contract:cli.secret-resolve/v1.

import (
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/nself-org/cli/internal/audit"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/secrets"
	"github.com/spf13/cobra"
)

var stdoutIsFIFO = realStdoutIsFIFO
var attemptIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// realStdoutIsFIFO checks the actual output descriptor before secret I/O.
func realStdoutIsFIFO(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeNamedPipe != 0
}

var secretsResolveCmd = newSecretsResolveCmd()

// newSecretsResolveCmd builds the additive resolver leaf.
func newSecretsResolveCmd() *cobra.Command {
	c := &cobra.Command{Use: "resolve <ref>...", Short: "Resolve secret references to a FIFO consumer", RunE: runSecretsResolveCommand}
	c.Flags().String("project", "", "Project directory (default current directory)")
	c.Flags().String("for-attempt", "", "Required attempt identifier")
	c.Flags().Bool("json", false, "Required JSON envelope output")
	return c
}

func init() { secretsCmd.AddCommand(secretsResolveCmd) }

// runSecretsResolveCommand emits value-free error envelopes in both modes.
// The v1.4 registry does not advertise the moved envelope, so the command
// owns its error document as well as its successful value document.
func runSecretsResolveCommand(cmd *cobra.Command, args []string) error {
	err := runSecretsResolve(cmd, args)
	if err == nil {
		return nil
	}
	if writeErr := output.EmitError(output.Default(), "config secrets resolve", err); writeErr != nil {
		return writeErr
	}
	return errs.Exit(errs.ExitCodeFor(err))
}

// runSecretsResolve audits refusals and emits values only to a FIFO.
func runSecretsResolve(cmd *cobra.Command, args []string) error {
	project, _ := cmd.Flags().GetString("project")
	project, absErr := filepath.Abs(project)
	attempt, _ := cmd.Flags().GetString("for-attempt")
	req := secrets.ResolveRequest{ProjectRoot: project, AttemptID: attempt, Refs: args}
	refuse := func(code, message string) error {
		if err := secrets.AuditResolve(audit.Write, req, code); err != nil {
			return err
		}
		return errs.New(code, message)
	}
	if cmd.Flags().Changed("env") {
		return refuse("E540", "Resolve does not accept --env")
	}
	jsonMode, _ := cmd.Flags().GetBool("json")
	if !jsonMode || !attemptIDPattern.MatchString(attempt) {
		return refuse("E401", "Resolve requires --json and a valid --for-attempt identifier")
	}
	if !stdoutIsFIFO(output.Default().Out) {
		return refuse("E542", "Secret resolution requires FIFO stdout")
	}
	if absErr != nil {
		return refuse("E401", "Cannot determine project directory")
	}
	fi, err := os.Stat(project)
	if err != nil || !fi.IsDir() {
		return refuse("E401", "Project must be an existing directory")
	}
	values, err := secrets.Resolve(cmd.Context(), req, audit.Write)
	if err != nil {
		return err
	}
	return output.EmitData(output.Default(), "config secrets resolve", secretResolveData{Schema: "cli.secret-resolve/v1", AttemptID: attempt, Values: values})
}
