package mount

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func fixture(t testing.TB, dir, slug, command string) {
	t.Helper()
	d := map[string]any{
		"manifest_version": 2, "name": slug, "version": "1.0.0", "description": "fixture",
		"license": "free", "licenseType": "free", "pluginType": "cli", "category": "infrastructure",
		"status": "stable", "tier": "free", "maturity": "implemented", "minNselfVersion": "1.5.0",
		"service": map[string]any{"kind": "cli"}, "requires": map[string]any{"nself": ">=1.5.0"},
		"binaryName": "nself-" + slug,
		"commands": map[string]any{"command": command, "binary": "nself-" + slug,
			"subcommands": []any{map[string]any{"name": "sub", "side_effect": "write"}, map[string]any{"name": "nested two"}}},
	}
	p := filepath.Join(dir, slug)
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(d)
	if err := os.WriteFile(filepath.Join(p, "plugin.json"), b, 0644); err != nil {
		t.Fatal(err)
	}
}

func binary(t testing.TB, dir, slug string) {
	t.Helper()
	p := filepath.Join(dir, "bin")
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "nself-"+slug), []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestDiscover(t *testing.T) {
	d := t.TempDir()
	fixture(t, d, "demo", "demo")
	binary(t, d, "demo")
	s, p := Discover(d, []string{"start"})
	if len(p) != 0 || len(s) != 1 || s[0].Command != "demo" || len(s[0].Subcommands) != 2 || strings.Join(s[0].Subcommands[1].Path, " ") != "nested two" {
		t.Fatalf("specs=%+v problems=%+v", s, p)
	}
}

func TestDiscoverV1QuietInV15(t *testing.T) {
	if dir := os.Getenv("NSELF_MOUNT_V1_CHILD"); dir != "" {
		specs, problems := Discover(dir, nil)
		if len(specs) != 1 || len(problems) != 0 {
			t.Fatalf("specs=%+v problems=%+v", specs, problems)
		}
		return
	}
	d := t.TempDir()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "cmd", "commands", "testdata", "mount", "claw", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(d, "claw"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "claw", "plugin.json"), b, 0644); err != nil {
		t.Fatal(err)
	}
	binary(t, d, "claw")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^TestDiscoverV1QuietInV15$")
	cmd.Env = append(os.Environ(), "NSELF_V15=1", "NSELF_MOUNT_V1_CHILD="+d)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("child: %v: %s", err, output)
	}
	if string(output) != "PASS\n" {
		t.Fatalf("child output=%q", output)
	}
}

func TestDiscoverRejects(t *testing.T) {
	d := t.TempDir()
	fixture(t, d, "start", "start")
	binary(t, d, "start")
	fixture(t, d, "one", "dup")
	binary(t, d, "one")
	fixture(t, d, "two", "dup")
	binary(t, d, "two")
	fixture(t, d, "missing", "missing")
	fixture(t, d, "disabled", "disabled")
	binary(t, d, "disabled")
	if err := os.WriteFile(filepath.Join(d, "disabled", ".disabled"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	s, p := Discover(d, []string{"start"})
	if len(s) != 0 || len(p) != 3 {
		t.Fatalf("specs=%+v problems=%+v", s, p)
	}
	got := map[string]bool{}
	for _, x := range p {
		got[x.Code] = true
	}
	if !got["E113"] || !got["E405"] || !got["E406"] {
		t.Fatalf("codes=%v", got)
	}
}

func TestDiscoverEscapeSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges")
	}
	d := t.TempDir()
	fixture(t, d, "escape", "escape")
	out := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(out, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(d, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out, filepath.Join(d, "bin", "nself-escape")); err != nil {
		t.Fatal(err)
	}
	s, p := Discover(d, nil)
	if len(s) != 0 || len(p) != 1 || p[0].Code != "E406" {
		t.Fatalf("specs=%+v problems=%+v", s, p)
	}
}

func TestDiscoverNoCanonLoad(t *testing.T) {
	d := t.TempDir()
	fixture(t, d, "demo", "demo")
	binary(t, d, "demo")
	s, p := Discover(d, []string{"demo"})
	if len(s) != 0 || len(p) != 1 || p[0].Code != "E405" {
		t.Fatalf("specs=%+v problems=%+v", s, p)
	}
}

func BenchmarkDiscover(b *testing.B) {
	d := b.TempDir()
	for i := 0; i < 50; i++ {
		slug := "p" + strconv.Itoa(i)
		fixture(b, d, slug, slug)
		binary(b, d, slug)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s, p := Discover(d, nil)
		if len(s) != 50 || len(p) != 0 {
			b.Fatalf("%d specs, %d problems", len(s), len(p))
		}
	}
}
