package compose

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
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

// v2Project builds <root>/.git, <root>/.dockerignore (a good one) and a
// project dir <root>/apps/proj holding two env files, and returns both dirs.
func v2Project(t *testing.T) (root, proj string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj = filepath.Join(root, "apps", "proj")
	for _, d := range []string{filepath.Join(root, ".git"), proj} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		filepath.Join(root, ".dockerignore"): "**/.env*\n**/.secrets\n",
		filepath.Join(proj, ".env.dev"):      "API_KEY=from-dev\nSHARED=dev\n",
		filepath.Join(proj, ".env.secrets"):  "SHARED=from-secrets\n",
	}
	for p, c := range files {
		if err := os.WriteFile(p, []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root, proj
}

// v2Config parses CS_1.. from env (the real parser) into a render config.
func v2Config(t *testing.T, env map[string]string) *config.Config {
	t.Helper()
	for i := 1; i <= 10; i++ {
		for _, k := range []string{"", "_PATH", "_IMAGE", "_ENV_FILE", "_DEPENDS_ON", "_NETWORKS", "_DOCKERFILE", "_BUILD_TARGET", "_COMMAND", "_VOLUMES"} {
			t.Setenv(fmt.Sprintf("CS_%d%s", i, k), "")
		}
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	svcs, err := config.CustomServicesFromEnv()
	if err != nil {
		t.Fatalf("parse CS env: %v", err)
	}
	cfg := minimalConfigWithCS()
	cfg.CustomServices = svcs
	return cfg
}

// errCode returns the CLIError code in err's chain, or "".
func errCode(err error) string {
	var ce *errs.CLIError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

// TestCustomServiceV2Render pins the rendering of every v2 key: the monorepo
// functions fixture and the external-network fixture are goldens; the errors
// and the safety checks are asserted directly.
func TestCustomServiceV2Render(t *testing.T) {
	t.Run("functions golden", func(t *testing.T) {
		_, proj := v2Project(t)
		cfg := v2Config(t, map[string]string{
			"CS_1": "fn:node:9500", "CS_1_PATH": "../..",
			"CS_1_DOCKERFILE": "backend/services/fn/Dockerfile", "CS_1_BUILD_TARGET": "runtime",
			"CS_1_ENV_FILE": ".env.dev,.env.secrets", "CS_1_DEPENDS_ON": "hasura:healthy",
			"CS_1_COMMAND": "node dist/server.js",
		})
		assertCSGolden(t, "functions.golden.yml", csGoldenView(t, cfg, proj))
	})

	t.Run("external network golden", func(t *testing.T) {
		_, proj := v2Project(t)
		cfg := v2Config(t, map[string]string{
			"CS_1": "fn:node:9500", "CS_1_NETWORKS": "test_kafka,test_network,test_kafka",
			"CS_1_DEPENDS_ON": "auth:started,migrate:completed",
		})
		assertCSGolden(t, "networks.golden.yml", csGoldenView(t, cfg, proj))
	})

	t.Run("second env file wins", func(t *testing.T) {
		_, proj := v2Project(t)
		cfg := v2Config(t, map[string]string{"CS_1": "fn:node:9500", "CS_1_ENV_FILE": ".env.dev,.env.secrets"})
		svc, err := NewGenerator(cfg).WithWorkDir(proj).buildCustomService(cfg.CustomServices[0])
		if err != nil {
			t.Fatal(err)
		}
		if svc.Environment["SHARED"] != "from-secrets" || svc.Environment["API_KEY"] != "from-dev" {
			t.Errorf("env merge wrong: SHARED=%q API_KEY=%q", svc.Environment["SHARED"], svc.Environment["API_KEY"])
		}
	})

	t.Run("missing second env file fails", func(t *testing.T) {
		_, proj := v2Project(t)
		cfg := v2Config(t, map[string]string{"CS_1": "fn:node", "CS_1_ENV_FILE": ".env.dev,.env.nope"})
		_, err := NewGenerator(cfg).WithWorkDir(proj).buildCustomService(cfg.CustomServices[0])
		if err == nil || !strings.Contains(err.Error(), "CS_1_ENV_FILE") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("foreign network is E501", func(t *testing.T) {
		_, proj := v2Project(t)
		for _, n := range []string{"other_net", "tes_net", "test_", "test_UPPER", "testing_x"} {
			cfg := v2Config(t, map[string]string{"CS_1": "fn:node", "CS_1_NETWORKS": n})
			_, err := NewGenerator(cfg).WithWorkDir(proj).Generate()
			if errCode(err) != "E501" || !strings.Contains(err.Error(), "CS_1_NETWORKS") || !strings.Contains(err.Error(), n) {
				t.Errorf("network %q: err = %v", n, err)
			}
		}
	})

	t.Run("a service cannot depend on itself", func(t *testing.T) {
		_, proj := v2Project(t)
		cfg := v2Config(t, map[string]string{"CS_1": "fn:node", "CS_1_DEPENDS_ON": "fn"})
		if _, err := NewGenerator(cfg).WithWorkDir(proj).Generate(); errCode(err) != "E500" {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("ancestor context is E528 without a good dockerignore", func(t *testing.T) {
		root, proj := v2Project(t)
		if err := os.WriteFile(filepath.Join(root, ".dockerignore"), []byte("node_modules\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg := v2Config(t, map[string]string{"CS_1": "fn:node", "CS_1_PATH": "../.."})
		_, err := NewGenerator(cfg).WithWorkDir(proj).Generate()
		if errCode(err) != "E528" || !strings.Contains(err.Error(), root) {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("bare .env* in the dockerignore is E528 (nested secrets would be sent)", func(t *testing.T) {
		root, proj := v2Project(t)
		if err := os.WriteFile(filepath.Join(root, ".dockerignore"), []byte(".env*\n.secrets/\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg := v2Config(t, map[string]string{"CS_1": "fn:node", "CS_1_PATH": "../.."})
		_, err := NewGenerator(cfg).WithWorkDir(proj).Generate()
		if errCode(err) != "E528" || !strings.Contains(err.Error(), "does not exclude") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("ancestor above the repository root is E528", func(t *testing.T) {
		_, proj := v2Project(t)
		cfg := v2Config(t, map[string]string{"CS_1": "fn:node", "CS_1_PATH": "../../.."})
		if _, err := NewGenerator(cfg).WithWorkDir(proj).Generate(); errCode(err) != "E528" {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("dockerfile symlink out of the context is refused", func(t *testing.T) {
		root, proj := v2Project(t)
		outside := filepath.Join(root, "outside.Dockerfile")
		if err := os.WriteFile(outside, []byte("FROM scratch\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		ctx := filepath.Join(proj, "services", "fn")
		if err := os.MkdirAll(ctx, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(ctx, "Dockerfile.prod")); err != nil {
			t.Fatal(err)
		}
		cfg := v2Config(t, map[string]string{"CS_1": "fn:node", "CS_1_DOCKERFILE": "Dockerfile.prod"})
		_, err := NewGenerator(cfg).WithWorkDir(proj).Generate()
		if err == nil || !strings.Contains(err.Error(), "resolves outside the build context") {
			t.Errorf("err = %v", err)
		}
	})
}

// TestCustomServiceV2VolumeRule is the ADR 0021 host-bind rule: v1.4 accepts
// every entry (warning once for the socket and host root); v1.5 refuses a
// writable absolute bind outside the project and the socket and host root in
// any mode (E502). Named volumes, project-relative and read-only outside
// binds pass in both modes.
func TestCustomServiceV2VolumeRule(t *testing.T) {
	cases := []struct {
		volume  string
		v15Fail bool
	}{
		{"/srv/x:/data", true},
		{"/srv/x:/data:rw", true},
		{"/srv/x:/data:ro", false},
		{"./data:/data", false},
		{"named:/data", false},
		{"named:/data:ro", false},
		{"/var/run/docker.sock:/var/run/docker.sock:ro", true},
		{"/var/run/docker.sock:/var/run/docker.sock", true},
		{"/run/docker.sock:/s", true},
		{"/:/host:ro", true},
		{"/:/host", true},
		{"//var//run/../run/docker.sock:/s:ro", true},
		{"PROJ/data:/data", false}, // absolute path inside the project
	}
	compattest.Both(t, func(t *testing.T) {
		_, proj := v2Project(t)
		v15 := strings.Contains(t.Name(), "v1.5")
		for _, tc := range cases {
			vol := strings.ReplaceAll(tc.volume, "PROJ", proj)
			cfg := v2Config(t, map[string]string{"CS_1": "fn:node", "CS_1_VOLUMES": vol})
			svc, err := NewGenerator(cfg).WithWorkDir(proj).buildCustomService(cfg.CustomServices[0])
			if v15 && tc.v15Fail {
				if errCode(err) != "E502" || !strings.Contains(err.Error(), "CS_1_VOLUMES") {
					t.Errorf("v1.5 %q: err = %v, want E502", vol, err)
				}
				continue
			}
			if err != nil {
				t.Errorf("%q: unexpected error %v", vol, err)
				continue
			}
			if len(svc.Volumes) != 1 || svc.Volumes[0] != vol {
				t.Errorf("%q: volumes = %q", vol, svc.Volumes)
			}
		}
	})
}

// TestCustomServiceV2DependsCycle: a dependency cycle between custom services
// is refused when the compose file is generated (E500 naming the path), as is
// a longer cycle; a chain without a cycle and a diamond pass.
func TestCustomServiceV2DependsCycle(t *testing.T) {
	_, proj := v2Project(t)
	gen := func(env map[string]string) error {
		_, err := NewGenerator(v2Config(t, env)).WithWorkDir(proj).Generate()
		return err
	}
	for name, env := range map[string]map[string]string{
		"two services": {"CS_1": "api:node:9501", "CS_2": "db2:node:9502", "CS_1_DEPENDS_ON": "db2:started", "CS_2_DEPENDS_ON": "api:started"},
		"three services": {"CS_1": "sa:node:9501", "CS_2": "sb:node:9502", "CS_3": "sc:node:9503",
			"CS_1_DEPENDS_ON": "sb", "CS_2_DEPENDS_ON": "sc", "CS_3_DEPENDS_ON": "sa"},
	} {
		t.Run("cycle "+name, func(t *testing.T) {
			err := gen(env)
			if errCode(err) != "E500" || !strings.Contains(err.Error(), "dependency cycle") || !strings.Contains(err.Error(), " -> ") {
				t.Fatalf("err = %v, want E500 naming the cycle path", err)
			}
		})
	}
	t.Run("chain and diamond pass", func(t *testing.T) {
		if err := gen(map[string]string{"CS_1": "sa:node:9501", "CS_2": "sb:node:9502", "CS_3": "sc:node:9503", "CS_4": "sd:node:9504",
			"CS_1_DEPENDS_ON": "sb,sc", "CS_2_DEPENDS_ON": "sd", "CS_3_DEPENDS_ON": "sd"}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
