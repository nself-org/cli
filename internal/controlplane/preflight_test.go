package controlplane

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

type fixedArch string

func (f fixedArch) Architecture(context.Context, Server) (string, error) { return string(f), nil }

func TestPreflightRefusesBeforeDeploy(t *testing.T) {
	file := filepath.Join(t.TempDir(), "docker-compose.yml")
	if err := os.WriteFile(file, []byte("services:\n  api:\n    image: example.test/amd64-only:1\n  built:\n    image: ignored\n    build: .\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	inv := &Inventory{Environments: map[string]Environment{"qa": {Name: "qa", Kind: "remote", Servers: []Server{{Name: "arm-host", Host: "u@arm.example.test", Role: RoleApp}}}}}
	lookups := 0
	err := preflightWith(context.Background(), inv, "qa", []string{file}, fixedArch("arm64"), func(context.Context, string) (map[string]string, error) {
		lookups++
		return map[string]string{"linux/amd64": "sha256:a"}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "E491") || !strings.Contains(err.Error(), "arm-host") || !strings.Contains(err.Error(), "example.test/amd64-only:1") || lookups != 1 {
		t.Fatalf("preflight = %v; lookups=%d", err, lookups)
	}
}

// TestPreflightComposeOverrideKeepsImage proves an override fragment without
// image or build keeps the base image, as compose merges, instead of a false E491.
func TestPreflightComposeOverrideKeepsImage(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "docker-compose.yml")
	override := filepath.Join(dir, "docker-compose.override.yml")
	if err := os.WriteFile(base, []byte("services:\n  api:\n    image: example.test/multi:1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(override, []byte("services:\n  api:\n    environment:\n      A: b\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	inv := &Inventory{Environments: map[string]Environment{"qa": {Name: "qa", Kind: "remote", Servers: []Server{{Name: "arm-host", Host: "u@arm.example.test", Role: RoleApp}}}}}
	lookup := func(_ context.Context, ref string) (map[string]string, error) {
		if ref != "example.test/multi:1" {
			t.Fatalf("lookup ref = %q, want the base image", ref)
		}
		return map[string]string{"linux/amd64": "sha256:a", "linux/arm64": "sha256:b"}, nil
	}
	if err := preflightWith(context.Background(), inv, "qa", []string{base, override}, fixedArch("arm64"), lookup); err != nil {
		t.Fatalf("preflight with override = %v; want pass", err)
	}
	amdOnly := func(context.Context, string) (map[string]string, error) {
		return map[string]string{"linux/amd64": "sha256:a"}, nil
	}
	if err := preflightWith(context.Background(), inv, "qa", []string{base, override}, fixedArch("arm64"), amdOnly); err == nil || !strings.Contains(err.Error(), "E491") {
		t.Fatalf("preflight amd64-only with override = %v; want E491", err)
	}
}

func TestProbeArchitectureCached(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell fixture")
	}
	// v1.4 path: the probe runs without a pinned host key and is cached.
	compattest.Set(t, false)
	dir := t.TempDir()
	log := filepath.Join(dir, "uname.log")
	script := "#!/bin/sh\nprintf 'uname\\n' >> '" + log + "'\nprintf 'aarch64\\n'\n"
	if err := os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("TEST_ARCH_KEY", filepath.Join(dir, "key"))
	prober := NewSSHProber(dir, false)
	server := Server{Name: "arm-host", Host: "u@arm.example.test", SSHKeyRef: "TEST_ARCH_KEY"}
	for i := 0; i < 2; i++ {
		arch, err := prober.Architecture(context.Background(), server)
		if err != nil || arch != "arm64" {
			t.Fatalf("architecture = %q, %v", arch, err)
		}
	}
	lines, err := os.ReadFile(log)
	if err != nil || strings.Count(string(lines), "uname") != 1 {
		t.Fatalf("uname calls = %q, %v", lines, err)
	}
}

// TestProbeArchitectureStrictHostKey proves v1.5 refuses to probe a host whose
// key is not enrolled (ADR 0021) instead of guessing an architecture.
func TestProbeArchitectureStrictHostKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell fixture")
	}
	compattest.Set(t, true)
	dir := t.TempDir()
	log := filepath.Join(dir, "uname.log")
	for _, name := range []string{"ssh", "ssh-keyscan"} {
		script := "#!/bin/sh\nprintf '" + name + "\\n' >> '" + log + "'\nexit 1\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("TEST_ARCH_KEY", filepath.Join(dir, "key"))
	prober := NewSSHProber(dir, false)
	server := Server{Name: "arm-host", Host: "u@arm.example.test", SSHKeyRef: "TEST_ARCH_KEY"}
	arch, err := prober.Architecture(context.Background(), server)
	if err == nil || arch != "" || !strings.Contains(err.Error(), "E487") {
		t.Fatalf("architecture = %q, %v; want E487 refusal", arch, err)
	}
	if lines, _ := os.ReadFile(log); strings.Contains(string(lines), "ssh\n") && !strings.Contains(string(lines), "ssh-keyscan") {
		t.Fatalf("probe ran ssh without a host-key check: %q", lines)
	}
}
