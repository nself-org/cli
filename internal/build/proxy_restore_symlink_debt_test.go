package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProxyRestoreCheckedSymlinks(t *testing.T) {
	for _, target := range []string{"dynamic.yml", "missing.yml"} {
		t.Run(target, func(t *testing.T) {
			root, dir := restoreProject(t, testRendererMarker)
			writeFixtureFile(t, filepath.Join(dir, "sub", proxyGeneratedMarker), testRendererMarker, 0644)
			if err := os.Symlink(target, filepath.Join(dir, "link")); err != nil {
				t.Fatal(err)
			}
			plan := newMemSink(root, "")
			if err := cleanupProviderOutput(plan, root, ProviderNginx); err != nil {
				t.Fatal(err)
			}
			checked := newExpectSink(root, plan.snapshot())
			if err := cleanupProviderOutput(checked, root, ProviderNginx); err != nil {
				t.Fatal(err)
			}
		})
	}
}
