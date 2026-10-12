package commands

// Purpose: `nself config secrets decrypt-on-deploy` (top-level `secrets
// decrypt-on-deploy` in v1.4), the deploy hook helper that prints every
// decrypted secret as KEY=VALUE. Split out of secrets_audit.go for debt
// D-0216: the plaintext now needs a terminal stdout or an explicit --reveal,
// and --json carries the key names only.
// Inputs: --reveal, the global --env and --json flags, the project's age store.
// Outputs: KEY=VALUE lines on stdout (text mode), or a v1 envelope whose
// values are all "[REDACTED]" (--json, v1.5).
// Constraints: v1.4 prints as before. In v1.5 a text run whose stdout is not a
// terminal and has no --reveal is refused (E401) before anything is decrypted.
// The decrypted values never enter an envelope, and --json never decrypts to
// text: the key names come from secrets.DeployKeys (the store's key set), because
// parsing the KEY=VALUE text would turn a line of a multi-line value (a PEM
// body) into a key. The canon row marks --reveal
// cli_only with json none, so --reveal --json is refused E402 in both modes.
// SPORT: CLI-CMD-SECRETS-001

import (
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/secrets"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// secretsRedacted replaces every value in the decrypt-on-deploy envelope.
const secretsRedacted = "[REDACTED]"

// decryptStdoutIsTerminal reports whether stdout is a terminal. A variable so
// a test can stand in for one.
var decryptStdoutIsTerminal = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }

var secretsDecryptOnDeployCmd = &cobra.Command{
	Use:   "decrypt-on-deploy",
	Short: "Output decrypted secrets as KEY=VALUE for CI/CD",
	Long: `Print every decrypted secret as KEY=VALUE.

In v1.5 the plaintext goes to a terminal, or anywhere else only with --reveal
(for CI: nself config secrets decrypt-on-deploy --reveal > .env). With --json
the envelope lists the key names and shows every value as [REDACTED].`,
	RunE: func(cmd *cobra.Command, args []string) error {
		reveal, _ := cmd.Flags().GetBool("reveal")
		// compat.V15(D-0216): decrypt-on-deploy prints KEY=VALUE to any stdout and puts values in --json -> plaintext needs a terminal stdout or --reveal; --json carries names only
		v15 := compat.V15()
		envelope := jsonEnvelopeOn(cmd)
		if v15 && !envelope && !reveal && !decryptStdoutIsTerminal() {
			return errs.New("E401", "decrypt-on-deploy needs --reveal when stdout is not a terminal").
				WithWhy("decrypt-on-deploy prints every secret in plaintext and stdout is not a terminal").
				WithFix("pass --reveal to print the values (for CI: nself config secrets decrypt-on-deploy --reveal > .env)")
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		if envelope {
			keys, err := secrets.DeployKeys(cwd, secretsEnvFlag)
			if err != nil {
				return err
			}
			d := secretsDecryptData{Environment: secretsEnvFlag, Values: map[string]string{}}
			for _, k := range keys {
				d.Values[k] = secretsRedacted
			}
			setData(cmd, d)
			return nil
		}
		output, err := secrets.DecryptForDeploy(cwd, secretsEnvFlag)
		if err != nil {
			return err
		}
		fmt.Print(output)
		return nil
	},
}

func init() {
	secretsDecryptOnDeployCmd.Flags().Bool("reveal", false, "Print the decrypted values; needed when stdout is not a terminal (v1.5)")
}
