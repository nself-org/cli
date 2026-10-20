package commands

// ci_bootstrap_test.go — Guards the nself-ci gate bootstrap (P7-CANON-24).
//
// Purpose: pin the resolution order (published plugin binary, adjacent dev
//          build, verified auto-install), the legacy-cache handling, and that
//          neither PATH nor a Go toolchain is ever used to obtain the gate.
// Inputs:  a temporary HOME, a loopback fixture registry, stub `go` and
//          `nself-ci` scripts first on PATH that record any execution.
// Outputs: assertions on ensureCIBinary and retireLegacyCICache.
// Constraints: no real network. Signing uses generated keys swapped into
//          signature.Keys by signaturetest.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
)

// setHome points os.UserHomeDir at a temp dir on every platform. HOME alone is
// not enough: on Windows os.UserHomeDir reads %USERPROFILE%, so a test that
// sets only HOME silently uses the runner's real home directory.
func setHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	return dir
}

// cbEnv isolates one bootstrap test: a clean HOME, a CLI executable with no
// adjacent plugins checkout, a fresh once-per-process notice captured in the
// returned buffer. The package vars it swaps are restored on cleanup.
func cbEnv(t *testing.T) (home string, notice *bytes.Buffer) {
	t.Helper()
	home = setHome(t)
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	t.Setenv("NSELF_SKIP_SBOM_CHECK", "1")
	exe := filepath.Join(t.TempDir(), "cli", "nself")
	if err := os.MkdirAll(filepath.Dir(exe), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("x"), 0o700); err != nil {
		t.Fatal(err)
	}
	notice = &bytes.Buffer{}
	oldExe, oldOut := ciExecutable, ciNoticeOut
	ciExecutable = func() (string, error) { return exe, nil }
	ciNoticeOut = notice
	ciLegacyOnce = sync.Once{}
	t.Cleanup(func() { ciExecutable, ciNoticeOut, ciLegacyOnce = oldExe, oldOut, sync.Once{} })
	return home, notice
}

// cbStubs puts recording `go` and `nself-ci` stubs first on PATH and returns
// the log file they append to. Any line in it means a stub was executed.
func cbStubs(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stubs")
	}
	dir, log := t.TempDir(), filepath.Join(t.TempDir(), "ran.log")
	for _, n := range []string{"go", "nself-ci"} {
		body := "#!/bin/sh\necho " + n + " \"$@\" >> '" + log + "'\nexit 0\n"
		if err := os.WriteFile(filepath.Join(dir, n), []byte(body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func cbAssertNoStubRan(t *testing.T, log string) {
	t.Helper()
	if b, err := os.ReadFile(log); err == nil {
		t.Fatalf("a PATH stub was executed:\n%s", b)
	}
}

// cbOpts selects what cbRegistry signs: the source release, the platform
// package, and whether the served platform bytes differ from the catalogue.
type cbOpts struct {
	sign, platSign, platTamper bool
}

// cbRegistry serves a one-entry free registry for ci with a source package and
// a binary package for this platform.
func cbRegistry(t *testing.T, o cbOpts) {
	t.Helper()
	plat, err := plugin.PlatformArch()
	if err != nil {
		t.Skip("unsupported platform")
	}
	src := aiTarball(t, nil)
	bin := aiTarball(t, map[string]string{"README": "binary package"})
	srcSum, binSum := aiSum(src), aiSum(bin)
	served := bin
	if o.platTamper {
		served = aiTarball(t, map[string]string{"README": "tampered"})
	}
	var entry map[string]any // set below, before the first request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "-"+plat+".tar.gz"):
			_, _ = w.Write(served)
		case strings.HasSuffix(r.URL.Path, "/tarball"):
			_, _ = w.Write(src)
		case strings.HasSuffix(r.URL.Path, ".tar.gz"):
			http.NotFound(w, r)
		default:
			body, _ := json.Marshal(map[string]any{"plugins": []any{entry}})
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}
	}))
	entry = map[string]any{
		"name": aiSlug, "version": aiVersion, "description": "fixture", "category": "util", "tier": "free",
		"license": "MIT", "status": "stable", "checksum": srcSum, "pluginType": "cli", "binaryName": "nself-ci",
		"repository": srv.URL, "checksums": map[string]any{"sha256": srcSum, "platforms": map[string]string{plat: binSum}},
	}
	if o.sign {
		key, signer := signaturetest.NewKey(t, signature.TierFree)
		signaturetest.UseKeys(t, []signature.Key{key}, nil)
		signaturetest.UseFloor(t, map[string]map[string]string{})
		rs := signaturetest.Sign(t, signer, aiSlug, aiVersion, srcSum)
		entry["release_signature"] = map[string]string{"key_id": rs.KeyID, "alg": rs.Alg, "sig": rs.Sig}
		if o.platSign {
			ps := signaturetest.SignPlatform(t, signer, aiSlug, aiVersion, plat, binSum)
			entry["platform_signatures"] = map[string]any{plat: map[string]string{"key_id": ps.KeyID, "alg": ps.Alg, "sig": ps.Sig}}
		}
	}
	t.Cleanup(srv.Close)
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
}

