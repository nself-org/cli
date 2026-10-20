package build

import (
	"path/filepath"
	"testing"
)

func TestProxyRestoreCheckedNestedMarkers(t *testing.T) {
	for _, marker := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "present"}[marker], func(t *testing.T) {
			root, dir := restoreProject(t, testRendererMarker)
			if marker {
				writeFixtureFile(t, filepath.Join(dir, "sub", proxyGeneratedMarker), testRendererMarker, 0644)
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
