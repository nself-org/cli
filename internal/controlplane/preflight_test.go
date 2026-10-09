package controlplane

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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

func TestProbeArchitectureCached(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell fixture")
	}
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
