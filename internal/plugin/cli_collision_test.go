package plugin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// fakeManifestFields are the required v1 fields parseManifest checks, so a
// fixture is a readable installed plugin and not silently skipped.
const fakeManifestFields = `"version":"1.0.0","description":"fake","category":"utility","license":"MIT"`

// collisionHome points HOME at a temp dir and returns its plugins directory.
func collisionHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return filepath.Join(home, ".nself", "plugins")
}

// extractFake writes plugin <name> into pluginsDir with a manifest carrying
// extra JSON members and the given binaries, as an extracted tarball leaves it.
func extractFake(t *testing.T, pluginsDir, name, extra string, bins ...string) string {
	t.Helper()
	dir := filepath.Join(pluginsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, dir, `{"name":"`+name+`",`+fakeManifestFields+extra+`}`)
	for _, b := range bins {
		if err := os.WriteFile(filepath.Join(dir, b), []byte("#!/bin/sh\necho "+name+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func codeOfErr(err error) string {
	var ce *errs.CLIError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

// TestCheckCLIBinaries_RefusesAnotherPluginsBinary is D-0259: a plugin whose
// cliCommands name a binary another installed plugin publishes is refused
// with E141 naming both plugins. The owner may declare the binary through
// cliCommands or through binaryName alone.
func TestCheckCLIBinaries_RefusesAnotherPluginsBinary(t *testing.T) {
	for _, owner := range []string{
		`,"cliCommands":[{"name":"media-server"}]`,
		`,"pluginType":"cli","binaryName":"nself-media-server"`,
	} {
		pluginsDir := collisionHome(t)
		extractFake(t, pluginsDir, "alpha", owner, "nself-media-server")
		m := &PluginManifest{Name: "beta", CLICommands: []CLICommand{{Name: "beta"}, {Name: "media-server"}}}
		err := checkCLIBinaries(pluginsDir, "beta", m)
		if codeOfErr(err) != "E141" {
			t.Fatalf("owner %s: want E141, got %v", owner, err)
		}
		for _, s := range []string{`"beta"`, `"alpha"`, "media-server"} {
			if !strings.Contains(err.Error(), s) {
				t.Errorf("error does not name %s: %v", s, err)
			}
		}
	}
}

// TestCheckCLIBinaries_ReinstallAndOrphan: the plugin's own manifest is
// skipped, so an upgrade is not a collision, and an orphan bin file no
// installed manifest claims is replaced by the plugin's binary.
func TestCheckCLIBinaries_ReinstallAndOrphan(t *testing.T) {
	pluginsDir := collisionHome(t)
	extractFake(t, pluginsDir, "alpha", `,"cliCommands":[{"name":"media-server"}]`, "nself-media-server")
	m := &PluginManifest{Name: "alpha", CLICommands: []CLICommand{{Name: "media-server"}}}
	if err := checkCLIBinaries(pluginsDir, "alpha", m); err != nil {
		t.Fatalf("reinstall refused: %v", err)
	}

	binDir, err := PluginBinDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(PublishedBinaryPath("nself-orphan"), []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	odir := extractFake(t, pluginsDir, "orphan", `,"pluginType":"cli"`, "nself-orphan")
	om := &PluginManifest{Name: "orphan", PluginType: "cli"}
	if err := checkCLIBinaries(pluginsDir, "orphan", om); err != nil {
		t.Fatalf("orphan bin file blocked install: %v", err)
	}
	if err := linkCLIBinary(odir, "orphan", om); err != nil {
		t.Fatalf("linkCLIBinary: %v", err)
	}
	got, _ := os.ReadFile(PublishedBinaryPath("nself-orphan"))
	if !strings.Contains(string(got), "echo orphan") {
		t.Errorf("orphan bin file was not replaced: %q", got)
	}
}

// TestCheckCLIBinaries_RefusesCoreVerbAndBadNames runs the manifestv2 name
// rules (E113 core verb, E106 name) as part of the same check.
func TestCheckCLIBinaries_RefusesCoreVerbAndBadNames(t *testing.T) {
	pluginsDir := collisionHome(t)
	for _, c := range []struct {
		cmd, code string
	}{{"init", "E113"}, {"status", "E113"}, {"Feeds", "E106"}, {"roms list", "E106"}} {
		err := checkCLIBinaries(pluginsDir, "gamma", &PluginManifest{Name: "gamma", CLICommands: []CLICommand{{Name: c.cmd}}})
		if codeOfErr(err) != c.code {
			t.Errorf("%q: want %s, got %v", c.cmd, c.code, err)
		}
	}
}

// TestInstallFromURL_RefusedReinstallKeepsTheOldInstall is the end-to-end
// guard for the order: a reinstall of beta that now collides with alpha is
// refused with E141 before the old beta directory is removed, and neither
// plugin's published binary changes.
func TestInstallFromURL_RefusedReinstallKeepsTheOldInstall(t *testing.T) {
	pluginsDir := collisionHome(t)
	extractFake(t, pluginsDir, "alpha", `,"cliCommands":[{"name":"media-server"}]`, "nself-media-server")
	oldBeta := extractFake(t, pluginsDir, "beta", `,"pluginType":"cli","binaryName":"nself-beta"`, "nself-beta")

	tarball := buildTarGz(t, []struct{ name, content string }{
		{name: "plugin.json", content: `{"name":"beta",` + fakeManifestFields + `,"cliCommands":[{"name":"beta"},{"name":"media-server"}]}`},
		{name: "nself-beta", content: "#!/bin/sh\necho new beta\n"},
		{name: "nself-media-server", content: "#!/bin/sh\necho new beta\n"},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	}))
	defer srv.Close()

	err := InstallFromURL(context.Background(), &config.Config{}, srv.URL+"/beta.tar.gz", pluginsDir, "")
	if codeOfErr(err) != "E141" {
		t.Fatalf("want E141, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(oldBeta, "plugin.json")); statErr != nil {
		t.Errorf("refused reinstall removed the installed beta: %v", statErr)
	}
	if _, statErr := os.Stat(PublishedBinaryPath("nself-media-server")); !os.IsNotExist(statErr) {
		t.Errorf("refused install published nself-media-server (stat err %v)", statErr)
	}
}

// TestCheckCLIBinaries_RunsBeforeTheInstallIsTouched pins the order the
// D-0259 review asked for: the registry path checks before it extracts, the
// third-party path before it removes the installed copy. A refusal after
// either point goes through rollbackInstall and loses a working upgrade.
func TestCheckCLIBinaries_RunsBeforeTheInstallIsTouched(t *testing.T) {
	for _, c := range []struct{ file, fn, later string }{
		{"installer_locked.go", "func installLocked(", "extractTarGzFrom(archive, destDir)"},
		{"install_thirdparty.go", "func InstallFromURL(", "os.RemoveAll(destDir)"},
	} {
		src := readFile(t, c.file)
		start := strings.Index(src, c.fn)
		if start < 0 {
			t.Fatalf("%s: %s not found", c.file, c.fn)
		}
		body := src[start:]
		if end := strings.Index(body[1:], "\nfunc "); end > 0 {
			body = body[:end+1]
		}
		check, later := strings.Index(body, "checkCLIBinaries("), strings.Index(body, c.later)
		if check < 0 || later < 0 || check > later {
			t.Errorf("%s: checkCLIBinaries (at %d) must run before %s (at %d)", c.file, check, c.later, later)
		}
	}
}
