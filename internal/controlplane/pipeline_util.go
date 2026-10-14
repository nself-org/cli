package controlplane

// Purpose: small helpers of the deploy pipeline (pipeline.go): grouping the
//          resolved servers by role, and the skip bookkeeping.

import (
	"fmt"
	"os"
)

// serverPair bundles a Server with its resolved TargetStatus.
type serverPair struct {
	srv Server
	ts  TargetStatus
}

// filterEnv returns the TargetStatuses that belong to envName.
func filterEnv(statuses []TargetStatus, envName string) []TargetStatus {
	var out []TargetStatus
	for _, ts := range statuses {
		if ts.Env == envName {
			out = append(out, ts)
		}
	}
	return out
}

// serversByRole returns servers of the given role with their TargetStatus,
// excluding CapHidden servers.
func serversByRole(env Environment, statuses []TargetStatus, role ServerRole) []serverPair {
	statusMap := make(map[string]TargetStatus, len(statuses))
	for _, ts := range statuses {
		statusMap[ts.Server] = ts
	}
	var out []serverPair
	for _, srv := range env.Servers {
		if srv.Role != role {
			continue
		}
		ts, ok := statusMap[srv.Name]
		if !ok || ts.Capability == CapHidden {
			continue
		}
		out = append(out, serverPair{srv: srv, ts: ts})
	}
	return out
}

// checkPrimarySkipped sets result.PrimarySkipped if any primary server was skipped.
func checkPrimarySkipped(result *DeployResult, srs []ServerResult) {
	for _, sr := range srs {
		if sr.Primary && sr.Status == "skipped" {
			result.PrimarySkipped = true
		}
	}
}

// logSkipped writes a SKIPPED line to stderr for a server that was not deployed.
func logSkipped(envName, serverName string) {
	_, _ = fmt.Fprintf(os.Stderr, "SKIPPED %s/%s (read-only capability)\n", envName, serverName)
}
