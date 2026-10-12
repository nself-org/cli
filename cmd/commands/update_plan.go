package commands

// Purpose: `nself update --plan` and `nself update --yes` (cap:cli.update-plan,
// P7-LIVE-10). --plan prints the pin diff of nself.lock.yaml against the CLI
// image lock, the plugin registry and the latest CLI release, plus the ordered
// step list, and writes nothing. --yes applies the image and plugin pins
// through the existing paths (the lock rewrite, `plugin update`), regenerates
// the project with the build pipeline, and replaces the CLI binary last.
// Inputs: the project's nself.lock.yaml, the release lookup (GitHub), the
// plugin registry, and the flags --plan, --yes, --cli, --version, --json.
// Outputs: a table and step list, or the v1 envelope with UpdateResult.
// Constraints: a source that cannot be reached (including HTTP 429) is a coded
// error, never an empty plan; a pin the source no longer offers is a row with
// resolvable=false. The sources are reached through updateDeps so tests run on
// fixtures. The steps that need a running stack belong to `update stack`.

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/lock"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/upgrade/plan"
	"github.com/spf13/cobra"
)

// addUpdatePlanFlags registers --plan and --yes on c.
func addUpdatePlanFlags(c *cobra.Command) {
	c.Flags().Bool("plan", false, "Show the pin diff (CLI, images, plugins) and the upgrade steps; writes nothing")
	c.Flags().Bool("yes", false, "Apply the image and plugin pins, rebuild, then update the CLI last (--cli=false skips the CLI)")
}

// runUpdatePlanMode handles --plan, --yes and --json. handled is false for a
// plain `nself update`, which keeps its previous behaviour.
func runUpdatePlanMode(cmd *cobra.Command) (handled bool, err error) {
	doPlan, _ := cmd.Flags().GetBool("plan")
	yes, _ := cmd.Flags().GetBool("yes")
	jsonOn, jerr := jsonModeOf(cmd)
	if jerr != nil {
		return true, jerr
	}
	switch {
	case doPlan && yes:
		return true, errs.New("E401", "--plan and --yes cannot be combined: --plan shows the changes, --yes applies them")
	case !doPlan && !yes:
		if jsonOn {
			return true, errs.New("E402", "nself update --json needs --plan or --yes")
		}
		return false, nil
	}
	if admin, _ := cmd.Flags().GetBool("admin"); admin {
		return true, errs.New("E401", "--admin cannot be combined with --plan or --yes: the admin image is an image pin")
	}
	if doPlan {
		return true, runUpdatePlan(cmd, newUpdateDeps())
	}
	return true, runUpdateYes(cmd, newUpdateDeps())
}

// httpStatusRE pulls the status out of fetchReleaseFromURL's error text.
var httpStatusRE = regexp.MustCompile(`status (\d{3})`)

// sourceFailure codes a source that could not answer. 429 gets its own words.
func sourceFailure(source string, err error) error {
	why := "The source did not answer: " + err.Error()
	if m := httpStatusRE.FindStringSubmatch(err.Error()); m != nil && m[1] == "429" {
		why = "The source rate-limited this request (HTTP 429)."
	}
	return errs.Wrap("E400", "update plan: "+source+" is unavailable", err).
		WithWhy(why).WithFix("Retry in a few minutes. Nothing was changed.")
}

// loadProjectLock returns the project dir and its lock.
func loadProjectLock() (string, lock.Lock, error) {
	dir, err := resolveProjectDir()
	if err != nil {
		return "", lock.Lock{}, err
	}
	l, err := lock.Load(dir)
	if err != nil {
		return "", lock.Lock{}, errs.Wrap("E455", "update needs nself.lock.yaml", err).
			WithFix("Run nself build --write-lock, commit the file, then retry.")
	}
	return dir, *l, nil
}

// humanWriter is where text goes: stdout, or stderr when stdout carries JSON.
func humanWriter(cmd *cobra.Command, jsonOn bool) io.Writer {
	if jsonOn {
		return cmd.ErrOrStderr()
	}
	return cmd.OutOrStdout()
}

func pendingSteps(p plan.Plan) []UpdateStep {
	out := make([]UpdateStep, 0, len(p.Steps))
	for _, s := range p.Steps {
		out = append(out, UpdateStep{Name: s.Name, Status: "pending"})
	}
	return out
}

// runUpdatePlan is `nself update --plan`: resolve, print, write nothing.
func runUpdatePlan(cmd *cobra.Command, d updateDeps) error {
	jsonOn, _ := jsonModeOf(cmd)
	_, l, err := loadProjectLock()
	if err != nil {
		return err
	}
	target, _ := cmd.Flags().GetString("version")
	p, err := gatherPlan(cmd.Context(), d, l, normalizeTarget(target))
	if err != nil {
		return err
	}
	if jsonOn {
		return output.EmitData(pilotWriter(), "update", UpdateResult{Plan: p, Applied: false, Steps: pendingSteps(p)})
	}
	return plan.Render(cmd.OutOrStdout(), p)
}

func normalizeTarget(v string) string {
	if v != "" && !strings.HasPrefix(v, "v") {
		return "v" + v
	}
	return v
}

// pinsOf returns the resolvable pins of one kind.
func pinsOf(p plan.Plan, kind string) []plan.Pin {
	var out []plan.Pin
	for _, pin := range p.Pins {
		if pin.Kind == kind && pin.Resolvable {
			out = append(out, pin)
		}
	}
	return out
}

