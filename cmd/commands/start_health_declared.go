package commands

// Purpose: Limits the `nself start` health report to the services the
// generated compose actually declares (D-0502). build.DetectServices sizes
// the list off config alone, so after a proxy swap that drops nginx from the
// compose (the traefik provider) every start reported a nginx container that
// does not exist as not_found and fell short of 100%.
// Inputs: the project workdir (compose manifest + env files) and the
// *health.HealthReport from health.RunAllChecks.
// Outputs: a report with undeclared, unhealthy results removed and its
// counts recomputed.
// Constraints: a service is dropped only when the compose does not declare it
// AND its check failed, so a name mismatch can never hide a healthy container
// and an unreadable compose leaves the report untouched.

import (
	"context"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/health"
)

// composeDeclaredServices returns the service names of the generated compose
// (`docker compose config --services` over the manifest), or nil when the
// manifest or compose cannot be read.
func composeDeclaredServices(ctx context.Context, workdir string) map[string]bool {
	files, err := build.ReadComposeManifest(workdir)
	if err != nil || len(files) == 0 {
		return nil
	}
	comp := docker.NewCompose(files...)
	comp.EnvFiles = build.ComposeEnvFiles(workdir)
	var names []string
	err = config.WithComposeEnvironment(func() error {
		var queryErr error
		names, queryErr = comp.ComposeServiceNames(ctx, workdir)
		return queryErr
	})
	if err != nil || len(names) == 0 {
		return nil
	}
	declared := make(map[string]bool, len(names))
	for _, n := range names {
		declared[n] = true
	}
	return declared
}

// filterDeclaredHealth drops failed results for services the compose does
// not declare and recomputes the counts. A nil declared set (compose
// unreadable) returns the report unchanged.
func filterDeclaredHealth(report *health.HealthReport, declared map[string]bool) *health.HealthReport {
	if report == nil || declared == nil {
		return report
	}
	out := *report
	out.Results = make([]health.HealthResult, 0, len(report.Results))
	out.Healthy, out.Unhealthy = 0, 0
	for _, r := range report.Results {
		ok := r.OK()
		if !ok && !declared[r.Service] {
			continue
		}
		out.Results = append(out.Results, r)
		if ok {
			out.Healthy++
		} else {
			out.Unhealthy++
		}
	}
	out.Total = len(out.Results)
	return &out
}
