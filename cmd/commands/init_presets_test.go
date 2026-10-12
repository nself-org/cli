package commands

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestInitPresetsRegistry(t *testing.T) {
	expected := []string{
		"b2b-saas",
		"mobile-backend",
		"ai-assistant",
		"community-forum",
		"media-hosting",
		"dev",
		"nclaw-app",
	}
	for _, name := range expected {
		p, ok := initPresets[name]
		if !ok {
			t.Errorf("preset %q missing from initPresets registry", name)
			continue
		}
		if p.Name != name {
			t.Errorf("preset %q: Name field = %q, want %q", name, p.Name, name)
		}
		if p.Description == "" {
			t.Errorf("preset %q: Description is empty", name)
		}
		if len(p.Notes) == 0 {
			t.Errorf("preset %q: Notes is empty — every preset needs at least one next-step", name)
		}
	}
}

func TestDevPreset(t *testing.T) {
	p, ok := initPresets["dev"]
	if !ok {
		t.Fatal("dev preset missing")
	}

	// dev preset must have no enabled services (lightweight / fast cold-start).
	if len(p.EnabledServices) != 0 {
		t.Errorf("dev preset: EnabledServices should be empty for fast cold-start, got %v", p.EnabledServices)
	}

	// dev preset must have no suggested plugins (no paid plugins required).
	if len(p.SuggestedPlugins) != 0 {
		t.Errorf("dev preset: SuggestedPlugins should be empty, got %v", p.SuggestedPlugins)
	}

	// Notes should mention monitoring is disabled.
	foundMonitoringNote := false
	for _, note := range p.Notes {
		if strings.Contains(strings.ToLower(note), "monitor") {
			foundMonitoringNote = true
			break
		}
	}
	if !foundMonitoringNote {
		t.Error("dev preset: Notes should mention monitoring is disabled")
	}
}

func TestNclawAppPreset(t *testing.T) {
	p, ok := initPresets["nclaw-app"]
	if !ok {
		t.Fatal("nclaw-app preset missing")
	}

	// Must include core nClaw bundle plugins.
	requiredPlugins := []string{"ai", "claw", "mux"}
	pluginSet := make(map[string]bool, len(p.SuggestedPlugins))
	for _, pl := range p.SuggestedPlugins {
		pluginSet[pl] = true
	}
	for _, rp := range requiredPlugins {
		if !pluginSet[rp] {
			t.Errorf("nclaw-app preset: SuggestedPlugins missing required plugin %q", rp)
		}
	}

	// Must have redis + search enabled (required by claw memory + vector search).
	serviceSet := make(map[string]bool, len(p.EnabledServices))
	for _, svc := range p.EnabledServices {
		serviceSet[svc] = true
	}
	for _, svc := range []string{"redis", "search"} {
		if !serviceSet[svc] {
			t.Errorf("nclaw-app preset: EnabledServices missing %q", svc)
		}
	}

	// Notes must mention ANTHROPIC_API_KEY.
	foundAnthropicNote := false
	for _, note := range p.Notes {
		if strings.Contains(note, "ANTHROPIC_API_KEY") {
			foundAnthropicNote = true
			break
		}
	}
	if !foundAnthropicNote {
		t.Error("nclaw-app preset: Notes must mention ANTHROPIC_API_KEY")
	}

	// Notes must mention GOOGLE_OAUTH env vars.
	foundGoogleNote := false
	for _, note := range p.Notes {
		if strings.Contains(note, "GOOGLE_OAUTH") {
			foundGoogleNote = true
			break
		}
	}
	if !foundGoogleNote {
		t.Error("nclaw-app preset: Notes must mention GOOGLE_OAUTH credentials")
	}
}

func TestListInitPresetsCoversAllSeven(t *testing.T) {
	listOrder := []string{"b2b-saas", "mobile-backend", "ai-assistant", "community-forum", "media-hosting", "dev", "nclaw-app"}
	if len(listOrder) != 7 {
		t.Errorf("listInitPresets order slice has %d entries, want 7", len(listOrder))
	}
	for _, name := range listOrder {
		if _, ok := initPresets[name]; !ok {
			t.Errorf("listInitPresets references %q but it is not in initPresets", name)
		}
	}
}

func TestPrintPresetPostInitUnknown(t *testing.T) {
	// Should not panic on an unknown preset name.
	printPresetPostInit("does-not-exist")
}

func TestInitPresetsWriteObjectStorage(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		t.Setenv("NSELF_TELEMETRY", "false")
		cmd := newInitCmd()
		cmd.SetArgs([]string{"init", "--non-interactive", "--quiet", "--demo", "--name", "storage-demo"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("demo init: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(dir, ".env"))
		if err != nil {
			t.Fatal(err)
		}
		env := string(data)
		preset, ok := resolveInitPreset("media-hosting")
		if !ok {
			t.Fatal("media-hosting preset missing")
		}
		output, err := captureStdout(t, func() error {
			printPresetPostInit("media-hosting")
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if compat.V15() {
			for _, line := range []string{"OBJECT_STORAGE_ENABLED=true\n", "OBJECT_STORAGE_BUCKETS=uploads\n", "S3_ACCESS_KEY=nself\n", "S3_SECRET_KEY="} {
				if !strings.Contains(env, line) {
					t.Errorf("v1.5 demo env missing %s", strings.TrimSpace(line))
				}
			}
			if strings.Contains(env, "MINIO_ENABLED=true") || strings.Contains(env, "MINIO_ROOT_") {
				t.Error("v1.5 demo still enables core MinIO")
			}
			if !slices.Contains(preset.EnabledServices, "object-storage") || slices.Contains(preset.EnabledServices, "minio") {
				t.Errorf("v1.5 media-hosting services = %v", preset.EnabledServices)
			}
			if strings.Contains(output, "nTV") {
				t.Error("v1.5 media-hosting notes still mention the cancelled nTV bundle")
			}
			if !strings.Contains(output, "OBJECT_STORAGE_ENABLED=true") || !strings.Contains(output, "nself add minio") {
				t.Error("v1.5 media-hosting notes omit object storage or existing MinIO guidance")
			}
		} else {
			if !strings.Contains(env, "MINIO_ENABLED=true\n") || strings.Contains(env, "OBJECT_STORAGE_ENABLED") {
				t.Error("v1.4 demo storage env changed")
			}
			if !slices.Contains(preset.EnabledServices, "minio") || !strings.Contains(output, "Configure MINIO_ACCESS_KEY in .env") || strings.Contains(output, "OBJECT_STORAGE_ENABLED") {
				t.Error("v1.4 media-hosting recommendations changed")
			}
		}
	})
}

// TestInitStandardEnvObjectStorageLine: a standard (non-demo) init lists
// OBJECT_STORAGE_ENABLED=false with the optional services in v1.5 only.
func TestInitStandardEnvObjectStorageLine(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		t.Setenv("NSELF_TELEMETRY", "false")
		cmd := newInitCmd()
		cmd.SetArgs([]string{"init", "--non-interactive", "--quiet", "--name", "storage-std"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("init: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(dir, ".env"))
		if err != nil {
			t.Fatal(err)
		}
		env := string(data)
		if !strings.Contains(env, "MINIO_ENABLED=false\n") {
			t.Error("standard env lost MINIO_ENABLED=false")
		}
		if has := strings.Contains(env, "OBJECT_STORAGE_ENABLED=false\n"); has != compat.V15() {
			t.Errorf("OBJECT_STORAGE_ENABLED=false present = %v, want %v", has, compat.V15())
		}
	})
}
