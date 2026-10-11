package commands

// config_set_plan.go — `nself config set` reconciles (P7-LIVE-05; EPIC P7-LIVE
// D3, D4, D6, D8).
//
// Purpose: one setting change heals the config that depends on it. The plan is
// computed before anything is written: the pending env line reaches the
// planner through reconcile.Request.EnvOverlay, so `--preview` prints the
// plan and writes nothing, not even the env line.
//   - v1.5: plan -> confirm (a prod-class env needs --yes or a terminal yes;
//     non-interactive refuses with E403, exit 4; a hand-edited overwrite needs
//     --force; a change away from a prod-class ENV counts as prod-class) ->
//     write the env line -> apply (the build pipeline; no container is
//     restarted). If the apply fails after the line was written, the env file
//     is restored byte for byte (under the project lock, atomically) and the
//     apply error is returned.
//   - v1.4: write the line as before, then print the plan summary and a
//     notice on stderr; nothing is regenerated (stdout unchanged).
//
// Inputs: the parsed command (key, value, --env file, --preview, --yes,
// --force, --json). Outputs: the env write, the applied plan and its one-line
// summary `config: KEY → <n> artifacts changed (<kinds>)`, or with --json
// (v1.5 mode) the v1 envelope {key, file, applied, plan}.
// Constraints: only cmd/commands calls reconcile (EPIC D6). `secrets rotate`
// is not wired here (EPIC D24); a live secret rotates with
// `config set <KEY> <value> --env secrets`.

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/spf13/cobra"
)

func init() {
	f := configSetCmd.Flags()
	f.Bool("preview", false, "Show what the change would regenerate and write nothing (not even the env line)")
	f.Bool("yes", false, "Confirm the change on a prod-class env (prod, staging) without a prompt")
	f.Bool("force", false, "Allow overwriting hand-edited generated files")
}

// configSetApply is the apply step; tests substitute a failing one to prove
// the env file is restored.
var configSetApply = reconcile.Apply

// configSetRun is one `config set` invocation after validation.
type configSetRun struct {
	cmd        *cobra.Command
	projectDir string
	envFile    string // absolute path of the target env file
	key, value string
	monorepo   bool // the project root was found above cwd
	jsonOut    bool
}

// relFile is the env file relative to the project, slash form. envFileName
// puts it at the root; an --env value holding a path separator does not, and
// the planner refuses such a name rather than plan a different file.
func (r configSetRun) relFile() string {
	rel, err := filepath.Rel(r.projectDir, r.envFile)
	if err != nil {
		return r.envFile
	}
	return filepath.ToSlash(rel)
}

// request is the reconcile request with the pending line as an env overlay.
func (r configSetRun) request() reconcile.Request {
	return reconcile.Request{
		ProjectDir: r.projectDir,
		Command:    reconcile.CmdConfigSet,
		Trigger:    reconcile.Trigger{Kind: reconcile.TriggerConfig, Subject: r.key},
		Containers: r.jsonOut,
		EnvOverlay: map[string]map[string]string{r.relFile(): {r.key: r.value}},
		Stderr:     r.cmd.ErrOrStderr(),
	}
}

func (r configSetRun) run() error {
	if preview, _ := r.cmd.Flags().GetBool("preview"); preview {
		return r.preview()
	}
	// compat.V15(P7-LIVE-05): env line written, regeneration left to the next build -> plan, confirm (prod-class), write the line, regenerate
	if !compat.V15() {
		return r.writeOnly()
	}
	return r.apply()
}

// preview prints the plan of the change and writes nothing.
func (r configSetRun) preview() error {
	req := r.request()
	req.Containers = true
	state, err := reconcile.LoadGeneratedState(r.projectDir)
	if err != nil {
		return err
	}
	req.HandEdited = state.HandEditedFn(r.projectDir)
	p, err := reconcile.Compute(context.Background(), req)
	if err != nil {
		return fmt.Errorf("planning config set %s: %w", r.key, err)
	}
	if r.jsonOut {
		return output.EmitData(pilotWriter(), "config set", configSetData{Key: r.key, File: r.relFile(), Plan: *p})
	}
	out := r.cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "Preview: %s in %s (nothing written)\n", r.key, r.envFile)
	return reconcile.RenderHuman(out, *p)
}

