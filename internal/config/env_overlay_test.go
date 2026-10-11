package config

// env_overlay_test.go — LoadOptions.EnvOverlay (P7-LIVE-05): an overlaid
// cascade file is read from the given bytes, never from disk, with the same
// checks and the same parser as the disk path.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// overlayProject writes a minimal project .env and isolates the process env.
func overlayProject(t *testing.T, dotenv string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(dotenv), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"ENV", "BASE_DOMAIN", "PROJECT_NAME", "LIVE05_ONLY"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	return dir
}

func TestLoadEnvOverlayReplacesDiskFile(t *testing.T) {
	dir := overlayProject(t, "PROJECT_NAME=ov\nBASE_DOMAIN=disk.test\n")
	cfg, err := LoadWithOptions(dir, LoadOptions{EnvOverlay: map[string][]byte{
		".env": []byte("PROJECT_NAME=ov\nBASE_DOMAIN=pending.test\n"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseDomain != "pending.test" {
		t.Fatalf("BaseDomain = %q, want the overlaid value", cfg.BaseDomain)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".env")); !strings.Contains(string(b), "disk.test") {
		t.Fatal("the overlay must not touch the file")
	}
}

func TestLoadEnvOverlayAddsMissingLayer(t *testing.T) {
	dir := overlayProject(t, "PROJECT_NAME=ov\n")
	if _, err := LoadWithOptions(dir, LoadOptions{EnvOverlay: map[string][]byte{
		".env.secrets": []byte("LIVE05_ONLY=from-overlay\n"),
	}}); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("LIVE05_ONLY"); got != "from-overlay" {
		t.Fatalf("LIVE05_ONLY = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.secrets")); !os.IsNotExist(err) {
		t.Fatal("the overlay created a file")
	}
}

func TestLoadEnvOverlaySelectsEnvLayer(t *testing.T) {
	dir := overlayProject(t, "PROJECT_NAME=ov\nENV=prod\n")
	if err := os.WriteFile(filepath.Join(dir, ".env.dev"), []byte("BASE_DOMAIN=dev.test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadWithOptions(dir, LoadOptions{EnvOverlay: map[string][]byte{".env": []byte("PROJECT_NAME=ov\nENV=dev\n")}})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseDomain != "dev.test" {
		t.Fatalf("the overlaid ENV did not select .env.dev: BaseDomain %q", cfg.BaseDomain)
	}
}

func TestResolveEnvWith(t *testing.T) {
	dir := overlayProject(t, "ENV=prod\n")
	if v, src := resolveEnvWith(dir, nil); v != "prod" || src != ".env" {
		t.Fatalf("no overlay: %s %s", v, src)
	}
	if v, src := resolveEnvWith(dir, map[string][]byte{".env": []byte("ENV=staging\n")}); v != "staging" || src != ".env" {
		t.Fatalf("overlay: %s %s", v, src)
	}
	if v, src := resolveEnvWith(dir, map[string][]byte{".env": []byte("A=1\n")}); v != "dev" || src != "default" {
		t.Fatalf("overlay without ENV: %s %s", v, src)
	}
	t.Setenv("ENV", "dev")
	if v, src := resolveEnvWith(dir, map[string][]byte{".env": []byte("ENV=staging\n")}); v != "dev" || src != "process environment" {
		t.Fatalf("process ENV must win: %s %s", v, src)
	}
}

func TestLoadEnvOverlayChecks(t *testing.T) {
	dir := overlayProject(t, "PROJECT_NAME=ov\n")
	for name, data := range map[string][]byte{
		"null byte": []byte("A=1\x00\n"),
		"too large": []byte("A=" + strings.Repeat("x", maxEnvFileSize) + "\n"),
		"bad parse": []byte("A=\"unterminated\n"),
	} {
		if _, err := LoadWithOptions(dir, LoadOptions{EnvOverlay: map[string][]byte{".env": data}}); err == nil {
			t.Errorf("%s: an invalid overlay was accepted", name)
		}
	}
}
