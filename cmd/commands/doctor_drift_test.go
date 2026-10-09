package commands

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/spf13/cobra"
)

func doctorTreeHash(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintf(h, "%s\x00%s\x00", rel, body)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func TestDoctorGeneratedDriftFixPlanIdempotent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=fx\nENV=dev\nBASE_DOMAIN=example.test\nSSL_MODE=none\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req := doctorDriftRequest(dir)
	req.Seed = []byte("doctor-seed")
	if _, err := reconcile.Apply(context.Background(), req, reconcile.ApplyOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(dir, "nginx", "nginx.conf")
	if err := os.Remove(conf); err != nil {
		t.Fatal(err)
	}
	compose := filepath.Join(dir, "docker-compose.yml")
	f, err := os.OpenFile(compose, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n# drift\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	before := doctorTreeHash(t, dir)
	r, p, err := checkGeneratedDrift(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" || !strings.Contains(r.Message, "nginx/nginx.conf") || !strings.Contains(r.Message, "docker-compose.yml (hand-edited)") {
		t.Fatalf("drift result: %+v", r)
	}
	var rendered strings.Builder
	planCmd := &cobra.Command{}
	planCmd.SetOut(&rendered)
	if err := printDoctorDriftPlan(context.Background(), planCmd, dir); err != nil {
		t.Fatal(err)
	}
	if p == nil || rendered.Len() == 0 || doctorTreeHash(t, dir) != before {
		t.Fatal("plan was empty or wrote files")
	}
	cmd := &cobra.Command{}
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("force", false, "")
	if err := cmd.Flags().Set("force", "true"); err != nil {
		t.Fatal(err)
	}
	r, err = runDriftFix(context.Background(), cmd, dir)
	if err != nil || r.Status != "pass" {
		t.Fatalf("fix: %+v, %v", r, err)
	}
	after := doctorTreeHash(t, dir)
	r, err = runDriftFix(context.Background(), cmd, dir)
	if err != nil || r.Status != "pass" || doctorTreeHash(t, dir) != after {
		t.Fatalf("second fix: %+v, %v", r, err)
	}
}

func TestDoctorFreshProjectSecondFixNoop(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=fx\nENV=dev\nBASE_DOMAIN=example.test\nSSL_MODE=none\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{}
	cmd.Flags().Bool("yes", false, "")
	cmd.Flags().Bool("force", false, "")
	if _, err := runDriftFix(context.Background(), cmd, dir); err != nil {
		t.Fatal(err)
	}
	first := doctorTreeHash(t, dir)
	if _, err := runDriftFix(context.Background(), cmd, dir); err != nil {
		t.Fatal(err)
	}
	if second := doctorTreeHash(t, dir); second != first {
		t.Fatal("second fix changed a fresh project")
	}
}

func TestDoctorGeneratedDriftBothModes(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=fx\nENV=dev\nBASE_DOMAIN=example.test\nSSL_MODE=none\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		before := doctorTreeHash(t, dir)
		r, _, err := checkGeneratedDrift(context.Background(), dir)
		if err != nil || r.Status != "fail" || !strings.Contains(r.Message, "docker-compose.yml") {
			t.Fatalf("check: %+v, %v", r, err)
		}
		if doctorTreeHash(t, dir) != before {
			t.Fatal("drift scan wrote project files")
		}
	})
}
