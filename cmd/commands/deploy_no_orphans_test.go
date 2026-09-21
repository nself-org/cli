package commands

// deploy_no_orphans_test.go — regression guard for defect 4 of the
// 2026-09-20 production deploy audit: the rolling-restart paths
// (deploy_strategies.go's runRollingRestart and deploy_remote.go's
// remoteDeployPush) must never pass --remove-orphans to "docker compose up".
//
// Production evidence: /opt/nself-web has 15 running containers that are
// orphans relative to the generated compose (plugin services started by
// `nself plugin` and a separate monitoring stack). A deploy that restarts
// individual services with `docker compose up -d --no-deps <svc>` never
// needs --remove-orphans (that flag only matters for a full `up`/`down`
// pass over the whole project), but the surest guarantee against ever
// silently starting to tear those containers down is a source-level pin
// rather than trusting future contributors not to add it. Mirrors the
// pattern in admin_start_scope_test.go (source-scan for a specific
// dangerous call shape).

import (
	"os"
	"strings"
	"testing"
)

// TestDeployRollingRestart_NeverPassesRemoveOrphans scans the two source
// files that build "docker compose up" argv for a rolling restart and
// asserts neither ever includes --remove-orphans.
func TestDeployRollingRestart_NeverPassesRemoveOrphans(t *testing.T) {
	for _, path := range []string{"deploy_strategies.go", "deploy_remote.go"} {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if strings.Contains(string(src), "--remove-orphans") {
			t.Errorf("%s must never pass --remove-orphans to a rolling-restart "+
				"docker compose command — production runs real plugin/monitoring "+
				"containers that are intentional orphans relative to the generated "+
				"compose file; removing them would take down services the deploy "+
				"was never asked to touch", path)
		}
	}
}
