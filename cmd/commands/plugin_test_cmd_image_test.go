package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// imageGoVersion extracts major and minor from a golang:<major>.<minor>-alpine tag.
func imageGoVersion(t *testing.T, image string) (int, int) {
	t.Helper()
	m := regexp.MustCompile(`^golang:(\d+)\.(\d+)-alpine$`).FindStringSubmatch(image)
	if m == nil {
		t.Fatalf("image %q is not golang:<major>.<minor>-alpine", image)
	}
	maj, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	return maj, min
}

// Official golang images set GOTOOLCHAIN=local, so the image must be at least
// as new as the module's go directive (D-0657).
func TestPluginUnitTestImageSatisfiesGoDirective(t *testing.T) {
	cases := []struct {
		name  string
		gomod string
		want  string
	}{
		{"go 1.25.0", "module x\n\ngo 1.25.0\n", "golang:1.25-alpine"},
		{"go 1.26.4", "module x\n\ngo 1.26.4\n", "golang:1.26-alpine"},
		{"two-part go line", "module x\n\ngo 1.21\n", "golang:1.21-alpine"},
		{"toolchain newer than go", "module x\n\ngo 1.23.0\ntoolchain go1.24.2\n", "golang:1.24-alpine"},
		{"toolchain older than go", "module x\n\ngo 1.25.0\ntoolchain go1.23.1\n", "golang:1.25-alpine"},
		{"toolchain default", "module x\n\ngo 1.25.0\ntoolchain default\n", "golang:1.25-alpine"},
		{"comment on go line", "module x\n\ngo 1.25.0 // min\n", "golang:1.25-alpine"},
		{"no go line", "module x\n", pluginTestFallbackImage},
		{"unparseable go line", "module x\n\ngo banana\n", pluginTestFallbackImage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(tc.gomod), 0o644); err != nil {
				t.Fatal(err)
			}
			got := pluginUnitTestImage(dir)
			if got != tc.want {
				t.Fatalf("pluginUnitTestImage = %q, want %q", got, tc.want)
			}
			imageGoVersion(t, got)
		})
	}
}

func TestPluginUnitTestImageMissingGoMod(t *testing.T) {
	got := pluginUnitTestImage(t.TempDir())
	if got != pluginTestFallbackImage {
		t.Fatalf("got %q, want fallback %q", got, pluginTestFallbackImage)
	}
	maj, min := imageGoVersion(t, got)
	if maj != 1 || min < 26 {
		t.Fatalf("fallback %s must be Go 1.26 or newer, free plugins declare up to go 1.26.4", fmt.Sprint(got))
	}
}

func TestPluginUnitTestDockerCmdUsesDerivedImage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args := pluginUnitTestDockerCmd(dir).Args
	found := false
	for _, a := range args {
		if a == "golang:1.25-alpine" {
			found = true
		}
		if a == "golang:1.22-alpine" {
			t.Fatalf("hard-coded golang:1.22-alpine still in args: %v", args)
		}
	}
	if !found {
		t.Fatalf("docker args %v do not use golang:1.25-alpine", args)
	}
}