// writeOnly is the v1.4 behaviour: the line is written as before; the plan,
// computed before the write, is summarised on stderr with a notice.
func (r configSetRun) writeOnly() error {
	p, planErr := reconcile.Compute(context.Background(), r.request())
	updated, err := setEnvFileLine(r.envFile, r.key, r.value)
	if err != nil {
		return err
	}
	r.reportWritten(updated)
	stderr := r.cmd.ErrOrStderr()
	if planErr != nil {
		_, _ = fmt.Fprintf(stderr, "nself: could not plan the regeneration %s causes: %v\n", r.key, planErr)
		return nil
	}
	_, _ = fmt.Fprintln(stderr, configSetSummary(r.key, *p))
	if !p.Empty {
		_, _ = fmt.Fprintln(stderr, "nself: not applied in v1.4 mode: run 'nself build' to regenerate (v1.5 applies it here; NSELF_V15=1 shows it now)")
	}
	return nil
}

// apply is the v1.5 behaviour: plan, confirm, write the line, regenerate.
// If anything fails after the line was written, the rollback (run by Apply
// under the project lock) puts the env file back. From the write until Apply
// returns, SIGINT/SIGTERM are held back so an interrupt cannot strand the new
// line next to a half-written tree; a held signal is reported afterwards.
func (r configSetRun) apply() error {
	yes, _ := r.cmd.Flags().GetBool("yes")
	force, _ := r.cmd.Flags().GetBool("force")
	stderr := r.cmd.ErrOrStderr()
	w := &envLineWrite{path: r.envFile}
	defer w.releaseSignals()
	opt := reconcile.ApplyOptions{Yes: yes, Force: force,
		BeforeWrite: func() error { return w.write(r.key, r.value) },
		Rollback: func() error {
			if !w.written { // failed before the file was touched: nothing to restore
				return nil
			}
			if err := w.restore(); err != nil {
				return fmt.Errorf("restoring %s: %w", r.envFile, err)
			}
			_, _ = fmt.Fprintf(stderr, "nself: %s restored; %s is unchanged. Generated files may be partly rewritten: review with 'nself build --plan'.\n", r.envFile, r.key)
			return nil
		}}
	if !r.jsonOut {
		opt.Interactive = interactiveConfirm(r.cmd)
	}
	p, err := configSetApply(context.Background(), r.request(), opt)
	if sig := w.releaseSignals(); sig != nil {
		_, _ = fmt.Fprintf(stderr, "nself: %v received during the apply; it was held until the apply finished\n", sig)
	}
	if err != nil {
		return configSetApplyError(r.key, err)
	}
	if r.jsonOut {
		return output.EmitData(pilotWriter(), "config set", configSetData{Key: r.key, File: r.relFile(), Applied: true, Plan: *p})
	}
	r.reportWritten(w.updated)
	_, _ = fmt.Fprintln(r.cmd.OutOrStdout(), configSetSummary(r.key, *p))
	return nil
}

// configSetApplyError names the command in a refusal; other errors keep their
// code and gain the key as context.
func configSetApplyError(key string, err error) error {
	if d := errs.Describe(err); d != nil && d.Code == "E403" {
		return errs.New("E403", fmt.Sprintf("config set %s needs confirmation; nothing was changed", key)).
			WithWhy(err.Error()).
			WithFix("review the change with --preview, then rerun with --yes (and --force for hand-edited files)")
	}
	return fmt.Errorf("config set %s: %w", key, err)
}

// configSetSummary is the one-line cascade summary of a plan.
func configSetSummary(key string, p reconcile.Plan) string {
	seen := map[string]bool{}
	var kinds []string
	for _, a := range p.Artifacts {
		if k := string(a.Kind); !seen[k] {
			seen[k] = true
			kinds = append(kinds, k)
		}
	}
	sort.Strings(kinds)
	s := fmt.Sprintf("config: %s → %d artifacts changed", key, len(p.Artifacts))
	if len(kinds) > 0 {
		s += " (" + strings.Join(kinds, ", ") + ")"
	}
	return s
}
