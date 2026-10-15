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

func TestUnrecordedMarkedHandEdit(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		if !compat.V15() {
			t.Skip("v1.5 only")
		}
		for _, missing := range []string{"record", "entry"} {
			t.Run(missing, func(t *testing.T) {
				f := loadFixture(t, "dev-minimal")
				req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("seed")}
				if _, err := Apply(context.Background(), req, ApplyOptions{Yes: true}); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(f.project, "nginx/sites/auth.conf")
				original, err := os.ReadFile(target)
				if err != nil {
					t.Fatal(err)
				}
				if missing == "record" {
					if err := os.Remove(GeneratedStatePath(f.project)); err != nil {
						t.Fatal(err)
					}
				} else {
					state, err := LoadGeneratedState(f.project)
					if err != nil {
						t.Fatal(err)
					}
					delete(state.Files, "nginx/sites/auth.conf")
					if err := state.Save(f.project); err != nil {
						t.Fatal(err)
					}
				}
				// Identical marked output can be adopted without --force.
				if _, err := Apply(context.Background(), req, ApplyOptions{Yes: true}); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(GeneratedStatePath(f.project)); err != nil {
					t.Fatal(err)
				}
				if missing == "entry" {
					state := &GeneratedState{Files: map[string]string{}}
					if err := state.Save(f.project); err != nil {
						t.Fatal(err)
					}
				}
				edited := append(append([]byte(nil), original...), []byte("\n# operator edit\n")...)
				if err := os.WriteFile(target, edited, 0644); err != nil {
					t.Fatal(err)
				}
				var stderr bytes.Buffer
				req.Stderr = &stderr
				p, err := Compute(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(strings.Join(HandEditedPaths(*p), ","), "nginx/sites/auth.conf") {
					t.Error("plan did not mark unrecorded edit")
				}
				_, err = Apply(context.Background(), req, ApplyOptions{Yes: true})
				if err == nil || !strings.Contains(err.Error(), "E403") {
					t.Fatalf("want E403, got %v", err)
				}
				got, err := os.ReadFile(target)
				if err != nil || !bytes.Equal(got, edited) {
					t.Fatalf("refusal changed file: %v", err)
				}
				if !strings.Contains(stderr.String(), "operator edit") {
					t.Fatal("missing edit diff")
				}
				if _, err := Apply(context.Background(), req, ApplyOptions{Yes: true, Force: true}); err != nil {
					t.Fatal(err)
				}
				got, err = os.ReadFile(target)
				if err != nil || !bytes.Equal(got, original) {
					t.Fatalf("force did not restore render: %v", err)
				}
			})
		}
	})
}

func TestUnrecordedMarkedOrphan(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		if !compat.V15() {
			t.Skip("v1.5 only")
		}
		for _, missing := range []bool{false, true} {
			t.Run(map[bool]string{false: "entry", true: "record"}[missing], func(t *testing.T) {
				f := loadFixture(t, "dev-minimal")
				req := Request{Runtime: &fakeRuntime{}, ProjectDir: f.project, Seed: []byte("seed")}
				if _, err := Apply(context.Background(), req, ApplyOptions{Yes: true}); err != nil {
					t.Fatal(err)
				}
				original, err := os.ReadFile(filepath.Join(f.project, "nginx/sites/auth.conf"))
				if err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(f.project, "nginx/sites/auth-copy.conf")
				original = bytes.ReplaceAll(original, []byte("auth.example.test"), []byte("auth-copy.example.test"))
				edited := append(original, []byte("\n# operator copy\n")...)
				if err := os.WriteFile(target, edited, 0644); err != nil {
					t.Fatal(err)
				}
				if missing {
					if err := os.Remove(GeneratedStatePath(f.project)); err != nil {
						t.Fatal(err)
					}
				}
				var stderr bytes.Buffer
				req.Stderr = &stderr
				if _, err := Apply(context.Background(), req, ApplyOptions{Yes: true}); err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(target)
				if err != nil || !bytes.Equal(got, edited) {
					t.Fatalf("foreign copy lost: %v", err)
				}
				if !strings.Contains(stderr.String(), "auth-copy.conf") {
					t.Fatal("foreign copy not printed")
				}
				state, err := LoadGeneratedState(f.project)
				if err != nil {
					t.Fatal(err)
				}
				if _, ok := state.Files["nginx/sites/auth-copy.conf"]; ok {
					t.Fatal("foreign copy adopted into state")
				}
				if _, err := Apply(context.Background(), req, ApplyOptions{Yes: true, Force: true}); err != nil {
					t.Fatal(err)
				}
				if _, err := os.Stat(target); !os.IsNotExist(err) {
					t.Fatalf("force did not remove copy: %v", err)
				}
			})
		}
	})
}
