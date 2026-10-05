package build

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// csDepsProject writes a generated compose, a plugin fragment that defines
// claw-api and the compose manifest into a temp project, then sets the CS_N
// environment (all ten slots cleared first). It returns the compose path.
func csDepsProject(t *testing.T, env map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	composePath := filepath.Join(dir, "docker-compose.yml")
	writeFile(t, composePath, `services:
  postgres:
    image: postgres:16
  hasura:
    image: hasura/graphql-engine:latest
  api:
    image: node:20
    depends_on:
      postgres:
        condition: service_healthy
`)
	fragment := filepath.Join(dir, "plugins", "claw", "docker-compose.plugin.yml")
	writeFile(t, fragment, "services:\n  claw-api:\n    image: claw:1\n")
	writeFile(t, filepath.Join(dir, ".nself", "compose-files.txt"), composePath+"\n"+fragment+"\n")
	for i := 1; i <= 10; i++ {
		for _, suffix := range []string{"", "_DEPENDS_ON"} {
			t.Setenv(fmt.Sprintf("CS_%d%s", i, suffix), "")
		}
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	return composePath
}

// TestPostValidateCSDeps covers the CS_N_DEPENDS_ON resolution that runs after
// the plugin step: core, custom and plugin services pass; an unknown name is
// E500 naming the key and the name; a project without the key is untouched.
func TestPostValidateCSDeps(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		wantErr string // substring of the single error, "" for none
	}{
		{"no dependency key", map[string]string{"CS_1": "fn:node:9500"}, ""},
		{"core service", map[string]string{"CS_1": "fn:node:9500", "CS_1_DEPENDS_ON": "hasura:healthy"}, ""},
		{"plugin service from a fragment", map[string]string{"CS_1": "fn:node:9500", "CS_1_DEPENDS_ON": "claw-api"}, ""},
		{"another custom service", map[string]string{"CS_1": "fn:node:9500", "CS_2": "api:node:9501", "CS_1_DEPENDS_ON": "api:started"}, ""},
		{"unknown name", map[string]string{"CS_1": "fn:node:9500", "CS_1_DEPENDS_ON": "nope"}, `[E500] CS_1_DEPENDS_ON names "nope"`},
		{"one known one unknown", map[string]string{"CS_1": "fn:node:9500", "CS_1_DEPENDS_ON": "hasura,claw-apx"}, `"claw-apx"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			composePath := csDepsProject(t, tc.env)
			var res PostValidateResult
			checkCustomServiceDeps(composePath, &res)
			if tc.wantErr == "" {
				if len(res.Errors) != 0 {
					t.Fatalf("unexpected errors: %v", res.Errors)
				}
				return
			}
			if len(res.Errors) != 1 || !strings.Contains(res.Errors[0], tc.wantErr) {
				t.Fatalf("errors = %q, want one containing %q", res.Errors, tc.wantErr)
			}
		})
	}
}

// TestPostValidateCSDeps_ThroughPostValidate proves PostValidate itself runs
// the check, so an unknown dependency fails the build.
func TestPostValidateCSDeps_ThroughPostValidate(t *testing.T) {
	composePath := csDepsProject(t, map[string]string{"CS_1": "fn:node:9500", "CS_1_DEPENDS_ON": "nope"})
	res := PostValidate(composePath, filepath.Join(filepath.Dir(composePath), "nginx", "sites"))
	found := false
	for _, e := range res.Errors {
		if strings.Contains(e, "E500") && strings.Contains(e, "nope") {
			found = true
		}
	}
	if !found {
		t.Fatalf("PostValidate errors %q lack the E500 finding", res.Errors)
	}
}

// TestPostValidateCSDeps_Cycle: a cycle that runs through a plugin service (or
// through two custom services whose compose entries list each other) fails the
// build with E500 naming the full path; acyclic stacks pass.
func TestPostValidateCSDeps_Cycle(t *testing.T) {
	cases := []struct {
		name     string
		fragment string // extra plugin service definition
		env      map[string]string
		want     string
	}{
		{"through a plugin service", "  claw-api:\n    image: claw:1\n    depends_on:\n      - fn\n",
			map[string]string{"CS_1": "fn:node:9500", "CS_1_DEPENDS_ON": "claw-api:started"}, "fn -> claw-api -> fn"},
		{"plugin map form", "  claw-api:\n    image: claw:1\n    depends_on:\n      fn:\n        condition: service_started\n",
			map[string]string{"CS_1": "fn:node:9500", "CS_1_DEPENDS_ON": "claw-api"}, "fn -> claw-api -> fn"},
		{"plugin depends on core only", "  claw-api:\n    image: claw:1\n    depends_on:\n      - postgres\n",
			map[string]string{"CS_1": "fn:node:9500", "CS_1_DEPENDS_ON": "claw-api"}, ""},
		{"diamond through a plugin fragment is not a cycle",
			"  claw-api:\n    image: claw:1\n    depends_on:\n      - claw-db\n      - claw-cache\n  claw-db:\n    image: db:1\n    depends_on:\n      - claw-base\n  claw-cache:\n    image: cache:1\n    depends_on:\n      - claw-base\n  claw-base:\n    image: base:1\n",
			map[string]string{"CS_1": "fn:node:9500", "CS_1_DEPENDS_ON": "claw-api"}, ""},
		{"cycle among plugin services reachable from a custom service",
			"  claw-api:\n    image: claw:1\n    depends_on:\n      - claw-db\n  claw-db:\n    image: db:1\n    depends_on:\n      - claw-api\n",
			map[string]string{"CS_1": "fn:node:9500", "CS_1_DEPENDS_ON": "claw-api"}, "claw-api -> claw-db -> claw-api"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			composePath := csDepsProject(t, tc.env)
			dir := filepath.Dir(composePath)
			// The generated compose lists the custom service and its depends_on, as the generator renders it.
			writeFile(t, composePath, "services:\n  postgres:\n    image: postgres:16\n  fn:\n    image: node:20\n    depends_on:\n      postgres:\n        condition: service_healthy\n      "+tc.env["CS_1_DEPENDS_ON"][:strings.IndexAny(tc.env["CS_1_DEPENDS_ON"]+":", ":")]+":\n        condition: service_started\n")
			writeFile(t, filepath.Join(dir, "plugins", "claw", "docker-compose.plugin.yml"), "services:\n"+tc.fragment)
			var res PostValidateResult
			checkCustomServiceDeps(composePath, &res)
			if tc.want == "" {
				if len(res.Errors) != 0 {
					t.Fatalf("unexpected errors: %v", res.Errors)
				}
				return
			}
			if len(res.Errors) != 1 || !strings.Contains(res.Errors[0], "[E500]") || !strings.Contains(res.Errors[0], tc.want) {
				t.Fatalf("errors = %q, want one E500 containing %q", res.Errors, tc.want)
			}
		})
	}
}
