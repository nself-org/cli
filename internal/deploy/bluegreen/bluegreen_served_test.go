package bluegreen

// bluegreen_served_test.go — the blue/green upstream conf must be written
// where the running nginx reads it (P7-LIVE-02, D-0045).
//
// Purpose: a project with NGINX_FRONTED_BY set has no nginx of its own; its
// weights file under <project>/nginx/conf.d was never read, so a canary
// "shifted" traffic nowhere. These tests pin that the fronted layout writes
// into the fronting stack and that a project with its own nginx is unchanged.
// Inputs: temp trees shaped like the production layout (D-0121): a fronting
// stack directory with the project in its backend/ subdirectory.
// Outputs: assertions on the written path and bytes.
// Constraints: no docker, no nginx, no network; only writeUpstreamConf and
// frontedByFor are exercised, never the reload.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/nginxtopo"
)

// servedTree creates <tmp>/nself-web/backend and returns both paths.
func servedTree(t *testing.T) (stack, project string) {
	t.Helper()
	stack = filepath.Join(t.TempDir(), "nself-web")
	project = filepath.Join(stack, "backend")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	return stack, project
}

// setFrontedEnv writes NGINX_FRONTED_BY into the project's .env and clears the
// process value so the file is the only source.
func setFrontedEnv(t *testing.T, project, value string) {
	t.Helper()
	t.Setenv("NGINX_FRONTED_BY", "")
	t.Setenv("ENV", "dev")
	if err := os.WriteFile(filepath.Join(project, ".env"), []byte("NGINX_FRONTED_BY="+value+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestBlueGreenServedDir covers the fronted, own-nginx and unconfirmed cases.
func TestBlueGreenServedDir(t *testing.T) {
	cfgFor := func(project string) DeployConfig { return DeployConfig{ProjectRoot: project} }

	t.Run("fronted project writes into the fronting stack", func(t *testing.T) {
		stack, project := servedTree(t)
		setFrontedEnv(t, project, "nself-web")

		root, err := writeUpstreamConf(cfgFor(project), 10)
		if err != nil {
			t.Fatal(err)
		}
		if root != stack {
			t.Errorf("served root = %s, want %s (the reload must run in the stack that owns nginx)", root, stack)
		}
		got, err := os.ReadFile(filepath.Join(stack, "nginx", "conf.d", "bluegreen-upstream.conf"))
		if err != nil {
			t.Fatalf("upstream conf not under the fronting stack: %v", err)
		}
		if want := GenerateNginxUpstream(cfgFor(project), 10); string(got) != want {
			t.Errorf("conf bytes differ:\n%s\nwant\n%s", got, want)
		}
		if _, err := os.Stat(filepath.Join(project, "nginx")); err == nil {
			t.Error("a fronted project wrote its own nginx/ tree, which no nginx reads")
		}
	})

	t.Run("project with its own nginx writes where it always did", func(t *testing.T) {
		project := t.TempDir()
		t.Setenv("NGINX_FRONTED_BY", "")
		root, err := writeUpstreamConf(cfgFor(project), 25)
		if err != nil {
			t.Fatal(err)
		}
		if root != filepath.Clean(project) {
			t.Errorf("served root = %s, want the project dir %s", root, project)
		}
		got, err := os.ReadFile(filepath.Join(project, "nginx", "conf.d", "bluegreen-upstream.conf"))
		if err != nil {
			t.Fatal(err)
		}
		if want := GenerateNginxUpstream(cfgFor(project), 25); string(got) != want {
			t.Errorf("conf bytes changed:\n%s", got)
		}
	})

	t.Run("an unconfirmed fronted layout is refused", func(t *testing.T) {
		project := filepath.Join(t.TempDir(), "elsewhere", "backend")
		if err := os.MkdirAll(project, 0o750); err != nil {
			t.Fatal(err)
		}
		setFrontedEnv(t, project, "nself-web")
		if _, err := writeUpstreamConf(cfgFor(project), 10); !errors.Is(err, nginxtopo.ErrFrontingUnresolved) {
			t.Fatalf("err = %v, want ErrFrontingUnresolved", err)
		}
		if _, err := os.Stat(filepath.Join(project, "nginx")); err == nil {
			t.Error("nothing may be written when the served stack is unknown")
		}
	})

	t.Run("frontedByFor reads the cascade and lets a file beat the process env", func(t *testing.T) {
		project := t.TempDir()
		t.Setenv("ENV", "prod")
		t.Setenv("NGINX_FRONTED_BY", "from-process")
		if got := frontedByFor(project); got != "from-process" {
			t.Errorf("process env only: got %q", got)
		}
		if err := os.WriteFile(filepath.Join(project, ".env"), []byte("NGINX_FRONTED_BY=from-base\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(project, ".env.prod"), []byte("NGINX_FRONTED_BY=from-prod\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := frontedByFor(project); got != "from-prod" {
			t.Errorf("env-specific file must win over .env: got %q", got)
		}
		if got := os.Getenv("NGINX_FRONTED_BY"); got != "from-process" {
			t.Errorf("frontedByFor must not touch the process environment, now %q", got)
		}
	})
}
