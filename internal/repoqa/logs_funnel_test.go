package repoqa

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLogsDockerFunnel prevents logs commands from bypassing internal/docker,
// including sites that were historically tolerated by the funnel allowlist.
func TestLogsDockerFunnel(t *testing.T) {
	root := repoRoot(t)
	for _, name := range []string{"logs.go", "logs_metrics.go", "logs_reports.go", "logs_stream.go"} {
		rel := filepath.Join("cmd", "commands", name)
		src, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		_, sites, err := scanSource(rel, src)
		if err != nil {
			t.Fatal(err)
		}
		if sites != 0 {
			t.Errorf("%s: %d direct docker calls; use internal/docker", rel, sites)
		}
	}
}
