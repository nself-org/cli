package compose

// fronted_stack_test.go — regression coverage for the 2026-09-03 staging
// incident: `nself start` in /opt/nself-ntask failed with "port 80/443 held
// by docker-proxy" because the generated docker-compose.yml always defines
// an nginx service, even for a stack whose domains are actually served by
// ANOTHER stack's nginx (nself-web fronted task.stack-two.nself.org). That
// nginx container can never bind 80/443 — the fronting stack's nginx
// already holds them — so it sits forever as one unhealthy container
// `nself status` can never clear (reported "6/7").
//
// Purpose: prove that setting NGINX_FRONTED_BY (config.NginxConfig.FrontedBy)
// removes the nginx service from the generated compose file entirely, and
// that leaving it unset (the default, every existing project) is unchanged.
// Inputs: a minimal valid Config, with and without FrontedBy set.
// Outputs: pass/fail on whether "nginx" is a key in the built compose's
// Services map.
// Constraints: exercises buildDockerCompose exactly as NewGenerator(cfg)
// callers do; no Docker or filesystem access.

import (
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func frontedTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := &config.Config{
		ProjectName:   "ntask",
		BaseDomain:    "task.stack-two.nself.org",
		Env:           "dev",
		DockerNetwork: "ntask_network",
		Postgres: config.PostgresConfig{
			Host: "postgres", User: "postgres", Password: "password", DB: "nself", Port: 5432,
		},
		Hasura: config.HasuraConfig{
			AdminSecret: "secret", JWTKey: "testsecretkey12345678901234567890", JWTType: "HS256",
		},
	}
	cfg, err := config.ApplyDefaults(cfg)
	if err != nil {
		t.Fatalf("ApplyDefaults() error: %v", err)
	}
	return cfg
}

// TestFrontedStackOmitsOwnNginx verifies that NGINX_FRONTED_BY removes the
// nginx service from the generated compose file — it can never start
// (another stack's nginx already holds 80/443), so it must not be generated.
func TestFrontedStackOmitsOwnNginx(t *testing.T) {
	cfg := frontedTestConfig(t)
	cfg.Nginx.FrontedBy = "nself-web"

	g := NewGenerator(cfg)
	dc, err := g.buildDockerCompose()
	if err != nil {
		t.Fatalf("buildDockerCompose() error: %v", err)
	}

	if _, ok := dc.Services["nginx"]; ok {
		t.Error("nginx service was generated despite NGINX_FRONTED_BY being set — it can never bind 80/443 and will sit unhealthy forever")
	}

	// The rest of the stack must be untouched.
	for _, want := range []string{"postgres", "hasura", "auth"} {
		if _, ok := dc.Services[want]; !ok {
			t.Errorf("expected service %q to still be generated when fronted, but it was missing", want)
		}
	}
}

// TestUnfrontedStackKeepsOwnNginx is the negative control: the default
// (FrontedBy empty) must generate its own nginx service exactly as every
// existing project relies on today.
func TestUnfrontedStackKeepsOwnNginx(t *testing.T) {
	cfg := frontedTestConfig(t)
	// cfg.Nginx.FrontedBy left empty — the default.

	g := NewGenerator(cfg)
	dc, err := g.buildDockerCompose()
	if err != nil {
		t.Fatalf("buildDockerCompose() error: %v", err)
	}

	if _, ok := dc.Services["nginx"]; !ok {
		t.Error("nginx service was not generated for an unfronted (default) stack — this would be a regression for every existing project")
	}
}
