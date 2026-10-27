package plugin

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("write plugin.json: %v", err)
	}
}

// TestLinkCLIBinary_PublishesWhereProxyLooks is the regression guard for the
// gap CLI-R11's first extraction exposed: Install extracted a plugin to
// ~/.nself/plugins/<name>/ while ProxyCommand only ever searched
// ~/.nself/plugins/bin/. A command-providing plugin installed cleanly and then
// could not be run — the user saw "unknown command" immediately after a
// successful install.
func TestLinkCLIBinary_PublishesWhereProxyLooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	destDir := filepath.Join(home, ".nself", "plugins", "dogfood")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "nself-dogfood"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	m := &PluginManifest{PluginType: "cli", BinaryName: "nself-dogfood"}
	if err := linkCLIBinary(destDir, "dogfood", m); err != nil {
		t.Fatalf("linkCLIBinary: %v", err)
	}

	published := PublishedBinaryPath("nself-dogfood")
	info, err := os.Stat(published)
	if err != nil {
		t.Fatalf("binary was not published where ProxyCommand looks (%s): %v", published, err)
	}
	// Windows has no executable permission bit — what makes a file runnable
	// there is the .exe extension, which PublishedBinaryPath supplies. Go
	// reports 0666 for any file it writes, so this assertion is Unix-only.
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Errorf("published binary is not executable: mode %v", info.Mode().Perm())
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(published, ".exe") {
		t.Errorf("published binary needs a .exe suffix to be runnable on Windows: %s", published)
	}
}

// TestLinkCLIBinary_IgnoresServicePlugins keeps the change narrow: the vast
// majority of plugins are services and must not have anything published into
// the command lookup path.
func TestLinkCLIBinary_IgnoresServicePlugins(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	destDir := filepath.Join(home, ".nself", "plugins", "backup")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "nself-backup"), []byte("x"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}

	m := &PluginManifest{PluginType: "service"}
	if err := linkCLIBinary(destDir, "backup", m); err != nil {
		t.Fatalf("linkCLIBinary: %v", err)
	}

	if _, err := os.Stat(PublishedBinaryPath("nself-backup")); err == nil {
		t.Error("a service plugin had a binary published into the command lookup path")
	}
}

// TestLinkCLIBinary_SourceOnlyIsAnError covers a plugin that declares a command
// but ships no built binary.
//
// This assertion is the reverse of what it was. It originally said a
// source-only package "should not fail install", on the assumption that such a
// package was a work in progress. Installing a real one showed what that
// assumption costs: release-tarballs.yml publishes plugins by running
// `tar -czf` over the source directory and never compiles, so EVERY extracted
// Go command arrives as source. The install reported success and the command
// stayed dead, with nothing anywhere saying why.
//
// Tolerating it here is what made that silent. A plugin whose entire purpose is
// to provide a command, arriving without the command, is a failed install.
func TestLinkCLIBinary_SourceOnlyIsAnError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	destDir := filepath.Join(home, ".nself", "plugins", "dogfood")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeManifest(t, destDir, `{"name":"dogfood","pluginType":"cli"}`)

	err := linkCLIBinary(destDir, "dogfood", &PluginManifest{PluginType: "cli"})
	if err == nil {
		t.Fatal("a CLI plugin with no binary installed silently; the command would be dead with no explanation")
	}
	if !strings.Contains(err.Error(), "source-only") {
		t.Errorf("error does not name the cause, so the user cannot act on it: %v", err)
	}
}

// TestUnlinkCLIBinary_RemovesPublishedCommand proves removal is symmetric: the
// proxy must stop resolving a plugin the user just removed.
func TestUnlinkCLIBinary_RemovesPublishedCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	binDir, err := PluginBinDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	published := PublishedBinaryPath("nself-dogfood")
	if err := os.WriteFile(published, []byte("x"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := unlinkCLIBinary("dogfood", &PluginManifest{PluginType: "cli"}); err != nil {
		t.Fatalf("unlinkCLIBinary: %v", err)
	}
	if _, err := os.Stat(published); err == nil {
		t.Error("published command survived plugin removal — the proxy would still run it")
	}
}

// TestUnlinkCLIBinary_MissingIsNotAnError keeps uninstall working for the
// plugin that never published anything.
func TestUnlinkCLIBinary_MissingIsNotAnError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if err := unlinkCLIBinary("never-installed", nil); err != nil {
		t.Fatalf("removing a plugin that published nothing should succeed: %v", err)
	}
}

