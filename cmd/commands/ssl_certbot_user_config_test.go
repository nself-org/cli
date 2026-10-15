package commands

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// Purpose: prevent per-user certbot hooks from bypassing the ACME cut-over scan.
// Inputs: isolated HOME and XDG_CONFIG_HOME configs naming a protected ssl dir.
// Outputs: the same file:line diagnostics as certbot's system config.
// Constraints: never writes to the operator's home or certbot tree.
func TestCertbotHooksUserConfigs(t *testing.T) {
	root := isolateCertbotUserConfigs(t)
	home, xdg, sslDir := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	bases := []string{filepath.Join(home, ".config"), xdg}
	if runtime.GOOS != "windows" {
		bases = append(bases, filepath.Join(root, ".config"))
	}
	for _, base := range bases {
		p := filepath.Join(base, "letsencrypt", "cli.ini")
		acmeWriteFile(t, p, []byte("# saved hook\ndeploy-hook = cp fullchain.pem "+sslDir+"/\n"))
	}
	hits, err := certbotHooks(t.TempDir(), sslDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, base := range bases {
		want := filepath.Join(base, "letsencrypt", "cli.ini") + ":2"
		if !slices.Contains(hits, want) {
			t.Errorf("hits = %v, want %s", hits, want)
		}
	}
	// A missing XDG config must leave the default home config protected.
	if err := os.Remove(filepath.Join(xdg, "letsencrypt", "cli.ini")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	if hits, err := certbotHooks(t.TempDir(), sslDir); err != nil || !slices.Contains(hits, filepath.Join(home, ".config", "letsencrypt", "cli.ini")+":2") {
		t.Fatalf("default config: hits=%v err=%v", hits, err)
	}
}

// isolateCertbotUserConfigs keeps hook scans independent of the host's accounts.
func isolateCertbotUserConfigs(t *testing.T) string {
	t.Helper()
	home, root := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	old := certbotLookupUser
	certbotLookupUser = func(uid string) (*user.User, error) {
		if uid != "0" {
			t.Fatalf("lookup uid = %q, want root", uid)
		}
		return &user.User{Uid: "0", HomeDir: root}, nil
	}
	t.Cleanup(func() { certbotLookupUser = old })
	return root
}

func TestCertbotHooksUserConfigErrors(t *testing.T) {
	root := isolateCertbotUserConfigs(t)
	if runtime.GOOS == "windows" {
		t.Skip("root account is POSIX only")
	}
	wantErr := errors.New("root account lookup failed")
	lookup := certbotLookupUser
	certbotLookupUser = func(string) (*user.User, error) { return nil, wantErr }
	if _, err := certbotHooks(t.TempDir(), t.TempDir()); !errors.Is(err, wantErr) {
		t.Fatalf("lookup error = %v, want %v", err, wantErr)
	}
	certbotLookupUser = lookup
	// ENOTDIR must refuse too, even when running the tests as root.
	acmeWriteFile(t, filepath.Join(root, ".config"), []byte("not a directory"))
	if _, err := certbotHooks(t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("unreadable root config accepted")
	}
}

func TestCertbotHooksUserConfigDeduplicates(t *testing.T) {
	root := isolateCertbotUserConfigs(t)
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	xdg := filepath.Join(root, ".config")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	dir, sslDir := filepath.Join(xdg, "letsencrypt"), t.TempDir()
	p := filepath.Join(dir, "cli.ini")
	acmeWriteFile(t, p, []byte("post_hook = cp cert.pem "+sslDir+"/\n"))
	if hits, err := certbotHooks(dir, sslDir); err != nil || len(hits) != 1 || hits[0] != p+":1" {
		t.Fatalf("duplicate configs: hits=%v err=%v", hits, err)
	}
}
