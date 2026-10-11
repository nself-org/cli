package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// hashTree returns "relpath sha256" lines for every file under roots.
func hashTree(t *testing.T, roots ...string) string {
	t.Helper()
	var lines []string
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			b, rerr := os.ReadFile(p)
			if rerr != nil {
				t.Fatal(rerr)
			}
			sum := sha256.Sum256(b)
			lines = append(lines, p+" "+hex.EncodeToString(sum[:]))
			return nil
		})
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

var nselfInvocation = regexp.MustCompile(`(^|[;&|(\s])nself(\s|$)`)

func seedTarget(t *testing.T, f *fakeRemote, rp string) {
	t.Helper()
	pd := filepath.Join(f.home, ".nself", "plugins")
	writeFile(t, filepath.Join(pd, "alpha", "plugin.json"), `{"name":"alpha","version":"1.2.0"}`, 0o644)
	writeFile(t, filepath.Join(pd, "beta", "plugin.json"), `{"name":"beta","version":"0.9.1"}`, 0o644)
	writeFile(t, filepath.Join(pd, "beta", ".disabled"), "", 0o644)
	writeFile(t, filepath.Join(pd, "broken", "plugin.json"), `not json`, 0o644)
	writeFile(t, filepath.Join(f.home, ".nself", "cache", "registry.json"), `{"cached":true}`, 0o644)
	cd := filepath.Join(rp, "ssl", "certificates")
	for _, n := range []string{"fullchain.pem", "privkey.pem", "chain.pem"} {
		writeFile(t, filepath.Join(cd, "api-example-com", n), "KEY MATERIAL "+n, 0o600)
	}
	writeFile(t, filepath.Join(cd, "www-example-com", "fullchain.pem"), "x", 0o644)
	writeFile(t, filepath.Join(rp, "docker-compose.yml"), "services: {}\n", 0o644)
}

func TestTargetSnapshotReadOnly(t *testing.T) {
	f := newFakeRemote(t)
	rp := t.TempDir()
	seedTarget(t, f, rp)
	before := hashTree(t, f.home, rp)

	snap, err := TakeTargetSnapshot(context.Background(), f, rp)
	if err != nil {
		t.Fatal(err)
	}
	if after := hashTree(t, f.home, rp); after != before {
		t.Fatalf("the snapshot changed files on the host:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if want := filepath.Join(f.home, ".nself", "plugins"); snap.PluginDir != want {
		t.Fatalf("plugin dir %q, want %q", snap.PluginDir, want)
	}
	wantPlugins := []PluginRef{{Name: "alpha", Version: "1.2.0"}, {Name: "beta", Version: "0.9.1", Disabled: true}}
	if len(snap.Plugins) != 2 || snap.Plugins[0] != wantPlugins[0] || snap.Plugins[1] != wantPlugins[1] {
		t.Fatalf("plugins %+v, want %+v (the unreadable manifest is not a plugin)", snap.Plugins, wantPlugins)
	}
	wantSSL := []CertDir{{Name: "api-example-com", Fullchain: true, Privkey: true, Chain: true}, {Name: "www-example-com", Fullchain: true}}
	if len(snap.SSL) != 2 || snap.SSL[0] != wantSSL[0] || snap.SSL[1] != wantSSL[1] {
		t.Fatalf("ssl %+v, want %+v", snap.SSL, wantSSL)
	}
	for _, c := range f.commands() {
		if nselfInvocation.MatchString(c) {
			t.Fatalf("the snapshot ran nself on the target: %q", c)
		}
		if strings.Contains(c, "privkey") || strings.Contains(c, "cat ") && strings.Contains(c, ".pem") {
			t.Fatalf("the snapshot reads key material: %q", c)
		}
	}
}

func TestTargetSnapshotPluginDirFromEnv(t *testing.T) {
	f := newFakeRemote(t)
	rp := t.TempDir()
	custom := filepath.Join(t.TempDir(), "plugins")
	writeFile(t, filepath.Join(custom, "gamma", "plugin.json"), `{"name":"gamma","version":"2.0.0"}`, 0o644)
	writeFile(t, filepath.Join(rp, ".env"), "A=1\nPLUGIN_DIR=/nowhere\nNSELF_PLUGIN_DIR=\""+custom+"\"\n", 0o600)
	snap, err := TakeTargetSnapshot(context.Background(), f, rp)
	if err != nil {
		t.Fatal(err)
	}
	if snap.PluginDir != custom || len(snap.Plugins) != 1 || snap.Plugins[0].Name != "gamma" {
		t.Fatalf("snapshot %+v, want plugin dir %s with gamma", snap, custom)
	}
	if got := pluginDirFrom("PLUGIN_DIR=$HOME/p\n", "/home/u", "/opt/x"); got != "/home/u/p" {
		t.Fatalf("$HOME expansion: %q", got)
	}
	if got := pluginDirFrom("PLUGIN_DIR=plugins\n", "/home/u", "/opt/x"); got != "/opt/x/plugins" {
		t.Fatalf("relative: %q", got)
	}
}

func TestPluginDriftAndLayoutDrift(t *testing.T) {
	target := TargetSnapshot{
		Plugins: []PluginRef{{Name: "alpha", Version: "1.2.0"}, {Name: "beta", Version: "0.9.1", Disabled: true}, {Name: "extra", Version: "1"}},
		SSL:     []CertDir{{Name: "api", Fullchain: true}},
	}
	local := []PluginRef{{Name: "alpha", Version: "1.3.0"}, {Name: "beta", Version: "0.9.1"}, {Name: "only-local", Version: "1"}}
	diffs := PluginDrift(local, target)
	joined := strings.Join(diffs, "|")
	for _, want := range []string{"alpha is 1.3.0 locally but 1.2.0", "beta 0.9.1 is installed locally but not on the target", "only-local 1 is installed locally", "extra 1 is on the target but not installed locally"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %q", want, joined)
		}
	}
	err := DriftError(diffs)
	if err == nil || !strings.Contains(err.Error(), "target drift") || !strings.Contains(err.Error(), "E485") {
		t.Fatalf("drift error %v", err)
	}
	if DriftError(nil) != nil {
		t.Fatal("no differences must be no error")
	}
	other := TargetSnapshot{Plugins: target.Plugins, SSL: []CertDir{{Name: "api"}, {Name: "new"}}}
	ld := strings.Join(LayoutDrift(target, other), "|")
	if !strings.Contains(ld, "ssl dir api has different files") || !strings.Contains(ld, "ssl dir new exists") {
		t.Fatalf("layout drift %q", ld)
	}
}