func TestCIBootstrapOrder(t *testing.T) {
	t.Run("published plugin binary wins over PATH", func(t *testing.T) {
		cbEnv(t)
		log := cbStubs(t)
		pub := plugin.PublishedBinaryPath("nself-ci")
		if err := os.MkdirAll(filepath.Dir(pub), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pub, []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		got, err := ensureCIBinary(false)
		if err != nil || got != pub {
			t.Fatalf("got %q, %v; want %q", got, err, pub)
		}
		cbAssertNoStubRan(t, log)
	})

	t.Run("adjacent plugins checkout dev build is unchanged", func(t *testing.T) {
		cbEnv(t)
		log := cbStubs(t)
		exe, _ := ciExecutable()
		exe, _ = filepath.EvalSymlinks(exe)
		dir := filepath.Join(filepath.Dir(exe), "..", "plugins", "free", "ci")
		for _, f := range []string{filepath.Join(dir, "cmd", "main.go"), filepath.Join(dir, "nself-ci")} {
			if err := os.MkdirAll(filepath.Dir(f), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(f, []byte("x"), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		old, newer := time.Now().Add(-time.Hour), time.Now()
		_ = os.Chtimes(filepath.Join(dir, "cmd", "main.go"), old, old)
		_ = os.Chtimes(filepath.Join(dir, "nself-ci"), newer, newer)
		got, err := ensureCIBinary(false)
		want, _ := filepath.Abs(filepath.Join(dir, "nself-ci"))
		if err != nil || got != want {
			t.Fatalf("got %q, %v; want %q", got, err, want)
		}
		cbAssertNoStubRan(t, log) // an up-to-date dev binary needs no go build
	})
}

func TestCIBootstrapOrderLegacyAndLinks(t *testing.T) {
	const notice = "notice: ~/.nself/bin/nself-ci is no longer used (unverified); nself now installs the signed ci plugin\n"
	t.Run("legacy file is retired once even when the plugin is installed", func(t *testing.T) {
		home, out := cbEnv(t)
		pub := plugin.PublishedBinaryPath("nself-ci")
		legacy := filepath.Join(home, ".nself", "bin", "nself-ci")
		for _, f := range []string{pub, legacy} {
			if err := os.MkdirAll(filepath.Dir(f), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(f, []byte("#!/bin/sh\n"), 0o700); err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < 3; i++ {
			if got, err := ensureCIBinary(false); err != nil || got != pub {
				t.Fatalf("call %d: %q, %v", i, got, err)
			}
		}
		if _, err := os.Lstat(legacy + ".unverified-legacy"); err != nil {
			t.Fatalf("legacy file not set aside: %v", err)
		}
		if out.String() != notice {
			t.Fatalf("want exactly one notice over three calls, got %q", out.String())
		}
	})

	t.Run("the notice prints once across calls while the legacy path stays", func(t *testing.T) {
		home, out := cbEnv(t)
		pub := plugin.PublishedBinaryPath("nself-ci")
		if err := os.MkdirAll(filepath.Dir(pub), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pub, []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(home, ".nself", "bin", "nself-ci"), 0o750); err != nil {
			t.Fatal(err) // a directory is never moved, so every call would see it again
		}
		for i := 0; i < 3; i++ {
			if _, err := ensureCIBinary(false); err != nil {
				t.Fatal(err)
			}
		}
		if out.String() != notice {
			t.Fatalf("want exactly one notice over three calls, got %q", out.String())
		}
	})

	t.Run("no home directory: nothing is trusted or installed", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("USERPROFILE semantics differ")
		}
		cbEnv(t)
		log := cbStubs(t)
		t.Setenv("HOME", "")
		t.Setenv("USERPROFILE", "")
		got, err := ensureCIBinary(false)
		if err == nil || !strings.Contains(err.Error(), "nself add ci") {
			t.Fatalf("got %q, %v; want a refusal naming `nself add ci`", got, err)
		}
		cbAssertNoStubRan(t, log)
	})

	t.Run("a symlink at the published path is not trusted", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks")
		}
		cbEnv(t)
		log := cbStubs(t)
		t.Setenv("NSELF_PLUGIN_REGISTRY", "http://127.0.0.1:1") // nothing to install from
		target := filepath.Join(t.TempDir(), "planted")
		if err := os.WriteFile(target, []byte("#!/bin/sh\necho ran >> '"+log+"'\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		pub := plugin.PublishedBinaryPath("nself-ci")
		if err := os.MkdirAll(filepath.Dir(pub), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, pub); err != nil {
			t.Fatal(err)
		}
		if got, err := ensureCIBinary(false); err == nil {
			t.Fatalf("a symlinked binary was returned: %q", got)
		}
		cbAssertNoStubRan(t, log)
	})
}

func TestCIBootstrapLegacyCache(t *testing.T) {
	const notice = "notice: ~/.nself/bin/nself-ci is no longer used (unverified); nself now installs the signed ci plugin\n"
	setup := func(t *testing.T) (string, string, *bytes.Buffer) {
		home, out := cbEnv(t)
		bin := filepath.Join(home, ".nself", "bin")
		if err := os.MkdirAll(bin, 0o750); err != nil {
			t.Fatal(err)
		}
		return bin, filepath.Join(bin, "nself-ci"), out
	}

	t.Run("regular file is renamed once, content unchanged", func(t *testing.T) {
		bin, legacy, out := setup(t)
		if err := os.WriteFile(legacy, []byte("legacy build"), 0o700); err != nil {
			t.Fatal(err)
		}
		retireLegacyCICache()
		retireLegacyCICache()
		if _, err := os.Lstat(legacy); !os.IsNotExist(err) {
			t.Fatalf("legacy still in place: %v", err)
		}
		b, err := os.ReadFile(filepath.Join(bin, "nself-ci.unverified-legacy"))
		if err != nil || string(b) != "legacy build" {
			t.Fatalf("aside copy: %q, %v", b, err)
		}
		if out.String() != notice {
			t.Fatalf("notice = %q, want one line", out.String())
		}
	})

	t.Run("an earlier aside copy is never overwritten", func(t *testing.T) {
		bin, legacy, out := setup(t)
		aside := filepath.Join(bin, "nself-ci.unverified-legacy")
		_ = os.WriteFile(aside, []byte("older"), 0o600)
		_ = os.WriteFile(legacy, []byte("newer"), 0o700)
		retireLegacyCICache()
		if b, _ := os.ReadFile(aside); string(b) != "older" {
			t.Fatalf("aside overwritten: %q", b)
		}
		if b, _ := os.ReadFile(legacy); string(b) != "newer" || out.String() != notice {
			t.Fatalf("legacy file changed or notice missing: %q %q", b, out.String())
		}
	})

	t.Run("symlink and directory are left untouched", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks")
		}
		bin, legacy, out := setup(t)
		target := filepath.Join(t.TempDir(), "real")
		_ = os.WriteFile(target, []byte("x"), 0o700)
		if err := os.Symlink(target, legacy); err != nil {
			t.Fatal(err)
		}
		retireLegacyCICache()
		if fi, err := os.Lstat(legacy); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("symlink changed: %v", err)
		}
		if out.String() != notice {
			t.Fatalf("symlink notice = %q", out.String())
		}

		_ = os.Remove(legacy)
		ciLegacyOnce, out = sync.Once{}, &bytes.Buffer{}
		ciNoticeOut = out
		if err := os.MkdirAll(filepath.Join(legacy, "keep"), 0o750); err != nil {
			t.Fatal(err)
		}
		retireLegacyCICache()
		if fi, err := os.Stat(filepath.Join(legacy, "keep")); err != nil || !fi.IsDir() {
			t.Fatalf("directory changed: %v", err)
		}
		if _, err := os.Lstat(filepath.Join(bin, "nself-ci.unverified-legacy")); err == nil {
			t.Fatal("a directory was renamed")
		}
		if out.String() != notice {
			t.Fatalf("directory notice = %q", out.String())
		}
	})

	t.Run("read-only bin dir: file stays, notice printed, no error", func(t *testing.T) {
		if runtime.GOOS == "windows" || os.Geteuid() == 0 {
			t.Skip("needs unix permissions and a non-root user")
		}
		bin, legacy, out := setup(t)
		_ = os.WriteFile(legacy, []byte("legacy"), 0o700)
		if err := os.Chmod(bin, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(bin, 0o700) })
		retireLegacyCICache()
		if _, err := os.Lstat(legacy); err != nil {
			t.Fatalf("legacy gone: %v", err)
		}
		if out.String() != notice {
			t.Fatalf("notice = %q", out.String())
		}
	})

	t.Run("nothing there: no notice", func(t *testing.T) {
		_, _, out := setup(t)
		retireLegacyCICache()
		if out.Len() != 0 {
			t.Fatalf("notice without a legacy file: %q", out.String())
		}
	})
}

