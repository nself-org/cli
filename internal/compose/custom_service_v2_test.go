package compose

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"gopkg.in/yaml.v3"
)

// Purpose: golden tests for the CS_N custom-service renderer. The basis golden
// proves services that use none of the v2 keys render byte-identically to the
// merge-base (P7-ADOPT-01: no v1.4 drift); the v2 goldens pin the new keys.
// Inputs: hand-built config.CustomService fixtures and a temp project dir.
// Outputs: comparison against internal/compose/testdata/custom_service_v2/*.
// Constraints: set NSELF_UPDATE_GOLDEN=1 to rewrite a golden; never do that
// for basis.golden.yml, which must stay equal to the merge-base rendering.

// csGoldenView renders cfg with the full generator and keeps only the
// top-level networks plus the named custom services, so the golden does not
// move when an unrelated core service changes.
func csGoldenView(t *testing.T, cfg *config.Config, workDir string) []byte {
	t.Helper()
	raw, err := NewGenerator(cfg).WithWorkDir(workDir).Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal generated compose: %v", err)
	}
	keep := map[string]bool{}
	for _, cs := range cfg.CustomServices {
		keep[cs.Name] = true
	}
	out := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	top := doc.Content[0].Content
	for i := 0; i+1 < len(top); i += 2 {
		switch top[i].Value {
		case "networks":
			out.Content = append(out.Content, top[i], top[i+1])
		case "services":
			svcs := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			s := top[i+1].Content
			for j := 0; j+1 < len(s); j += 2 {
				if keep[s[j].Value] {
					svcs.Content = append(svcs.Content, s[j], s[j+1])
				}
			}
			out.Content = append(out.Content, top[i], svcs)
		}
	}
	b, err := yaml.Marshal(out)
	if err != nil {
		t.Fatalf("marshal golden view: %v", err)
	}
	return b
}

// assertCSGolden compares got with testdata/custom_service_v2/<name>.
func assertCSGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "custom_service_v2", name)
	if os.Getenv("NSELF_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	if string(got) != string(want) {
		t.Errorf("%s drifted.\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

// basisFixtureConfig returns a config whose custom services use only keys
// that existed before CS_N v2 (path, image, env file, volumes, healthcheck,
// underscore name). Its rendering is frozen in basis.golden.yml.
func basisFixtureConfig(t *testing.T) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env.basis"), []byte("SMTP_HOST=mail\nFOO=bar\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := minimalConfigWithCS()
	cfg.CustomServices = []config.CustomService{
		{Index: 1, Name: "ping-api", RawName: "ping_api", Template: "go", Port: 8001, Route: "ping", Public: true, Memory: "256m", CPU: "0.5"},
		{Index: 2, Name: "blobs", Template: "minio", Port: 9000, Memory: "512m", CPU: "1.0",
			Image: "minio/minio:RELEASE.2024-01-16T16-07-38Z", HealthCheck: "disabled"},
		{Index: 3, Name: "worker", Template: "node", Port: 8003, Memory: "128m", CPU: "0.25",
			BuildPath: "./apps/worker", EnvFile: ".env.basis", ExtraEnv: "MODE=fast",
			Volumes: "./data:/data,named:/cache:ro", HealthCheck: "/ready"},
	}
	return cfg, dir
}

// TestCustomServiceBasisGolden proves services without any v2 key render
// byte-identically to the merge-base (no v1.4 drift).
func TestCustomServiceBasisGolden(t *testing.T) {
	cfg, dir := basisFixtureConfig(t)
	assertCSGolden(t, "basis.golden.yml", csGoldenView(t, cfg, dir))
}
