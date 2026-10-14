package commands

// Purpose: the v1 envelope for `add` and `remove` (and `config bundles
// install`; P7-SURF-13): what was installed or removed, what was skipped or
// kept, and the next command.
// Inputs: the plugin and bundle install/remove bodies, which record their
// outcome here (recordInstall, recordRemove) from the results they already
// have: bundle.InstallResult / bundle.RemoveResult (the bundle ledger's view)
// and the plugin install loop's installed list. Nothing is re-read or decided
// here, so install, entitlement and remove behaviour is unchanged.
// Outputs: one envelope on the real stdout in v1.5 JSON mode, after the body
// succeeded; a failed body emits nothing (the top-level handler prints the
// error envelope). Human output is untouched.
// Constraints: no secrets (names only; a third-party URL install is reported
// by the URL the user typed, never by a derived credential).
// SPORT: P7-SURF-13, contract:cli.json-envelope, contract:cli.bundle-ledger

import (
	"sync"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/spf13/cobra"
)

// addData is the data of `add` and `config bundles install`.
type addData struct {
	// Installed are the plugins installed by this run, in install order.
	Installed []string `json:"installed"`
	// Skipped are the plugins a bundle install left out (missing from the registry, non-strict).
	Skipped []string `json:"skipped"`
	// Next is the command to run next, "" when nothing was installed or this was a dry run.
	Next string `json:"next"`
	// DryRun is true for --dry-run, --preview and --show-graph: nothing was installed.
	DryRun bool `json:"dry_run"`
}

// removeData is the data of `remove` (a plugin, a builtin or a bundle).
type removeData struct {
	// Removed are the plugins removed by this run.
	Removed []string `json:"removed"`
	// Kept are the bundle members that stay (another bundle or an explicit install needs them).
	Kept []string `json:"kept"`
	// Planned are the plugins a --dry-run would remove; empty otherwise.
	Planned []string `json:"planned"`
	// Next is the command to run next, "" when nothing was removed.
	Next string `json:"next"`
	// DryRun is true for --dry-run: nothing was removed.
	DryRun bool `json:"dry_run"`
}

// installOutcome is what a body recorded for its invocation.
type installOutcome struct {
	installed, skipped, removed, kept, planned []string
}

var installOutcomes sync.Map // *cobra.Command -> *installOutcome

func outcomeOf(cmd *cobra.Command) *installOutcome {
	v, _ := installOutcomes.LoadOrStore(cmd, &installOutcome{})
	return v.(*installOutcome)
}

// recordInstall adds plugins an install body installed or skipped.
func recordInstall(cmd *cobra.Command, installed, skipped []string) {
	o := outcomeOf(cmd)
	o.installed = append(o.installed, installed...)
	o.skipped = append(o.skipped, skipped...)
}

// recordBundleInstall records a bundle install result (nil when it never ran).
func recordBundleInstall(cmd *cobra.Command, r *bundle.InstallResult) {
	if r != nil {
		recordInstall(cmd, r.Installed, r.Skipped)
	}
}

// recordRemove adds what a remove body removed, kept or (dry run) planned.
func recordRemove(cmd *cobra.Command, removed, kept, planned []string) {
	o := outcomeOf(cmd)
	o.removed = append(o.removed, removed...)
	o.kept = append(o.kept, kept...)
	o.planned = append(o.planned, planned...)
}

// recordBundleRemove records a bundle remove result (nil when it never ran).
func recordBundleRemove(cmd *cobra.Command, r *bundle.RemoveResult) {
	if r == nil {
		return
	}
	kept := make([]string, 0, len(r.Kept))
	for _, k := range r.Kept {
		kept = append(kept, k.Name)
	}
	recordRemove(cmd, r.Removed, kept, r.Planned)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func flagOn(cmd *cobra.Command, name string) bool {
	on, _ := cmd.Flags().GetBool(name)
	return on
}

// nextAfterInstall names the command to run next: the first installed plugin's
// own command, else `nself start` to bring the new services up.
func nextAfterInstall(installed []string) string {
	if len(installed) == 0 {
		return ""
	}
	for _, name := range installed {
		if c := pluginCommandName(name); c != "" {
			return "nself " + c + " --help"
		}
	}
	return "nself start"
}

func addFacts(cmd *cobra.Command, _ []string) any {
	o := outcomeOf(cmd)
	dry := flagOn(cmd, "dry-run") || flagOn(cmd, "preview") || flagOn(cmd, "show-graph")
	d := addData{Installed: nonNil(o.installed), Skipped: nonNil(o.skipped), DryRun: dry}
	if !dry {
		d.Next = nextAfterInstall(o.installed)
	}
	return d
}

func removeFacts(cmd *cobra.Command, _ []string) any {
	o := outcomeOf(cmd)
	d := removeData{Removed: nonNil(o.removed), Kept: nonNil(o.kept), Planned: nonNil(o.planned), DryRun: flagOn(cmd, "dry-run")}
	if len(o.removed) > 0 {
		d.Next = "nself start"
	}
	return d
}

// outcomeBody runs an install/remove body and, in v1.5 JSON mode, answers with
// the facts it recorded. The slot is cleared before and after every run.
func outcomeBody(run func(*cobra.Command, []string) error, facts func(*cobra.Command, []string) any) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		installOutcomes.Delete(cmd)
		defer installOutcomes.Delete(cmd)
		if err := run(cmd, args); err != nil {
			return err
		}
		if !jsonEnvelopeOn(cmd) {
			return nil
		}
		return emitEnv(cmd, facts(cmd, args), false)
	}
}

func init() {
	installCmd.RunE = outcomeBody(installCmd.RunE, addFacts)
	removeCmd.RunE = outcomeBody(removeCmd.RunE, removeFacts)
}
