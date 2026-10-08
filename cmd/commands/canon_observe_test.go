package commands

import (
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
)

func TestCanonObserveResolution(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		undo := prepareTreeWith(&canonTable, RootCmd, compat.V15(), false)
		defer undo()
		for _, row := range []struct{ old, new string }{
			{"urls", "status urls"}, {"health", "status health"},
			{"health check", "status health check"}, {"self-heal", "doctor heal"},
			{"help-topics", "help topics"},
		} {
			args, _, err := rewriteCanonArgsWith(&canonTable, RootCmd, strings.Fields(row.old), compat.V15())
			if err != nil {
				t.Fatal(err)
			}
			cmd, _, err := RootCmd.Find(args)
			if err != nil {
				t.Fatal(err)
			}
			want := "nself " + row.old
			if compat.V15() {
				want = "nself " + row.new
			}
			if got := cmd.CommandPath(); got != want {
				t.Errorf("%s resolved %s, want %s", row.old, got, want)
			}
		}
	})
}

func TestCanonObserveRegistry(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		r, err := canon.Effective(compat.V15())
		if err != nil {
			t.Fatal(err)
		}
		for _, old := range []string{"urls", "health", "self-heal", "help-topics"} {
			want := "pending"
			if compat.V15() {
				want = "deprecated-shim"
			}
			if got := r.Commands[old].Canon; got != want {
				t.Errorf("%s canon=%s, want %s", old, got, want)
			}
		}
		for _, name := range []string{"completion", "man", "version"} {
			want := "builtin"
			if got := r.Commands[name].Canon; got != want {
				t.Errorf("%s canon=%s, want %s", name, got, want)
			}
		}
	})
}

func TestStatusServiceArgNotShadowed(t *testing.T) {
	undo := prepareTreeWith(&canonTable, RootCmd, true, false)
	defer undo()
	cmd, rest, err := RootCmd.Find([]string{"status", "postgres"})
	if err != nil || cmd != statusCmd || len(rest) != 1 || rest[0] != "postgres" {
		t.Fatalf("status postgres: command=%v rest=%q err=%v", cmd, rest, err)
	}
}

func TestStatusDashDashService(t *testing.T) {
	nextStepFixture(t, "project")
	t.Setenv("NEXTSTEP_DOCKER", "service")
	t.Setenv("CS_1", "health:go")
	cfg, err := config.Load(mustCwd(t))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, name := range build.DetectServices(cfg) {
		if name == "health" {
			found = true
		}
	}
	if !found {
		t.Fatal("fixture must define custom service health")
	}
	undo := prepareTreeWith(&canonTable, RootCmd, true, false)
	defer undo()
	for _, service := range []string{"health", "urls"} {
		args := []string{"status", "--", service}
		got, _, err := rewriteCanonArgsWith(&canonTable, RootCmd, args, true)
		if err != nil || strings.Join(got, " ") != strings.Join(args, " ") {
			t.Fatalf("rewrite %q = %q, %v", args, got, err)
		}
		cmd, rest, err := RootCmd.Find(got)
		if err != nil || cmd != statusCmd || len(rest) == 0 || rest[len(rest)-1] != service {
			t.Fatalf("status -- %s: command=%v rest=%q err=%v", service, cmd, rest, err)
		}
		if service == "health" {
			if err := statusCmd.RunE(statusCmd, []string{service}); err != nil {
				t.Fatalf("single-service handler did not run: %v", err)
			}
		}
	}
}

func mustCwd(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return cwd
}
