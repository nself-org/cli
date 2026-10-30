//go:build e2eagent

package agent

// Purpose: run the replay's CI leg against a ci plugin built from plugins
// source (run.sh --ci-plugin-src) until a served ci release carries the
// machine surface (P7-CI-62). The agent's `nself_add ci` stays a real MCP
// step; the harness then replaces the installed manifest and binary with the
// source build, as a `role: harness` record naming the source revision.
// Inputs: AGENT_E2E_CI_PLUGIN_SRC (the free/ci directory of a plugins checkout).
// Outputs: HOME/.nself/plugins/ci/plugin.json and HOME/.nself/plugins/bin/nself-ci
// in the sandbox (the P7-CI-47 layout).
// Constraints: the build runs before HOME points at the sandbox, so the Go
// caches stay the operator's; without a source dir the served plugin is kept.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CIPluginSrcEnv names the plugins free/ci source directory.
const CIPluginSrcEnv = "AGENT_E2E_CI_PLUGIN_SRC"

// CIPlugin is a ci plugin built from source.
type CIPlugin struct {
	Manifest, Binary, Revision string
}

// BuildCIPlugin builds the ci plugin from $AGENT_E2E_CI_PLUGIN_SRC, or
// returns nil when it is not set.
func BuildCIPlugin(t *testing.T) *CIPlugin {
	t.Helper()
	src := os.Getenv(CIPluginSrcEnv)
	if src == "" {
		return nil
	}
	bin := filepath.Join(t.TempDir(), "nself-ci")
	ctx := context.Background()
	if _, err := run(ctx, src, "go", "build", "-mod=vendor", "-trimpath", "-o", bin, "./cmd"); err != nil {
		t.Fatalf("build the ci plugin from %s: %v", src, err)
	}
	rev, err := run(ctx, src, "git", "rev-parse", "HEAD")
	if err != nil {
		rev = "unknown"
	}
	return &CIPlugin{Manifest: filepath.Join(src, "plugin.json"), Binary: bin, Revision: strings.TrimSpace(rev)}
}

// installCIPlugin replaces the installed ci plugin with the source build.
func (h *SimHarness) installCIPlugin() (string, error) {
	if h.CI == nil {
		return "served ci plugin kept (no " + CIPluginSrcEnv + ")", nil
	}
	root := filepath.Join(h.Box.Home, ".nself", "plugins")
	for src, dst := range map[string]string{
		h.CI.Manifest: filepath.Join(root, "ci", "plugin.json"),
		h.CI.Binary:   filepath.Join(root, "bin", "nself-ci"),
	} {
		b, err := os.ReadFile(src)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(dst+".tmp", b, 0o755); err != nil {
			return "", err
		}
		if err := os.Rename(dst+".tmp", dst); err != nil {
			return "", err
		}
	}
	return "ci plugin from plugins source " + h.CI.Revision, nil
}
