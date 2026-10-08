package commands

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestCanonExtendResolution(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		undo := prepareTreeWith(&canonTable, RootCmd, compat.V15(), false)
		defer undo()
		for _, row := range [][2]string{{"install", "add"}, {"plugin", "config plugins"}, {"plugin install", "add"}, {"plugin remove", "remove"}, {"bundle", "config bundles"}, {"bundle install", "config bundles install"}, {"bundle remove", "remove"}} {
			args, _, err := rewriteCanonArgsWith(&canonTable, RootCmd, strings.Fields(row[0]), compat.V15())
			if err != nil {
				t.Fatalf("rewrite %s: %v", row[0], err)
			}
			cmd, _, err := RootCmd.Find(args)
			if err != nil {
				t.Fatalf("find %s: %v", row[0], err)
			}
			want := row[0]
			if compat.V15() {
				want = row[1]
			}
			if got := strings.TrimPrefix(cmd.CommandPath(), "nself "); got != want {
				t.Errorf("%s: got %s, want %s", row[0], got, want)
			}
		}
	})
}

func TestCanonExtendRegistry(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		r, err := canon.Effective(compat.V15())
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range [][2]string{{"install", "add"}, {"plugin", "config plugins"}, {"bundle", "config bundles"}} {
			e, ok := r.Commands[row[0]]
			if !ok {
				t.Errorf("missing %s", row[0])
				continue
			}
			if compat.V15() && (e.Canon != canon.CanonShim || e.Target != row[1]) {
				t.Errorf("%s: canon=%s target=%s", row[0], e.Canon, e.Target)
			}
			if !compat.V15() && e.Canon == canon.CanonShim {
				t.Errorf("%s shimmed in v1.4", row[0])
			}
		}
		if compat.V15() {
			if e := r.Commands["add"]; e.Canon != canon.CanonCore {
				t.Errorf("add canon=%q", e.Canon)
			}
			for _, row := range [][2]string{{"plugin install", "add"}, {"plugin remove", "remove"}, {"bundle remove", "remove"}} {
				if e := r.Commands[row[0]]; e.Canon != canon.CanonShim || e.Target != row[1] {
					t.Errorf("%s: canon=%q target=%q", row[0], e.Canon, e.Target)
				}
			}
			if _, ok := r.Commands["config bundles install"]; !ok {
				t.Error("missing moved bundle install")
			}
		}
	})
}

func TestInstallFlagParity(t *testing.T) {
	for _, row := range []struct{ target, source *cobra.Command }{{installCmd, pluginInstallCmd}, {installCmd, bundleInstallCmd}, {removeCmd, pluginRemoveCmd}, {removeCmd, bundleRemoveCmd}} {
		row.source.Flags().VisitAll(func(f *pflag.Flag) {
			got := row.target.Flags().Lookup(f.Name)
			if got == nil {
				t.Errorf("%s missing --%s from %s", row.target.Name(), f.Name, row.source.CommandPath())
				return
			}
			if got.Value.Type() != f.Value.Type() || got.DefValue != f.DefValue {
				t.Errorf("%s --%s type/default = %s/%s, want %s/%s", row.target.Name(), f.Name, got.Value.Type(), got.DefValue, f.Value.Type(), f.DefValue)
			}
			// Shared flags with different upstream usage cannot have both texts.
			if counterpart := otherExtensionSource(row.source); (counterpart == nil || counterpart.Flags().Lookup(f.Name) == nil) && !(row.source == pluginInstallCmd && f.Name == "yes") {
				if got.Usage != f.Usage {
					t.Errorf("%s --%s usage = %q, want %q", row.target.Name(), f.Name, got.Usage, f.Usage)
				}
			}
		})
	}
}

func otherExtensionSource(source *cobra.Command) *cobra.Command {
	switch source {
	case pluginInstallCmd:
		return bundleInstallCmd
	case bundleInstallCmd:
		return pluginInstallCmd
	case pluginRemoveCmd:
		return bundleRemoveCmd
	case bundleRemoveCmd:
		return pluginRemoveCmd
	default:
		return nil
	}
}

