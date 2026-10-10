package appready

// run.go — the five steps after the ledger: reconcile, start, readiness,
// tracking, health.
//
// Purpose: drive Hooks through the Runner so the first failure stops the run and
// names the step and the plugin.
// Inputs: a Runner, Hooks, the plugin directory and the member names.
// Outputs: Outcome (whether the reconcile hook ran) and the Runner's Report.
// Constraints: readiness waits only for members that declare migrations.apply:
// boot; a member with a migrations directory but no boot declaration is named
// with E120 in the step detail, and the tracking step catches its missing tables.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/plugin/readiness"
)

// Outcome tells the caller what Run did beyond the Report.
type Outcome struct {
	// Reconciled is true once the Reconcile hook ran, success or not.
	Reconciled bool
}

// Run executes reconcile .. health. changed is whether bundle membership
// changed; with no change reconcile is skipped (start and the checks still run).
func Run(ctx context.Context, rn *Runner, h Hooks, pluginDir string, names []string, changed bool) Outcome {
	var out Outcome
	rest := []string{StepStart, StepReadiness, StepTracking, StepHealth}
	if f := rn.Failure(); f != nil {
		for _, s := range append([]string{StepReconcile}, rest...) {
			rn.Skip(s, "not run: step "+f.Step+" failed")
		}
		return out
	}
	switch {
	case h.Reconcile == nil:
		rn.Skip(StepReconcile, "no reconcile hook")
		rn.Unverified("reconcile was not run")
	case !changed:
		rn.Skip(StepReconcile, "no membership change")
	default:
		out.Reconciled = true
		noProject := false
		_ = rn.Step(StepReconcile, func() (string, error) {
			if err := h.Reconcile(ctx); err != nil {
				if errors.Is(err, ErrNoProject) {
					noProject = true
					return "", nil
				}
				return "", err
			}
			return "", nil
		})
		if noProject {
			for _, s := range append([]string{StepReconcile}, rest...) {
				rn.Skip(s, "not in an nSelf project")
			}
			rn.Unverified("not in an nSelf project: no stack was started or checked")
			return out
		}
	}
	rn.step(StepStart, h.Start != nil, "no start hook", func() (string, error) {
		return "", h.Start(ctx)
	})
	var members []Member
	rn.step(StepReadiness, true, "", func() (string, error) {
		var err error
		if members, err = LoadMembers(pluginDir, names); err != nil {
			return "", err
		}
		return waitMembers(ctx, h, members)
	})
	rn.step(StepTracking, h.ExportMetadata != nil, "no Hasura metadata hook", func() (string, error) {
		return waitTracked(ctx, h, members)
	})
	rn.step(StepHealth, true, "", func() (string, error) { return checkHealth(ctx, h, members) })
	return out
}

// step runs fn through the Runner, or skips it (unverified) when its hook is nil.
func (r *Runner) step(name string, have bool, missing string, fn func() (string, error)) {
	if !have {
		r.Skip(name, missing)
		r.Unverified(name + ": " + missing)
		return
	}
	_ = r.Step(name, fn)
}

// waitMembers is the readiness step.
func waitMembers(ctx context.Context, h Hooks, members []Member) (string, error) {
	var waited, unwaited []string
	for _, m := range members {
		if !m.BootMigrations {
			if m.HasMigrationsDir {
				unwaited = append(unwaited, m.Name)
			}
			continue
		}
		if m.Port <= 0 && h.HealthURL == nil {
			return "", &StepError{Plugin: m.Name, Err: fmt.Errorf("plugin %s: plugin.json declares no port, so migrations readiness cannot be probed", m.Name)}
		}
		if err := readiness.Wait(ctx, m.Name, h.healthURL(m), h.timeout(), readiness.WithInterval(h.interval())); err != nil {
			return "", &StepError{Plugin: m.Name, Err: err}
		}
		waited = append(waited, m.Name)
	}
	detail := "no member declares migrations.apply: boot"
	if len(waited) > 0 {
		detail = fmt.Sprintf("migrations applied for %s", strings.Join(waited, ", "))
	}
	if len(unwaited) > 0 {
		detail += fmt.Sprintf("; E120: %s ships migrations/ without migrations.apply: boot", strings.Join(unwaited, ", "))
	}
	return detail, nil
}

// checkHealth is the health step: stack services first, then each member's /health.
func checkHealth(ctx context.Context, h Hooks, members []Member) (string, error) {
	if h.ServiceHealth != nil {
		if err := h.ServiceHealth(ctx); err != nil {
			return "", err
		}
	}
	probed := 0
	for _, m := range members {
		if m.Port <= 0 && h.HealthURL == nil {
			continue
		}
		if err := waitHealthy(ctx, h, m); err != nil {
			return "", &StepError{Plugin: m.Name, Err: err}
		}
		probed++
	}
	return fmt.Sprintf("%d plugins answered /health", probed), nil
}

// waitHealthy polls one member until it answers HTTP 200 or the timeout ends.
// An answered 200 without a migrations field (E119) still counts as healthy:
// health, not migrations, is what this step proves.
func waitHealthy(ctx context.Context, h Hooks, m Member) error {
	url := h.healthURL(m)
	deadline := time.Now().Add(h.timeout())
	for {
		_, err := readiness.Probe(ctx, nil, m.Name, url)
		if err == nil || !readiness.IsUnreachable(err) {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().Add(h.interval()).After(deadline) {
			return fmt.Errorf("plugin %s: /health at %s did not answer 200 within %s: %w", m.Name, url, h.timeout(), err)
		}
		pause(ctx, h.interval())
	}
}

// pause sleeps for d or until ctx ends.
func pause(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