func TestCIBootstrapSigned(t *testing.T) {
	t.Run("signed binary package installs and its path is returned", func(t *testing.T) {
		cbEnv(t)
		cbRegistry(t, cbOpts{sign: true, platSign: true})
		got, err := ensureCIBinary(false)
		if err != nil {
			t.Fatalf("ensureCIBinary: %v", err)
		}
		if got != plugin.PublishedBinaryPath("nself-ci") {
			t.Fatalf("path %q", got)
		}
		if fi, err := os.Stat(got); err != nil || !fi.Mode().IsRegular() {
			t.Fatalf("binary missing: %v", err)
		}
	})

	t.Run("source signature alone is refused with E121", func(t *testing.T) {
		cbEnv(t)
		cbRegistry(t, cbOpts{sign: true})
		_, err := ensureCIBinary(false)
		if err == nil || aiCode(err) != "E121" {
			t.Fatalf("want E121, got %v (code %q)", err, aiCode(err))
		}
		if _, serr := os.Stat(plugin.PublishedBinaryPath("nself-ci")); serr == nil {
			t.Fatal("an unverified binary was published")
		}
	})
}

func TestCIBootstrapNoGo(t *testing.T) {
	cases := map[string]struct {
		o    cbOpts
		code string
	}{
		"unsigned release":  {cbOpts{}, "E107"},
		"tampered platform": {cbOpts{sign: true, platSign: true, platTamper: true}, "E124"},
	}
	for name, tc := range cases {
		o := tc.o
		t.Run(name, func(t *testing.T) {
			cbEnv(t)
			log := cbStubs(t)
			cbRegistry(t, o)
			got, err := ensureCIBinary(false)
			if err == nil {
				t.Fatalf("a %s was accepted: %q", name, got)
			}
			if got := aiCode(err); got != tc.code {
				t.Fatalf("verification error code %q, want %q (%v)", got, tc.code, err)
			}
			if !strings.Contains(err.Error(), "nself add ci") {
				t.Fatalf("error does not name `nself add ci`: %v", err)
			}
			if _, serr := os.Stat(plugin.PublishedBinaryPath("nself-ci")); serr == nil {
				t.Fatal("a refused release was published")
			}
			cbAssertNoStubRan(t, log)
		})
	}
}
