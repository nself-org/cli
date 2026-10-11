package commands

// Purpose: per-file age recipient management for D5 node grants.
// Inputs: file selector, public recipient and explicit release consent.
// Outputs: public recipients or a value-free JSON document.
// Constraints: operator identity never copied; mutation rows are CLI-only.
// SPORT: P7-TRUST-03.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/audit"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/secrets"
	"github.com/spf13/cobra"
)

var secretsRecipientsCmd = newSecretsRecipientsCmd()

// newSecretsRecipientsCmd builds the hub and per-file recipient actions.
func newSecretsRecipientsCmd() *cobra.Command {
	hub := &cobra.Command{Use: "recipients", Short: "Manage age recipients of one secret store", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if jsonEnvelopeOn(cmd) {
			return output.EmitData(output.Default(), invokedKey(cmd), dataHub{Subcommands: []dataHubEntry{{Name: "add"}, {Name: "remove"}, {Name: "list"}}})
		}
		return cmd.Help()
	}}
	hub.PersistentFlags().Bool("json", false, "JSON output in v1.5 mode")
	hub.PersistentFlags().String("project", "", "Project directory (default current directory)")
	for _, action := range []string{"add", "remove", "list"} {
		use := action
		if action != "list" {
			use += " <recipient>"
		}
		c := &cobra.Command{Use: use, Short: action + " public recipients for an encrypted store", RunE: runSecretsRecipients}
		c.Flags().String("file", "", "Store: ci, ci-<env>, release or deploy <env>")
		if action != "list" {
			c.Flags().Bool("allow-release-class", false, "Permit a release or deploy recipient change")
		}
		hub.AddCommand(c)
	}
	return hub
}

func init() { secretsCmd.AddCommand(secretsRecipientsCmd) }

// runSecretsRecipients validates arguments and renders public recipient results.
func runSecretsRecipients(cmd *cobra.Command, args []string) error {
	file, _ := cmd.Flags().GetString("file")
	action := cmd.Name()
	want := 1
	if action == "list" {
		want = 0
	}
	if len(args) != want {
		return errs.New("E401", "Recipient action has the wrong number of arguments")
	}
	project, _ := cmd.Flags().GetString("project")
	project, err := filepath.Abs(project)
	if err != nil {
		return errs.New("E401", "Cannot determine project directory")
	}
	fi, err := os.Stat(project)
	if err != nil || !fi.IsDir() {
		return errs.New("E401", "Project must be an existing directory")
	}
	allow, _ := cmd.Flags().GetBool("allow-release-class")
	req := secrets.RecipientsRequest{ProjectRoot: project, File: file, Action: action, AllowReleaseClass: allow}
	if len(args) == 1 {
		req.Recipient = args[0]
	}
	rows, changed, err := secrets.Recipients(req, audit.Write)
	if err != nil {
		return err
	}
	if jsonEnvelopeOn(cmd) {
		return output.EmitData(output.Default(), invokedKey(cmd), secretRecipientsData{File: file, Recipients: rows, Changed: changed})
	}
	if action == "list" {
		for _, r := range rows {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), r); err != nil {
				return err
			}
		}
	} else if changed {
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "Recipients updated")
	} else {
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "Recipients unchanged")
	}
	return err
}
