package reconcile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func TestProcessEnvProdPlanAndRefusal(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	t.Setenv("NSELF_V15", "1")
	t.Setenv("ENV", "prod")
	writeFile(t, filepath.Join(f.project, ".env.prod"), "BASE_DOMAIN=fromprod.example\n", 0o600)
	req := Request{ProjectDir: f.project, Runtime: &fakeRuntime{}, Seed: []byte("process-env-regression")}
	before := treeHash(t, f.root)
	c, err := compute(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if c.plan.Env != "prod" || c.plan.EnvClass != ClassProd || !c.plan.RequiresConfirmation {
		t.Fatalf("process ENV=prod: env=%q env_class=%q requires_confirmation=%v", c.plan.Env, c.plan.EnvClass, c.plan.RequiresConfirmation)
	}
	if !strings.Contains(string(c.after["docker-compose.yml"].Data), "fromprod.example") {
		t.Fatal("the prod cascade was not rendered")
	}
	_, _, err = ApplyBuild(context.Background(), req, ApplyOptions{})
	var refusal *errs.CLIError
	if !errors.As(err, &refusal) || refusal.Code != "E403" {
		t.Fatalf("unconfirmed process ENV=prod build: %v, want E403", err)
	}
	if treeHash(t, f.root) != before {
		t.Fatal("planning or refusal changed the project")
	}
}

func TestProcessEnvDevKeepsProdFileRefusal(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	t.Setenv("ENV", "dev")
	appendEnv(t, f, "ENV=prod\n")
	p := planOf(t, f, nil)
	if !p.RequiresConfirmation {
		t.Fatal("a prod ENV loaded from the file must still require confirmation")
	}
	p = planOf(t, f, func(r *Request) {
		r.EnvOverlay = map[string]map[string]string{".env": {"BASE_DOMAIN": "changed.example"}}
		r.EnvBefore = "dev"
	})
	if !p.RequiresConfirmation {
		t.Fatal("an explicit dev prior ENV must not waive a loaded prod ENV")
	}
}

func TestProcessEnvPinnedThroughConfirmation(t *testing.T) {
	f := loadFixture(t, "dev-minimal")
	t.Setenv("NSELF_V15", "1")
	t.Setenv("ENV", "prod")
	writeFile(t, filepath.Join(f.project, ".env.prod"), "BASE_DOMAIN=fromprod.example\n", 0o600)
	writeFile(t, filepath.Join(f.project, ".env.dev"), "BASE_DOMAIN=fromdev.example\n", 0o600)
	req := Request{ProjectDir: f.project, Runtime: &fakeRuntime{}, Seed: []byte("process-env-pin")}
	p, _, err := ApplyBuild(context.Background(), req, ApplyOptions{Interactive: func(string) bool {
		if err := os.Setenv("ENV", "dev"); err != nil {
			t.Fatal(err)
		}
		return true
	}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Env != "prod" {
		t.Fatalf("confirmed environment = %q, want prod", p.Env)
	}
	compose, err := os.ReadFile(filepath.Join(f.project, "docker-compose.yml"))
	if err != nil || !strings.Contains(string(compose), "fromprod.example") {
		t.Fatalf("confirmed cascade was not applied: %v\n%s", err, compose)
	}
	if os.Getenv("ENV") != "prod" {
		t.Fatal("apply must restore the caller's environment")
	}
}
