package reconcile

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestBuildHandEditRefusal proves v1.5 refuses to overwrite a hand-edited file without --force.
func TestBuildHandEditRefusal(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		if !compat.V15() {
			t.Skip("v1.5 only")
		}

		f := loadFixture(t, "dev-minimal")
		req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("seed")}

		// 1. Initial successful apply creates the state.
		_, err := Apply(context.Background(), req, ApplyOptions{Yes: true})
		if err != nil {
			t.Fatal(err)
		}

		// 2. Simulate a hand edit by modifying a generated file directly.
		target := filepath.Join(f.project, "docker-compose.yml")
		b, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(target, append(b, []byte("\n# hand edit")...), 0644)

		// 3. The next apply should fail in v1.5 without --force
		var stderr bytes.Buffer
		req.Stderr = &stderr
		_, err = Apply(context.Background(), req, ApplyOptions{Yes: true})

		if err == nil || !strings.Contains(err.Error(), "hand-edited") || !strings.Contains(err.Error(), "--force") {
			t.Fatalf("expected refusal with --force instruction, got %v", err)
		}
		if !strings.Contains(stderr.String(), "---") {
			t.Fatalf("expected unified diff of hand edit, got: %s", stderr.String())
		}

		// With force, it must succeed.
		_, err = Apply(context.Background(), req, ApplyOptions{Yes: true, Force: true})
		if err != nil {
			t.Fatalf("expected success with --force, got %v", err)
		}
	})
}

// TestBuildHandEditV14Warns proves v1.4 mode warns instead of refusing
func TestBuildHandEditV14Warns(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		if compat.V15() {
			t.Skip("v1.4 only")
		}

		f := loadFixture(t, "dev-minimal")
		req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("seed")}

		_, err := Apply(context.Background(), req, ApplyOptions{Yes: true})
		if err != nil {
			t.Fatal(err)
		}

		target := filepath.Join(f.project, "docker-compose.yml")
		b, err := os.ReadFile(target)
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(target, append(b, []byte("\n# hand edit")...), 0644)

		var stderr bytes.Buffer
		req.Stderr = &stderr
		_, err = Apply(context.Background(), req, ApplyOptions{Yes: true})

		if err != nil {
			t.Fatalf("expected v1.4 to succeed with a warning, got %v", err)
		}
		if !strings.Contains(stderr.String(), "v1.5 refuses without --force") {
			t.Fatalf("expected v1.4 to warn about hand edits, got: %s", stderr.String())
		}
	})
}
