package build

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
)

func sitesFile(project, name string) string {
	return filepath.Join(project, "nginx", "sites", name)
}

// TestPluginNginxStampsOwner (D-0491): an injected site file names its owner
// (project/plugin) on a line right after the generated marker.
func TestPluginNginxStampsOwner(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		project, plugins := t.TempDir(), t.TempDir()
		writeNginxConf(t, plugins, "storage", "console.conf", "console.fixture.test")
		if _, err := InjectPluginNginxRoutes(project, plugins, minimalTestConfig("test")); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(sitesFile(project, "storage-console.conf"))
		if err != nil {
			t.Fatal(err)
		}
		want := nginxGeneratedMarker + "\n" + nginxOwnerPrefix + "testproj/storage\n"
		if !strings.HasPrefix(string(data), want) {
			t.Errorf("want owner stamp after marker, got:\n%s", data)
		}
		// A rebuild by the same owner rewrites its own file.
		if _, err := InjectPluginNginxRoutes(project, plugins, minimalTestConfig("test")); err != nil {
			t.Fatalf("same owner rebuild: %v", err)
		}
	})
}

// TestPluginNginxRefusesForeignOwner (D-0491): a file stamped by another
// project (shared fronting sites dir) is not replaced; v1.4 warns and keeps
// the old behaviour.
func TestPluginNginxRefusesForeignOwner(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		resetWarnOnce()
		project, plugins := t.TempDir(), t.TempDir()
		writeNginxConf(t, plugins, "storage", "console.conf", "console.fixture.test")
		other := nginxGeneratedMarker + "\n" + nginxOwnerPrefix + "otherproj/storage\nserver { listen 80; server_name other.fixture.test; }\n"
		if err := os.MkdirAll(filepath.Dir(sitesFile(project, "x")), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(sitesFile(project, "storage-console.conf"), []byte(other), 0o644); err != nil {
			t.Fatal(err)
		}
		var err error
		stderr := captureStderr(t, func() {
			_, err = InjectPluginNginxRoutes(project, plugins, minimalTestConfig("test"))
		})
		got, _ := os.ReadFile(sitesFile(project, "storage-console.conf"))
		if compat.V15() {
			var ce *errs.CLIError
			if !errors.As(err, &ce) || ce.Code != "E055" {
				t.Fatalf("v1.5: want E055, got %v", err)
			}
			if !strings.Contains(ce.What, "otherproj/storage") || !strings.Contains(ce.What, "testproj/storage") {
				t.Errorf("E055 should name both owners: %s", ce.What)
			}
			if string(got) != other {
				t.Errorf("v1.5 must leave the other owner's file untouched:\n%s", got)
			}
			return
		}
		if err != nil {
			t.Fatalf("v1.4 must not fail: %v", err)
		}
		if !strings.Contains(stderr, "otherproj/storage") {
			t.Errorf("v1.4 should warn naming the old owner: %q", stderr)
		}
	})
}

// TestPluginNginxRefusesCoreSiteFile (D-0491): plugin storage + console.conf
// maps to storage-console.conf, a core route file with no owner stamp.
func TestPluginNginxRefusesCoreSiteFile(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		resetWarnOnce()
		project, plugins := t.TempDir(), t.TempDir()
		writeNginxConf(t, plugins, "storage", "console.conf", "console.fixture.test")
		writeExistingSite(t, project, "storage-console.conf", "core-console.fixture.test")
		coreBefore, _ := os.ReadFile(sitesFile(project, "storage-console.conf"))
		var err error
		captureStderr(t, func() {
			_, err = InjectPluginNginxRoutes(project, plugins, minimalTestConfig("test"))
		})
		if !compat.V15() {
			if err != nil {
				t.Fatalf("v1.4 must not fail: %v", err)
			}
			return
		}
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E055" {
			t.Fatalf("v1.5: want E055, got %v", err)
		}
		after, _ := os.ReadFile(sitesFile(project, "storage-console.conf"))
		if string(after) != string(coreBefore) {
			t.Errorf("core site file was replaced:\n%s", after)
		}
	})
}

// TestPluginNginxLegacyUnstampedPluginFile: a plugin file written before the
// owner stamp existed (marker, no owner line, not a core name) is upgraded.
func TestPluginNginxLegacyUnstampedPluginFile(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		project, plugins := t.TempDir(), t.TempDir()
		writeNginxConf(t, plugins, "a", "x.conf", "one.fixture.test")
		if err := os.MkdirAll(filepath.Dir(sitesFile(project, "x")), 0o755); err != nil {
			t.Fatal(err)
		}
		legacy := nginxGeneratedMarker + "\nserver { listen 80; server_name one.fixture.test; }\n"
		if err := os.WriteFile(sitesFile(project, "a-x.conf"), []byte(legacy), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := InjectPluginNginxRoutes(project, plugins, minimalTestConfig("test")); err != nil {
			t.Fatalf("legacy file must be upgraded: %v", err)
		}
		got, _ := os.ReadFile(sitesFile(project, "a-x.conf"))
		if !strings.Contains(string(got), nginxOwnerPrefix+"testproj/a\n") {
			t.Errorf("want stamp after upgrade:\n%s", got)
		}
	})
}