func TestBuiltinAddRemove(t *testing.T) {
	if os.Getenv("NSELF_C08_BUILTIN_CHILD") == "1" {
		testBuiltinAddRemoveInChild(t)
		return
	}
	compattest.Both(t, func(t *testing.T) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestBuiltinAddRemove$")
		cmd.Env = append(os.Environ(), "NSELF_C08_BUILTIN_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("builtin lifecycle: %v\n%s", err, output)
		}
	})
}

func testBuiltinAddRemoveInChild(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", dir)
	old := builtinFamilies
	builtinFamilies = []builtinFamily{{slug: "zzfam", build: func() *cobra.Command {
		return &cobra.Command{Use: "zzfam", RunE: func(*cobra.Command, []string) error { return nil }}
	}}}
	defer func() { builtinFamilies = old }()
	defer func() {
		for _, node := range RootCmd.Commands() {
			if node.Name() == "zzfam" {
				RootCmd.RemoveCommand(node)
			}
		}
	}()
	if handled, err := toggleBuiltin([]string{"unknown"}, false); handled || err != nil {
		t.Fatalf("unknown handled=%v err=%v", handled, err)
	}
	if handled, err := toggleBuiltin([]string{"zzfam"}, false); !handled || err != nil {
		t.Fatalf("disable handled=%v err=%v", handled, err)
	}
	if builtinEnabled("zzfam") {
		t.Fatal("disabled family remains enabled")
	}
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"nself", "zzfam"}
	err := Execute()
	if err == nil || !strings.Contains(err.Error(), "E407") || !strings.Contains(err.Error(), "nself add zzfam") {
		t.Fatalf("disabled dispatch error = %v", err)
	}
	if handled, err := toggleBuiltin([]string{"zzfam"}, true); !handled || err != nil {
		t.Fatalf("enable handled=%v err=%v", handled, err)
	}
	if !builtinEnabled("zzfam") {
		t.Fatal("enabled family remains disabled")
	}
	if _, err := os.Stat(filepath.Join(dir, "zzfam", ".disabled")); !os.IsNotExist(err) {
		t.Fatalf("marker still exists: %v", err)
	}
	if err := Execute(); err != nil {
		t.Fatalf("re-enabled builtin did not run: %v", err)
	}
}

func TestAddNextHint(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("NSELF_PLUGIN_DIR", dir)
		p := filepath.Join(dir, "zzhint")
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest, err := os.ReadFile(filepath.Join("testdata", "mount", "collide-completion", "plugin.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "plugin.json"), manifest, 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := captureStdout(t, func() error { printAddedCommandHints([]string{"zzhint"}); return nil })
		if err != nil {
			t.Fatal(err)
		}
		want := ""
		if compat.V15() {
			want = "Next: nself completion --help\n"
		}
		if out != want {
			t.Errorf("hint=%q, want %q", out, want)
		}
	})
}

// equivalenceResult records the observable command result and installed tree.
type equivalenceResult struct {
	stdout, stderr, err string
	tree                []string
}

func runExtensionFixture(t *testing.T, fixture func(string) error, invoke func() error) equivalenceResult {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	pluginDir := filepath.Join(home, ".nself", "plugins")
	t.Setenv("NSELF_PLUGIN_DIR", pluginDir)
	if fixture != nil {
		if err := fixture(pluginDir); err != nil {
			t.Fatal(err)
		}
	}
	var result equivalenceResult
	result.stderr = captureStderr(t, func() {
		var err error
		result.stdout, err = captureStdout(t, invoke)
		if err != nil {
			result.err = err.Error()
		}
	})
	result.stdout = strings.ReplaceAll(result.stdout, home, "<HOME>")
	result.stderr = strings.ReplaceAll(result.stderr, home, "<HOME>")
	result.err = strings.ReplaceAll(result.err, home, "<HOME>")
	_ = filepath.WalkDir(pluginDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(pluginDir, path)
		if err != nil {
			return err
		}
		result.tree = append(result.tree, rel)
		return nil
	})
	return result
}

