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

// writeNginxConf plants <plugins>/<plugin>/nginx/<file> with a server block
// for host.
func writeNginxConf(t *testing.T, plugins, plugin, file, host string) {
	t.Helper()
	conf := filepath.Join(plugins, plugin, "nginx", file)
	if err := os.MkdirAll(filepath.Dir(conf), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "server { listen 80; server_name " + host + "; }\n"
	if err := os.WriteFile(conf, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestPluginNginxDestCollision (D-0287): plugin "a-b" with c.conf and plugin
// "a" with b-c.conf both map to nginx/sites/a-b-c.conf.
func TestPluginNginxDestCollision(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		resetWarnOnce()
		project, plugins := t.TempDir(), t.TempDir()
		writeNginxConf(t, plugins, "a-b", "c.conf", "one.fixture.test")
		writeNginxConf(t, plugins, "a", "b-c.conf", "two.fixture.test")

		var count int
		var err error
		stderr := captureStderr(t, func() {
			count, err = InjectPluginNginxRoutes(project, plugins, minimalTestConfig("test"))
		})
		entries, _ := os.ReadDir(filepath.Join(project, "nginx", "sites"))
		if compat.V15() {
			var ce *errs.CLIError
			if !errors.As(err, &ce) || ce.Code != "E055" {
				t.Fatalf("v1.5: want E055, got %v", err)
			}
			for _, want := range []string{`"a"`, `"a-b"`, "a-b-c.conf", "b-c.conf", "c.conf"} {
				if !strings.Contains(ce.What, want) {
					t.Errorf("E055 should mention %s: %s", want, ce.What)
				}
			}
			if ce.Why == "" || ce.Fix == "" {
				t.Errorf("E055 needs Why and Fix: %+v", ce)
			}
			return
		}
		if err != nil {
			t.Fatalf("v1.4 must not fail: %v", err)
		}
		if len(entries) != 1 || count != 2 {
			t.Errorf("v1.4: want 1 file and 2 writes, got %d files, count %d", len(entries), count)
		}
		if !strings.Contains(stderr, "a-b-c.conf") || !strings.Contains(stderr, "replaces") {
			t.Errorf("v1.4 should warn about the overwrite: %q", stderr)
		}
	})
}

func TestPluginNginxDistinctNamesNoCollision(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		project, plugins := t.TempDir(), t.TempDir()
		writeNginxConf(t, plugins, "a", "x.conf", "one.fixture.test")
		writeNginxConf(t, plugins, "a-b", "x.conf", "two.fixture.test")
		writeNginxConf(t, plugins, "a", "y.conf", "three.fixture.test")
		count, err := InjectPluginNginxRoutes(project, plugins, minimalTestConfig("test"))
		if err != nil || count != 3 {
			t.Fatalf("count=%d err=%v", count, err)
		}
	})
}

// TestPluginNginxDestCollisionCaseInsensitive: a-b + C.conf and a + b-c.conf
// are one file on a case-insensitive disk (APFS), so they count as a collision.
func TestPluginNginxDestCollisionCaseInsensitive(t *testing.T) {
	compattest.Set(t, true)
	owners := map[string]nginxDestOwner{}
	if err := checkNginxDestCollision(owners, "a-b-C.conf", "a-b", "/p/a-b/nginx/C.conf"); err != nil {
		t.Fatal(err)
	}
	err := checkNginxDestCollision(owners, "a-b-c.conf", "a", "/p/a/nginx/b-c.conf")
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E055" {
		t.Fatalf("want E055, got %v", err)
	}
	// Same plugin, same name: not a collision.
	if err := checkNginxDestCollision(owners, "A-B-c.conf", "a-b", "/p/a-b/nginx/c.conf"); err != nil {
		t.Errorf("same plugin must not collide: %v", err)
	}

	project, plugins := t.TempDir(), t.TempDir()
	writeNginxConf(t, plugins, "a-b", "C.conf", "one.fixture.test")
	writeNginxConf(t, plugins, "a", "b-c.conf", "two.fixture.test")
	_, err = InjectPluginNginxRoutes(project, plugins, minimalTestConfig("test"))
	if !errors.As(err, &ce) || ce.Code != "E055" {
		t.Fatalf("inject: want E055, got %v", err)
	}
}
