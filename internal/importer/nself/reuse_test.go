package nself

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/portable"
)

func TestVerifiedReadersSurviveStaging(t *testing.T) {
	for _, mutate := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged", true: "changed after detect"}[mutate], func(t *testing.T) {
			in := input(makeBundle(t))
			d, err := (Source{}).Detect(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			if d.Bundle == nil {
				t.Fatal("Detect did not hand off verified reader")
			}
			in.Bundle = d.Bundle
			if mutate {
				if err := os.WriteFile(filepath.Join(in.Values["dir"], "db", "data.dump"), []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			stage := filepath.Join(t.TempDir(), "stage")
			ext, err := (Source{}).Extract(context.Background(), in, stage)
			if mutate {
				if codeOf(err) != "E516" {
					t.Fatalf("changed source accepted: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if ext.Bundle == nil || ext.Bundle.Dir() != stage {
				t.Fatal("Extract did not hand off verified stage")
			}
			if _, err := portable.Open(stage); err != nil {
				t.Fatal(err)
			}
		})
	}
}