// writeLockFile replaces nself.lock.yaml atomically (temp file, then rename).
func writeLockFile(dir string, l lock.Lock) error {
	data, err := l.Marshal()
	if err != nil {
		return err
	}
	path := lock.Path(dir)
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func progress(w io.Writer, format string, a ...any) { _, _ = fmt.Fprintf(w, format+"\n", a...) }

// finishSteps fills every step the apply did not report: the steps that need a
// running stack belong to `update stack`.
func finishSteps(steps []UpdateStep, done map[string]UpdateStep) []UpdateStep {
	for i := range steps {
		if s, ok := done[steps[i].Name]; ok {
			steps[i] = s
		} else {
			steps[i] = UpdateStep{Name: steps[i].Name, Status: "skipped", Detail: "needs a running stack: run by nself update stack"}
		}
	}
	return steps
}

// runUpdateYes is `nself update --yes`: images, plugins, build, then the CLI.
func runUpdateYes(cmd *cobra.Command, d updateDeps) error {
	jsonOn, _ := jsonModeOf(cmd)
	w := humanWriter(cmd, jsonOn)
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	dir, l, err := loadProjectLock()
	if err != nil {
		return err
	}
	target, _ := cmd.Flags().GetString("version")
	p, err := gatherPlan(ctx, d, l, normalizeTarget(target))
	if err != nil {
		return err
	}
	onlyCLI, _ := cmd.Flags().GetBool("cli")
	skipCLI := cmd.Flags().Changed("cli") && !onlyCLI
	for _, pin := range p.Pins {
		if !pin.Resolvable {
			progress(w, "Skipping %s %s: %s", pin.Kind, pin.Name, pin.Reason)
		}
	}
	st := map[string]UpdateStep{}
	if onlyCLI {
		for _, n := range []string{plan.StepImages, plan.StepPlugins, plan.StepBuild} {
			st[n] = UpdateStep{Name: n, Status: "skipped", Detail: "--cli updates only the CLI"}
		}
	} else if err := applyLocalSteps(ctx, w, d, dir, l, p, st); err != nil {
		return err
	}
	st[plan.StepCLI] = UpdateStep{Name: plan.StepCLI, Status: "skipped", Detail: "no CLI update on offer"}
	if cli := pinsOf(p, plan.KindCLI); skipCLI {
		st[plan.StepCLI] = UpdateStep{Name: plan.StepCLI, Status: "skipped", Detail: "--cli=false"}
	} else if len(cli) > 0 {
		progress(w, "Updating CLI: %s -> %s", cli[0].From, cli[0].To)
		if err := d.selfUpdate(cli[0].To); err != nil {
			return fmt.Errorf("CLI update failed (pins and the build are already applied): %w", err)
		}
		st[plan.StepCLI] = UpdateStep{Name: plan.StepCLI, Status: "done", Detail: cli[0].From + " -> " + cli[0].To}
	}
	res := UpdateResult{Plan: p, Applied: true, Steps: finishSteps(pendingSteps(p), st)}
	if jsonOn {
		return output.EmitData(pilotWriter(), "update", res)
	}
	progress(w, "Update applied. Run nself update stack to restart the stack on the new pins.")
	return nil
}

// applyLocalSteps runs images, plugins and build, in that order.
func applyLocalSteps(ctx context.Context, w io.Writer, d updateDeps, dir string, l lock.Lock, p plan.Plan, st map[string]UpdateStep) (err error) {
	changed := false
	skip := func(n, why string) { st[n] = UpdateStep{Name: n, Status: "skipped", Detail: why} }
	orig, rerr := os.ReadFile(lock.Path(dir))
	if rerr != nil {
		return rerr
	}
	// Until the build has rewritten the lock, any failure puts the old pins back,
	// so a retry sees the same diff instead of a half-applied "nothing to do".
	defer func() {
		if err != nil {
			err = restoreLock(dir, orig, err)
		}
	}()
	if imgs := pinsOf(p, plan.KindImage); len(imgs) > 0 {
		if err := d.writeLock(dir, plan.ApplyImagePins(l, imgs)); err != nil {
			return fmt.Errorf("moving image pins in %s: %w", lock.FileName, err)
		}
		st[plan.StepImages] = UpdateStep{Name: plan.StepImages, Status: "done", Detail: fmt.Sprintf("%d pin(s)", len(imgs))}
		changed = true
	} else {
		skip(plan.StepImages, "no image pin changes")
	}
	if plugs := pinsOf(p, plan.KindPlugin); len(plugs) > 0 {
		for _, pin := range plugs {
			progress(w, "Updating plugin %s: %s -> %s", pin.Name, pin.From, pin.To)
			if err := d.updatePlugin(ctx, dir, pin.Name); err != nil {
				return fmt.Errorf("updating plugin %q (plugins updated before it stay installed): %w", pin.Name, err)
			}
		}
		st[plan.StepPlugins] = UpdateStep{Name: plan.StepPlugins, Status: "done", Detail: fmt.Sprintf("%d plugin(s)", len(plugs))}
		changed = true
	} else {
		skip(plan.StepPlugins, "no plugin changes")
	}
	if !changed {
		skip(plan.StepBuild, "nothing to regenerate")
		return nil
	}
	progress(w, "Regenerating the project (nself build)")
	if err := d.build(ctx, dir); err != nil {
		return fmt.Errorf("build after the pin changes failed: %w", err)
	}
	st[plan.StepBuild] = UpdateStep{Name: plan.StepBuild, Status: "done", Detail: lock.FileName + " rewritten"}
	return nil
}