func compareExtensionResults(t *testing.T, legacy, canonical equivalenceResult) {
	t.Helper()
	if !reflect.DeepEqual(legacy, canonical) {
		t.Fatalf("legacy=%+v\ncanonical=%+v", legacy, canonical)
	}
}

func TestEquivalencePluginInstall(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		manifest := `{"name":"zzfixture","version":"1.0.0","description":"fixture","category":"utility","license":"MIT"}`
		var archive bytes.Buffer
		gz := gzip.NewWriter(&archive)
		tw := tar.NewWriter(gz)
		if err := tw.WriteHeader(&tar.Header{Name: "plugin.json", Mode: 0o644, Size: int64(len(manifest))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(manifest)); err != nil {
			t.Fatal(err)
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(archive.Bytes())
		var downloads int
		registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/tarball") {
				downloads++
				_, _ = w.Write(archive.Bytes())
				return
			}
			_, _ = fmt.Fprintf(w, `{"plugins":[{"name":"zzfixture","version":"1.0.0","description":"fixture","category":"utility","tier":"free","license":"MIT","checksum":"%s"}]}`, hex.EncodeToString(sum[:]))
		}))
		defer registry.Close()
		t.Setenv("NSELF_PLUGIN_REGISTRY", registry.URL)
		t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "fixture-key")
		for _, cmd := range []*cobra.Command{pluginInstallCmd, installCmd} {
			if err := cmd.Flags().Set("skip-sbom-check", "true"); err != nil {
				t.Fatal(err)
			}
			defer cmd.Flags().Set("skip-sbom-check", "false")
		}
		legacy := runExtensionFixture(t, nil, func() error { return runPluginInstall(pluginInstallCmd, []string{"zzfixture"}) })
		canonical := runExtensionFixture(t, nil, func() error { return runInstall(installCmd, []string{"zzfixture"}) })
		if !strings.Contains(legacy.stderr, "installed successfully") || legacy.err != "" || len(legacy.tree) < 2 || downloads != 2 {
			t.Fatalf("fixture install was not exercised: %+v", legacy)
		}
		compareExtensionResults(t, legacy, canonical)
	})
}

func TestEquivalencePluginRemove(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		for _, cmd := range []*cobra.Command{pluginRemoveCmd, removeCmd} {
			if err := cmd.Flags().Set("keep-data", "true"); err != nil {
				t.Fatal(err)
			}
			defer cmd.Flags().Set("keep-data", "false")
		}
		fixture := func(dir string) error { return os.MkdirAll(filepath.Join(dir, "zzfixture"), 0o755) }
		legacy := runExtensionFixture(t, fixture, func() error { return runPluginRemove(pluginRemoveCmd, []string{"zzfixture"}) })
		canonical := runExtensionFixture(t, fixture, func() error { return runRemove(removeCmd, []string{"zzfixture"}) })
		if legacy.err != "" || !strings.Contains(legacy.stderr, "removed successfully") {
			t.Fatalf("fixture removal was not exercised: %+v", legacy)
		}
		compareExtensionResults(t, legacy, canonical)
	})
}

func TestEquivalenceBundleRemove(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		if err := bundle.LoadBytes([]byte(fixtureBundlesJSON)); err != nil {
			t.Fatal(err)
		}
		fixture := func(dir string) error { return os.MkdirAll(filepath.Join(dir, "bots"), 0o755) }
		for _, cmd := range []*cobra.Command{bundleRemoveCmd, removeCmd} {
			if err := cmd.Flags().Set("keep-data", "true"); err != nil {
				t.Fatal(err)
			}
			defer cmd.Flags().Set("keep-data", "false")
		}
		legacy := runExtensionFixture(t, fixture, func() error { return runBundleRemove(bundleRemoveCmd, []string{"nchat"}) })
		canonical := runExtensionFixture(t, fixture, func() error { return runRemove(removeCmd, []string{"nchat"}) })
		if legacy.err != "" || !strings.Contains(legacy.stderr, "bots") || len(legacy.tree) != 1 {
			t.Fatalf("fixture bundle was not exercised: %+v", legacy)
		}
		compareExtensionResults(t, legacy, canonical)
	})
}
