package commands

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// TestUnitTestImageFollowsGoDirective: the unit-phase container image must
// satisfy the plugin's go.mod go directive (D-0657).
func TestUnitTestImageFollowsGoDirective(t *testing.T) {
	cases := []struct {
		name, gomod, want string
	}{
		{"patch version", "module x\n\ngo 1.26.4\n", "golang:1.26-alpine"},
		{"minor only", "module x\n\ngo 1.23\n", "golang:1.23-alpine"},
		{"older", "module x\n\ngo 1.21.0\n\ntoolchain go1.22.1\n", "golang:1.21-alpine"},
		{"trailing comment", "module x\ngo 1.25.0 // pinned\n", "golang:1.25-alpine"},
		{"crlf", "module x\r\ngo 1.23.4\r\n", "golang:1.23-alpine"},
		{"no go line", "module x\n", fallbackGoImage()},
		{"toolchain line only", "module x\ntoolchain go1.24.0\n", fallbackGoImage()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(c.gomod), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := unitTestImage(dir); got != c.want {
				t.Fatalf("unitTestImage = %q, want %q", got, c.want)
			}
		})
	}
	if got := unitTestImage(t.TempDir()); got != fallbackGoImage() {
		t.Fatalf("missing go.mod: got %q, want %q", got, fallbackGoImage())
	}
}

func TestUnitTestDockerArgs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\ngo 1.26.4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args := unitTestDockerArgs(dir)
	for _, want := range []string{"golang:1.26-alpine", "GOTOOLCHAIN=auto", dir + ":/workspace"} {
		if !slices.Contains(args, want) {
			t.Errorf("args %v missing %q", args, want)
		}
	}
	if slices.Contains(args, "golang:1.22-alpine") {
		t.Errorf("stale 1.22 image in %v", args)
	}
}

func TestFallbackGoImageFor(t *testing.T) {
	cases := map[string]string{
		"go1.26.9":             goImage("1.26"),
		"go1.27rc1":            goImage("1.27"),
		"devel go1.28-abc Mon": goImage(fallbackGoMinor),
		"":                     goImage(fallbackGoMinor),
	}
	for in, want := range cases {
		if got := fallbackGoImageFor(in); got != want {
			t.Errorf("fallbackGoImageFor(%q) = %q, want %q", in, got, want)
		}
	}
}
