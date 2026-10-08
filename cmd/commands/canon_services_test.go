package commands

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestCanonServicesResolution(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		undo := prepareTreeWith(&canonTable, RootCmd, compat.V15(), false)
		defer undo()
		for _, pair := range [][2]string{{"service", "config services"}, {"service list", "config services list"}, {"service enable", "config services enable"}, {"service stop", "config services stop"}, {"service restart", "config services restart"}} {
			for _, path := range pair {
				args, _, err := rewriteCanonArgsWith(&canonTable, RootCmd, strings.Fields(path), compat.V15())
				if err != nil {
					t.Fatalf("rewrite %s: %v", path, err)
				}
				cmd, _, err := RootCmd.Find(args)
				if err != nil {
					t.Fatalf("find %s: %v", path, err)
				}
				want := pair[0]
				if compat.V15() {
					want = pair[1]
				} else if path == pair[1] {
					want = "config"
				}
				if cmd.CommandPath() != "nself "+want {
					t.Errorf("%s: got %s, want %s", path, cmd.CommandPath(), want)
				}
			}
		}
	})
}

func TestCanonServicesRegistry(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		r, err := canon.Effective(compat.V15())
		if err != nil {
			t.Fatal(err)
		}
		entry := r.Commands["service"]
		want := "pending"
		if compat.V15() {
			want = "deprecated-shim"
		}
		if entry.Canon != want {
			t.Errorf("service canon=%q, want %q", entry.Canon, want)
		}
	})
}
