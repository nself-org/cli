package commands

// start_ports.go — Step 3 (port availability check) for `nself start`.
// Split out of start.go (T-P6-E2-W1-S1-T3) for 300-line compliance.
// Inputs:  ctx, opts, projectDir, composeFiles.
// Outputs: error on a detected port conflict; nil otherwise (warnings for
//          skip/probe-failure cases are printed, not returned as errors,
//          matching the original inline behavior).
// Constraints: pure move, same checks/output/errors, no behavior change.

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/ports"
	"github.com/nself-org/cli/internal/ui"
)

// checkStartPorts validates that the ports this stack will actually bind
// (read from the resolved compose config, not a fixed default list) are
// free. Checking the default list instead would block a second project on
// the same host over ports it was never going to bind — see the inline
// comment on checkPorts below for the ɳTask staging incident this guards
// against.
func checkStartPorts(ctx context.Context, opts startOpts, projectDir string, composeFiles []string) error {
	if opts.skipPortCheck {
		ui.Warn("Port check skipped (--skip-port-check)")
	} else {
		// Check the ports THIS stack will actually bind, read from the resolved
		// compose config, rather than a fixed list of nSelf defaults.
		//
		// A second project on the same host has to move off the defaults
		// (POSTGRES_PORT=5433, HASURA_PORT=8181, ...). Checking the default
		// list then blocks it on 5432 and 8080 — ports it was never going to
		// bind — purely because the first project holds them. That is not a
		// conflict, and it is what kept the ɳTask staging stack down: its
		// compose published 5433/8181/4001/6380/9010/9011, every one free,
		// while start refused over six defaults it does not use.
		// Pass the compose env files. An installed plugin's
		// docker-compose.plugin.yml refers to ${DOCKER_NETWORK}; without them
		// it resolves to empty and docker rejects the whole project, so both
		// queries below fail the moment any plugin is installed.
		envFiles := build.ComposeEnvFiles(projectDir)

		report := projectPortConflicts(ctx, projectDir, envFiles, composeFiles...)

		if report.UsedFallbackPorts() {
			ui.Warn(fmt.Sprintf("Could not read published ports from compose (%v); using the default port list", report.DeclaredErr))
		}

		conflicts := report.Conflicts
		if report.OwnershipUnknown() {
			// We could not establish which ports this project already owns.
			// Reporting conflicts anyway would block a legitimate start over
			// the project's own running containers, which is a worse failure
			// than not checking: a real conflict still surfaces when docker
			// refuses to bind the port.
			ui.Warn(fmt.Sprintf(
				"Could not determine which ports this project already owns (%v); skipping the conflict check", report.OwnershipErr))
		} else if len(conflicts) > 0 {
			var portList []string
			for _, c := range conflicts {
				svc := report.ServiceName(c.Port)
				// Attempt to identify the holder process for a richer message.
				holder, _ := ports.WhoHoldsPort(c.Port)
				var detail string
				if holder != nil {
					detail = fmt.Sprintf("Port %d (%s): %s", c.Port, svc,
						ports.FormatConflictMessage(c.Port, holder))
				} else {
					detail = fmt.Sprintf("Port %d (%s) is already in use", c.Port, svc)
				}
				ui.Error(detail)
				portList = append(portList, fmt.Sprintf("%d", c.Port))
			}

			suggestions := []string{
				fmt.Sprintf("Find what is using these ports: lsof -i :%s", strings.Join(portList, " -i :")),
				"Stop the conflicting services before starting nSelf",
				"Or change the conflicting ports in your .env file (e.g., NGINX_HTTP_PORT, HASURA_PORT, POSTGRES_EXPOSE_PORT)",
				"Use --skip-port-check to start anyway (not recommended)",
			}
			ui.UXError(
				fmt.Sprintf("Port conflicts detected (%d port(s))", len(conflicts)),
				fmt.Sprintf("Ports in use: %s", strings.Join(portList, ", ")),
				suggestions,
			)
			return fmt.Errorf("port conflicts detected: %d port(s) in use", len(conflicts))
		} else {
			ui.Success("All ports available")
		}
	}
	return nil
}
