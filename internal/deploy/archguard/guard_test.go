package archguard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
)

func TestArchGuardCheck(t *testing.T) {
	images := []ImageRef{{Service: "api", Ref: "example.test/api:1"}, {Service: "custom", Ref: "ignored", Build: true}}
	lookup := func(context.Context, string) (map[string]string, error) {
		return map[string]string{"linux/amd64": "sha256:a"}, nil
	}
	m, err := Check(context.Background(), images, "arm64", lookup)
	if err != nil || len(m) != 1 || m[0].Image != images[0].Ref || !strings.Contains(m[0].Reason, "arm64") {
		t.Fatalf("arm64 mismatch = %+v, %v", m, err)
	}
	m, err = Check(context.Background(), images, "amd64", lookup)
	if err != nil || len(m) != 0 {
		t.Fatalf("amd64 result = %+v, %v", m, err)
	}
	multiArch := func(context.Context, string) (map[string]string, error) {
		return map[string]string{"linux/amd64": "sha256:a", "linux/arm64": "sha256:b"}, nil
	}
	m, err = Check(context.Background(), images, "arm64", multiArch)
	if err != nil || len(m) != 0 {
		t.Fatalf("multi-arch result = %+v, %v", m, err)
	}
}

func TestArchGuardLockFirst(t *testing.T) {
	locked := compose.LockedImages()[0]
	calls := 0
	lookup := func(context.Context, string) (map[string]string, error) {
		calls++
		return nil, errors.New("registry called")
	}
	_, err := Check(context.Background(), []ImageRef{{Ref: locked.String()}, {Ref: locked.String()}}, "amd64", lookup)
	if err != nil || calls != 0 {
		t.Fatalf("lock-first lookup: calls=%d err=%v", calls, err)
	}
}

func TestArchGuardRegistryError(t *testing.T) {
	calls := 0
	lookup := func(context.Context, string) (map[string]string, error) {
		calls++
		return nil, errors.New("registry unavailable")
	}
	m, err := Check(context.Background(), []ImageRef{{Ref: "example.test/bad:1"}, {Ref: "example.test/bad:1"}}, "arm64", lookup)
	if err != nil || len(m) != 2 || calls != 1 || !strings.Contains(m[0].Reason, "registry unavailable") {
		t.Fatalf("registry error = %+v, calls=%d, err=%v", m, calls, err)
	}
}

func TestVerboseSinglePlatform(t *testing.T) {
	raw := []byte(`{"Descriptor":{"digest":"sha256:abc","platform":{"os":"linux","architecture":"amd64"}}}`)
	platforms, err := parseVerbosePlatforms(raw)
	if err != nil || platforms["linux/amd64"] != "sha256:abc" || platforms["linux/arm64"] != "" {
		t.Fatalf("verbose platforms = %v, %v", platforms, err)
	}
}

func TestRegistryLookupVerboseFallback(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell fixture")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "docker.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + log + "'\n" +
		"if [ \"$1\" = buildx ]; then exit 1; fi\n" +
		"if [ \"$3\" = --verbose ]; then printf '%s\\n' '{\"Descriptor\":{\"digest\":\"sha256:abc\",\"platform\":{\"os\":\"linux\",\"architecture\":\"amd64\"}}}'; else printf '{}\\n'; fi\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	platforms, err := RegistryLookup(context.Background(), "example.test/single:1")
	if err != nil || platforms["linux/amd64"] != "sha256:abc" {
		t.Fatalf("registry fallback = %v, %v", platforms, err)
	}
	commands, err := os.ReadFile(log)
	if err != nil || strings.Contains(string(commands), "pull") || strings.Count(string(commands), "manifest inspect") != 2 {
		t.Fatalf("docker calls = %q, %v", commands, err)
	}
}
