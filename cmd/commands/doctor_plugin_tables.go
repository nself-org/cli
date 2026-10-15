package commands

// Purpose: diagnose and repair deferred Hasura tracking after offline installs.
// Inputs: project directory, selected environment, context and repair flags.
// Outputs: per-plugin findings, or a confirmation refusal returned to the caller.
// Constraints: v1.5 only; callers complete doctor's reconcile guard before fix.
// Repairs confirm metadata changes, wait for boot migrations and verify metadata.

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/doctor"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin/readiness"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/spf13/cobra"
)

func doctorPluginTableDeepChecks(ctx context.Context, cmd *cobra.Command, cwd, selected, only string, fix bool, results []doctor.CheckResult) ([]doctor.CheckResult, error) {
	// compat.V15(D-0437): doctor only repairs generated files -> diagnoses and repairs deferred boot-plugin table tracking
	if !compat.V15() || only != "" && only != "plugins" {
		return results, nil
	}
	if fix {
		if _, err := runDriftFix(ctx, cmd, cwd); err != nil {
			return nil, err
		}
	}
	tables, err := checkPluginTables(ctx, cmd, cwd, selected, fix, false)
	if err != nil {
		return nil, err
	}
	for _, r := range tables {
		results = append(results, doctor.CheckResult{Section: "plugins", Name: r.Name, Status: r.Status, Message: r.Message, FixCmd: r.Detail})
	}
	return results, nil
}

func checkPluginTables(ctx context.Context, cmd *cobra.Command, cwd, selected string, fix, verbose bool) ([]doctorCheckResult, error) {
	// compat.V15(D-0437): doctor only repairs generated files -> diagnoses and repairs deferred boot-plugin table tracking
	if !compat.V15() || doctorProjectRoot(cwd) == "" {
		return nil, nil
	}
	if selected == "" {
		selected, _ = config.ResolveEnv(cwd)
	}
	boot, err := doctorBootMembers()
	if err != nil {
		return pluginTableProblem(err, verbose), nil
	}
	if len(boot) == 0 {
		return nil, nil
	}
	cfg, err := loadDoctorPluginConfig(doctorProjectRoot(cwd), selected)
	if err != nil {
		return pluginTableProblem(err, verbose), nil
	}
	set, err := doctorPluginTableSet(ctx, cmd, cfg, boot, selected, fix)
	if err != nil {
		if detail := errs.Describe(err); detail != nil && detail.Code == "E403" {
			return nil, err
		}
		return pluginTableProblem(err, verbose), nil
	}
	var out []doctorCheckResult
	for _, m := range boot {
		r := doctorCheckResult{Name: "Plugin tables: " + m.Name, Status: "pass", Message: "all manifest tables tracked in Hasura"}
		if _, missing := appready.Missing([]appready.Member{m}, set); len(missing) > 0 {
			r.Status = "fail"
			r.Message = "untracked Hasura tables: " + strings.Join(missing, ", ")
			r.Detail = "After nself start, run nself doctor --fix to track these tables."
		}
		printCheck(r.Status, r.Name, r.Message, verbose)
		out = append(out, r)
	}
	return out, nil
}

func loadDoctorPluginConfig(root, selected string) (*config.Config, error) {
	previous, present := os.LookupEnv("ENV")
	if err := os.Setenv("ENV", selected); err != nil {
		return nil, err
	}
	defer func() {
		if present {
			_ = os.Setenv("ENV", previous)
		} else {
			_ = os.Unsetenv("ENV")
		}
	}()
	return bundleLoadConfig(root)
}

func doctorBootMembers() ([]appready.Member, error) {
	var names []string
	for name := range stagePluginNames(resolvePluginDir()) {
		if name != "bin" { // command binaries, not a plugin manifest directory
			names = append(names, name)
		}
	}
	sort.Strings(names)
	members, err := appready.LoadMembers(resolvePluginDir(), names)
	if err != nil {
		return nil, err
	}
	boot, _ := trackableMembers(members)
	return boot, nil
}

func doctorPluginTableSet(ctx context.Context, cmd *cobra.Command, cfg *config.Config, boot []appready.Member, selected string, fix bool) (map[string]bool, error) {
	set, err := readDoctorPluginTables(ctx, cfg)
	if err != nil || !fix {
		return set, err
	}
	var pending []appready.Member
	for _, m := range boot {
		if name, _ := appready.Missing([]appready.Member{m}, set); name != "" {
			pending = append(pending, m)
		}
	}
	if len(pending) == 0 {
		return set, nil
	}
	if err := confirmPluginTableRepair(cmd, cfg, selected); err != nil {
		return nil, err
	}
	if err := repairPluginTables(ctx, cfg, pending); err != nil {
		return nil, err
	}
	return readDoctorPluginTables(ctx, cfg)
}

func readDoctorPluginTables(ctx context.Context, cfg *config.Config) (map[string]bool, error) {
	raw, err := bundleExportMetadata(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("cannot inspect Hasura plugin tables (start the stack first): %w", err)
	}
	return appready.TrackedTables(raw)
}

// A clean file plan does not authorize a separate metadata change. Retain both
// the process-first selector and a prod-class value loaded from cascade files.
func confirmPluginTableRepair(cmd *cobra.Command, cfg *config.Config, selected string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	if reconcile.EnvClass(cfg.Env) == reconcile.ClassProd {
		selected = cfg.Env
	}
	p := reconcile.Plan{Command: reconcile.CmdDoctorFix, Env: selected, EnvClass: reconcile.EnvClass(selected)}
	p.RequiresConfirmation = p.EnvClass == reconcile.ClassProd
	return reconcile.Confirm(p, reconcile.ApplyOptions{Yes: yes, Interactive: interactiveConfirm(cmd)}, true, cmd.ErrOrStderr())
}

func repairPluginTables(ctx context.Context, cfg *config.Config, boot []appready.Member) error {
	timeout, err := readiness.TimeoutFromEnv()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	for _, m := range boot {
		if m.Port <= 0 {
			return fmt.Errorf("plugin %s: no health port; cannot verify boot migrations before tracking", m.Name)
		}
		if err := pluginWait(ctx, m.Name, appready.DefaultHealthURL(m), max(time.Until(deadline), time.Millisecond), readiness.WithInterval(pluginReadyEvery)); err != nil {
			return err
		}
	}
	plugins := pluginTablesOf(boot)
	if _, err := bundleTrackTables(ctx, cfg, plugins); err != nil {
		return trackIssue(plugins, err)
	}
	return nil
}

func pluginTableProblem(err error, verbose bool) []doctorCheckResult {
	r := doctorCheckResult{Name: "Plugin tables", Status: "fail", Message: err.Error(), Detail: "Start the stack, then run nself doctor --fix (with --yes on prod/staging)."}
	printCheck(r.Status, r.Name, r.Message, verbose)
	return []doctorCheckResult{r}
}
