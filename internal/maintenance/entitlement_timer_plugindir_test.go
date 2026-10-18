//go:build darwin || linux

package maintenance

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Purpose: D-0946 regression. A custom plugin directory must be carried into every backend's
// unit as NSELF_PLUGIN_DIR (the timer's own environment is otherwise empty) after the same
// path-safety check as the project and binary paths.

func pluginDirFixture(t *testing.T, root string) string {
	t.Helper()
	p := filepath.Join(root, "plugins")
	if err := os.MkdirAll(p, 0o755); err != nil || os.Chmod(p, 0o755) != nil {
		t.Fatal(err)
	}
	return p
}

func TestRenderCarriesPluginDir(t *testing.T) {
	const plugins = "/srv/plugins"
	hash := Hash(gDir)
	svc, _, err := renderSystemd(gBin, gDir, plugins, hash, 1000)
	cron, err2 := renderCron(gBin, gDir, plugins, hash)
	plist, err3 := renderPlist(gBin, gDir, plugins, hash)
	if err != nil || err2 != nil || err3 != nil {
		t.Fatal(err, err2, err3)
	}
	for name, out := range map[string]string{"systemd": svc, "cron": cron, "plist": string(plist)} {
		if !strings.Contains(out, "NSELF_PLUGIN_DIR=") || !strings.Contains(out, plugins) { // quoting differs per format
			t.Errorf("%s text does not carry NSELF_PLUGIN_DIR:\n%s", name, out)
		}
	}
	// No plugin directory: the text is the unchanged golden (the timer resolves its own default).
	plain, _ := renderCron(gBin, gDir, "", hash)
	if strings.Contains(plain, "NSELF_PLUGIN_DIR") {
		t.Fatalf("default plugin dir leaked into the unit: %s", plain)
	}
	if _, err := renderCron(gBin, gDir, "/srv/plug\nins", hash); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("control character in the plugin dir accepted: %v", err)
	}
}

func TestEnsureCarriesValidatedPluginDir(t *testing.T) {
	for _, tc := range []struct {
		goos, backend string
		euid          int
	}{{"linux", BackendSystemd, 0}, {"linux", BackendCron, 1000}, {"darwin", BackendLaunchAgent, 501}} {
		t.Run(tc.backend, func(t *testing.T) {
			f, root := newFake(t, tc.goos, tc.euid)
			dir, bin := project(t, root)
			f.exe = bin
			plugins := pluginDirFixture(t, root)
			if res, err := f.ensure(dir, bin, plugins); err != nil || !res.Changed {
				t.Fatalf("ensure = %+v, %v", res, err)
			}
			if res, err := f.ensure(dir, bin, plugins); err != nil || res.Changed {
				t.Fatalf("second ensure = %+v, %v (want no change)", res, err)
			}
			var text string
			switch tc.backend {
			case BackendSystemd:
				b, _ := os.ReadFile(filepath.Join(f.unitDir, entUnitName(Hash(dir))+".service"))
				text = string(b)
			case BackendCron:
				text = f.cron
			default:
				b, _ := os.ReadFile(filepath.Join(f.launchDir, launchAgentLabel(Hash(dir))+".plist"))
				text = string(b)
			}
			if !strings.Contains(text, "NSELF_PLUGIN_DIR=") || !strings.Contains(text, plugins) {
				t.Fatalf("installed unit does not carry the plugin dir %q:\n%s", plugins, text)
			}
		})
	}
}

func TestEnsureRejectsUnsafePluginDir(t *testing.T) {
	f, root := newFake(t, "linux", 1000)
	dir, bin := project(t, root)
	f.exe = bin
	loose := pluginDirFixture(t, root)
	if err := os.Chmod(loose, 0o777); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]string{"world-writable": loose, "relative": "plugins", "missing": filepath.Join(root, "nope")} {
		if _, err := f.ensure(dir, bin, p); !errors.Is(err, ErrUnsafePath) {
			t.Errorf("%s plugin dir: err = %v, want ErrUnsafePath", name, err)
		}
	}
	if f.cron != "" {
		t.Fatalf("a crontab line was written for an unsafe plugin dir: %q", f.cron)
	}
}
