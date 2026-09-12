package runner

import "testing"

func TestLoadEmbeddedManifest(t *testing.T) {
	m, err := LoadEmbeddedManifest()
	if err != nil {
		t.Fatalf("LoadEmbeddedManifest: %v", err)
	}
	if m.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", m.SchemaVersion)
	}
	if m.RunnerUser == "" || m.WorkDir == "" {
		t.Fatalf("expected runner_user and work_dir to be set, got %+v", m)
	}

	want := []string{"git", "jq", "yq", "gh", "make", "zip", "unzip", "curl",
		"docker", "build-essential", "pkg-config", "libssl-dev",
		"libnspr4", "libnss3", "fonts-liberation"}
	byName := map[string]bool{}
	for _, d := range m.Dependencies {
		byName[d.Name] = true
	}
	for _, name := range want {
		if !byName[name] {
			t.Errorf("manifest missing required dependency %q", name)
		}
	}
	if len(m.ChromiumCacheGlobs) == 0 {
		t.Error("expected at least one chromium_cache_globs entry")
	}
}

func TestParseManifest_RejectsBadSchemaVersion(t *testing.T) {
	_, err := ParseManifest([]byte("schema_version: 2\ndependencies:\n  - name: x\n    apt_package: x\n"))
	if err == nil {
		t.Fatal("expected error for unsupported schema_version")
	}
}

func TestParseManifest_RejectsEmptyDependencies(t *testing.T) {
	_, err := ParseManifest([]byte("schema_version: 1\ndependencies: []\n"))
	if err == nil {
		t.Fatal("expected error for zero dependencies")
	}
}

func TestManifest_AptPackages(t *testing.T) {
	m := &Manifest{Dependencies: []Dependency{
		{Name: "git", AptPackage: "git"},
		{Name: "libssl-dev", AptPackage: "libssl-dev"},
	}}
	got := m.AptPackages()
	if len(got) != 2 || got[0] != "git" || got[1] != "libssl-dev" {
		t.Fatalf("AptPackages() = %v", got)
	}
}

func TestDependency_HasBinary(t *testing.T) {
	if (Dependency{Binary: ""}).HasBinary() {
		t.Error("empty binary should report HasBinary() == false")
	}
	if !(Dependency{Binary: "git"}).HasBinary() {
		t.Error("non-empty binary should report HasBinary() == true")
	}
}
