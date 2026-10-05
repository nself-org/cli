package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

// Purpose: parse and build-context tests for the CS_N v2 keys (P7-ADOPT-01).
// Inputs: CS_1_* variables set with t.Setenv; temp directory trees.
// Outputs: none (t.Error on a wrong parse result or error text).
// Constraints: every test clears all ten CS slots first so no state leaks.

// csV2Env clears every CS slot variable the v2 tests touch, then sets env.
func csV2Env(t *testing.T, env map[string]string) {
	t.Helper()
	keys := []string{"", "_PATH", "_IMAGE", "_ENV_FILE", "_DEPENDS_ON", "_NETWORKS", "_DOCKERFILE", "_BUILD_TARGET", "_COMMAND", "_VOLUMES"}
	for i := 1; i <= 10; i++ {
		for _, k := range keys {
			t.Setenv("CS_"+itoa(i)+k, "")
		}
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
}

// TestCustomServiceV2Parse covers each v2 key: valid, invalid and empty.
func TestCustomServiceV2Parse(t *testing.T) {
	t.Run("full acceptance fixture", func(t *testing.T) {
		csV2Env(t, map[string]string{
			"CS_1": "fn:node:9500", "CS_1_PATH": "../..",
			"CS_1_DOCKERFILE": "backend/services/fn/Dockerfile", "CS_1_BUILD_TARGET": "runtime",
			"CS_1_ENV_FILE": ".env.dev,.env.secrets", "CS_1_DEPENDS_ON": "hasura:healthy",
			"CS_1_COMMAND": "node dist/server.js", "CS_1_NETWORKS": "proj_kafka",
		})
		svcs, err := parseCustomServices()
		if err != nil || len(svcs) != 1 {
			t.Fatalf("parse: %v (%d services)", err, len(svcs))
		}
		cs := svcs[0]
		if cs.BuildPath != "../.." || cs.Dockerfile != "backend/services/fn/Dockerfile" || cs.BuildTarget != "runtime" {
			t.Errorf("build fields wrong: %+v", cs)
		}
		if cs.EnvFile != ".env.dev" || !reflect.DeepEqual(cs.EnvFiles, []string{".env.dev", ".env.secrets"}) {
			t.Errorf("env files = %q / %q", cs.EnvFile, cs.EnvFiles)
		}
		if !reflect.DeepEqual(cs.DependsOn, []CSDependency{{"hasura", "service_healthy"}}) {
			t.Errorf("depends_on = %+v", cs.DependsOn)
		}
		if !reflect.DeepEqual(cs.Command, []string{"node", "dist/server.js"}) || !reflect.DeepEqual(cs.Networks, []string{"proj_kafka"}) {
			t.Errorf("command/networks = %q / %q", cs.Command, cs.Networks)
		}
	})

	t.Run("no v2 key leaves the fields zero", func(t *testing.T) {
		csV2Env(t, map[string]string{"CS_1": "api:go:8001", "CS_1_ENV_FILE": ".env.one"})
		svcs, err := parseCustomServices()
		if err != nil {
			t.Fatal(err)
		}
		cs := svcs[0]
		if cs.DependsOn != nil || cs.Networks != nil || cs.Dockerfile != "" || cs.BuildTarget != "" || cs.Command != nil {
			t.Errorf("v2 fields set without a v2 key: %+v", cs)
		}
		if cs.EnvFile != ".env.one" || len(cs.EnvFiles) != 1 {
			t.Errorf("single env file lost: %q %q", cs.EnvFile, cs.EnvFiles)
		}
	})

	bad := []struct {
		name, key, val, want string
	}{
		{"condition", "CS_1_DEPENDS_ON", "hasura:bogus", `[E500] CS_1_DEPENDS_ON has an invalid condition "bogus"`},
		{"dep name", "CS_1_DEPENDS_ON", "has ura", "[E500] CS_1_DEPENDS_ON"},
		{"dep empty entry", "CS_1_DEPENDS_ON", "hasura,,auth", "[E500] CS_1_DEPENDS_ON"},
		{"dep duplicate", "CS_1_DEPENDS_ON", "hasura,hasura:started", "more than once"},
		{"network syntax", "CS_1_NETWORKS", "ok_net,bad net", "[E501] CS_1_NETWORKS"},
		{"network empty entry", "CS_1_NETWORKS", "a,,b", "[E501] CS_1_NETWORKS"},
		{"dockerfile dotdot", "CS_1_DOCKERFILE", "../Dockerfile", "CS_1_DOCKERFILE must not contain '..'"},
		{"dockerfile nested dotdot", "CS_1_DOCKERFILE", "a/../../Dockerfile", "CS_1_DOCKERFILE"},
		{"dockerfile absolute", "CS_1_DOCKERFILE", "/etc/passwd", "CS_1_DOCKERFILE must be a relative path"},
		{"dockerfile backslash", "CS_1_DOCKERFILE", `a\..\b`, "CS_1_DOCKERFILE must be a plain relative path"},
		{"target spaces", "CS_1_BUILD_TARGET", "run time", "CS_1_BUILD_TARGET must match"},
		{"target too long", "CS_1_BUILD_TARGET", strings.Repeat("a", 65), "CS_1_BUILD_TARGET must match"},
		{"env file dotdot", "CS_1_ENV_FILE", ".env,../.env", "CS_1_ENV_FILE must not contain '..'"},
		{"env file empty entry", "CS_1_ENV_FILE", ".env,,.env.b", "CS_1_ENV_FILE contains an empty entry"},
		{"path sibling", "CS_1_PATH", "../sibling", "CS_1_PATH may name an ancestor"},
		{"path ancestor then descend", "CS_1_PATH", "../../apps/x", "CS_1_PATH may name an ancestor"},
		{"path absolute", "CS_1_PATH", "/srv/x", "CS_1_PATH must be a relative path"},
		{"path dotdot inside", "CS_1_PATH", "a/../b", "CS_1_PATH"},
	}
	for _, tc := range bad {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			csV2Env(t, map[string]string{"CS_1": "fn:node:9500", tc.key: tc.val})
			_, err := parseCustomServices()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}

	t.Run("image excludes dockerfile and target", func(t *testing.T) {
		for _, k := range []string{"CS_1_DOCKERFILE", "CS_1_BUILD_TARGET"} {
			csV2Env(t, map[string]string{"CS_1": "fn:node", "CS_1_IMAGE": "x/y:1", k: "a"})
			if _, err := parseCustomServices(); err == nil || !strings.Contains(err.Error(), "CS_1_IMAGE") {
				t.Errorf("%s with IMAGE: err = %v", k, err)
			}
		}
	})

	t.Run("depends_on conditions", func(t *testing.T) {
		csV2Env(t, map[string]string{"CS_1": "fn:node", "CS_1_DEPENDS_ON": "a, b:started ,c:completed"})
		svcs, err := parseCustomServices()
		if err != nil {
			t.Fatal(err)
		}
		want := []CSDependency{{"a", "service_healthy"}, {"b", "service_started"}, {"c", "service_completed_successfully"}}
		if !reflect.DeepEqual(svcs[0].DependsOn, want) {
			t.Errorf("depends_on = %+v, want %+v", svcs[0].DependsOn, want)
		}
	})
}

// buildContextTree makes <root>/.git and returns the root and a project dir
// two levels below it. dockerignore is written at the root when non-empty.
func buildContextTree(t *testing.T, dockerignore string) (root, proj string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	proj = filepath.Join(root, "apps", "svc")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	if dockerignore != "" {
		if err := os.WriteFile(filepath.Join(root, ".dockerignore"), []byte(dockerignore), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, proj
}

// TestCustomServiceV2BuildContext covers the ancestor bound and the
// .dockerignore requirement of CS_N_PATH (E528).
func TestCustomServiceV2BuildContext(t *testing.T) {
	good := "node_modules\n.env*\n.secrets/\n"
	cases := []struct {
		name, ignore, path, wantErr string
	}{
		{"ancestor at the repo root with a good dockerignore", good, "../..", ""},
		{"alternate spellings", "**/.env*\n/.secrets\n", "../..", ""},
		{"re-including one example file is fine", good + "!.env.example\n", "../..", ""},
		{"path inside the project needs no dockerignore", "", "./services/x", ""},
		{"empty path", "", "", ""},
		{"one level up is not the root but is inside it", "", "..", "whose .dockerignore must exclude"},
		{"above the repository root", good, "../../..", "above the repository root"},
		{"missing dockerignore", "", "../..", "whose .dockerignore must exclude"},
		{"dockerignore without .env*", "node_modules\n.secrets/\n", "../..", "found .env*: false, .secrets: true"},
		{"dockerignore without .secrets", ".env*\n", "../..", "found .env*: true, .secrets: false"},
		{"comment does not count", "# .env*\n# .secrets\n", "../..", "whose .dockerignore must exclude"},
		{"negated .env* cancels", good + "!.env*\n", "../..", "found .env*: false"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, proj := buildContextTree(t, tc.ignore)
			err := ValidateBuildContext("CS_1_PATH", proj, tc.path)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var ce *errs.CLIError
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
			if !asCLIError(err, &ce) || ce.Code != "E528" {
				t.Errorf("error is not E528: %v", err)
			}
			if !strings.Contains(err.Error(), "CS_1_PATH") {
				t.Errorf("error does not name CS_1_PATH: %v", err)
			}
			_ = root
		})
	}

	t.Run("names the directory", func(t *testing.T) {
		root, proj := buildContextTree(t, "")
		err := ValidateBuildContext("CS_1_PATH", proj, "../..")
		if err == nil || !strings.Contains(err.Error(), root) {
			t.Fatalf("error should name %s: %v", root, err)
		}
	})

	t.Run("no .git: the project dir is the limit", func(t *testing.T) {
		dir, _ := filepath.EvalSymlinks(t.TempDir())
		proj := filepath.Join(dir, "a", "b")
		if err := os.MkdirAll(proj, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".dockerignore"), []byte(".env*\n.secrets\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := ValidateBuildContext("CS_1_PATH", proj, ".."); err == nil || !strings.Contains(err.Error(), "above the repository root") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("a linked worktree .git file marks the root", func(t *testing.T) {
		root, proj := buildContextTree(t, good)
		if err := os.Remove(filepath.Join(root, ".git")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := ValidateBuildContext("CS_1_PATH", proj, "../.."); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("project with its own .git cannot climb", func(t *testing.T) {
		_, proj := buildContextTree(t, good)
		if err := os.Mkdir(filepath.Join(proj, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := ValidateBuildContext("CS_1_PATH", proj, "../.."); err == nil {
			t.Error("expected E528: the project dir holds .git so it is the limit")
		}
	})
}

// asCLIError is errors.As for *errs.CLIError without importing errors twice.
func asCLIError(err error, target **errs.CLIError) bool {
	for err != nil {
		if ce, ok := err.(*errs.CLIError); ok {
			*target = ce
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