// TestIsCommandInstalled_TracksThePublishedBinary backs the rule that decides
// whether a relocated command still nags the user.
//
// Found by round-tripping a real plugin: with nself-soak installed, `nself
// soak` still printed "use nself install soak" — telling the user to install
// what they had just installed. The notice must fire only when the plugin is
// actually absent.
func TestIsCommandInstalled_TracksThePublishedBinary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	if IsCommandInstalled("soak") {
		t.Fatal("reported installed before anything was published")
	}

	binDir, err := PluginBinDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(PublishedBinaryPath("nself-soak"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}

	if !IsCommandInstalled("soak") {
		t.Error("did not see the published binary that ProxyCommand would run")
	}

	// A directory of the right name is not an installed command.
	if err := os.Remove(PublishedBinaryPath("nself-soak")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := os.MkdirAll(PublishedBinaryPath("nself-soak"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if IsCommandInstalled("soak") {
		t.Error("a directory was treated as an installed command binary")
	}
}

// TestProxyCommandWithHint_SuppressesAContradictoryHint guards a real UX bug.
//
// A plugin is not always named after the command it provides: `claw` lives in a
// plugin called `claw-cli`, because a paid `claw` service plugin already owns
// that name. The deprecation registry said "use 'nself install claw-cli'" and
// the proxy's generic hint said "nself install claw" one line below it — two
// different instructions, one of which does not work.
func TestProxyCommandWithHint_SuppressesAContradictoryHint(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// Empty hint: the caller has already printed an accurate one.
	err := ProxyCommandWithHint("claw", nil, "")
	if err == nil {
		t.Fatal("expected an error for an uninstalled command")
	}
	if strings.Contains(err.Error(), "install it with") {
		t.Errorf("emitted a second install instruction after the registry gave one: %q", err)
	}

	// No registry entry: the generic hint is the only guidance, so keep it.
	err = ProxyCommandWithHint("frobnicate", nil, "nself install frobnicate")
	if err == nil {
		t.Fatal("expected an error for an uninstalled command")
	}
	if !strings.Contains(err.Error(), "nself install frobnicate") {
		t.Errorf("dropped the only install hint the user would get: %q", err)
	}
}

// TestProxyCommandDoesNotLogToSlog pins the fix for a leak found by running the
// binary rather than reading the code.
//
// The CLI never calls slog.SetDefault, so slog's default handler writes to
// stderr. A slog.Warn on the not-found path therefore printed a raw timestamped
// line directly above the real error, telling the user the same thing twice in
// two registers:
//
//	2026/08/25 10:16:19 WARN plugin binary not found command=gateway ...
//	Plugin error: unknown command "gateway" ...
//
// The returned error is the user-facing channel here. Nothing on this path may
// write to the default logger.
func TestProxyCommandDoesNotLogToSlog(t *testing.T) {
	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(restore) })

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads this on Windows

	err := ProxyCommand("definitely-not-a-plugin", nil)
	if err == nil {
		t.Fatal("expected an error for a command with no plugin binary")
	}
	if got := logged.String(); got != "" {
		t.Errorf("proxy wrote to the default logger, which reaches the user's terminal:\n%s", got)
	}
}

// TestReadPluginManifestFindsNestedManifests covers the reason a whole set of
// fallbacks were being taken silently.
//
// Release tarballs carry a leading directory — `<name>/...` for the platform
// archive, `free/<name>/...` for the source one — and extraction keeps it. So
// an installed plugin's manifest is nested, and readPluginManifest only looked
// at the root. It returned nil for every plugin installed from a release, and
// each caller quietly used its nil fallback: unlinkCLIBinary guessed the binary
// name, and pluginOwnsTables assumed tables existed, which made `nself remove`
// demand a database from a plugin that had none.
func TestReadPluginManifestFindsNestedManifests(t *testing.T) {
	const body = `{"name":"demo","pluginType":"cli","binaryName":"nself-demo","tables":[]}`

	for _, layout := range []struct {
		name string
		rel  string
	}{
		{"at the root", "plugin.json"},
		{"platform archive: <name>/", filepath.Join("demo", "plugin.json")},
		{"source archive: free/<name>/", filepath.Join("free", "demo", "plugin.json")},
	} {
		t.Run(layout.name, func(t *testing.T) {
			dir := t.TempDir()
			full := filepath.Join(dir, layout.rel)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}

			m := readPluginManifest(dir)
			if m == nil {
				t.Fatal("manifest not found — callers would silently take their nil fallback")
			}
			if m.BinaryName != "nself-demo" {
				t.Errorf("BinaryName = %q, want nself-demo", m.BinaryName)
			}
			if pluginOwnsTables(m) {
				t.Error("a plugin declaring \"tables\": [] was reported as owning tables, " +
					"which is what made `nself remove` demand a database")
			}
		})
	}
}
