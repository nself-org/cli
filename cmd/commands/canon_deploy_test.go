package commands

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestCanonDeployResolution(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		undo := prepareTreeWith(&canonTable, RootCmd, compat.V15(), false)
		defer undo()
		rows := []struct{ old, new, parent string }{
			{"dev", "start dev", "dev"},
			{"promote", "deploy promote-env", "deploy"},
			{"promote rollback", "deploy promote-env rollback", "promote rollback"},
			{"ops deploy", "deploy ops", "deploy"},
			{"access list", "deploy access list", "access list"},
			{"security audit", "doctor security audit", "doctor"},
			{"security status", "doctor security status", "doctor"},
			{"security setup", "deploy security setup", "security setup"},
			{"verify-sbom", "update verify-sbom", "update"},
			{"ops", "deploy ops", "deploy"},
			{"security", "doctor security", "doctor"},
		}
		for _, row := range rows {
			for _, path := range []string{row.old, row.new} {
				args, _, err := rewriteCanonArgsWith(&canonTable, RootCmd, strings.Fields(path), compat.V15())
				if err != nil {
					t.Fatalf("rewrite %s: %v", path, err)
				}
				cmd, _, err := RootCmd.Find(args)
				if err != nil {
					t.Fatalf("find %s: %v", path, err)
				}
				want := row.old
				if compat.V15() {
					want = row.new
				} else if path == row.new {
					want = row.parent
				}
				if cmd.CommandPath() != "nself "+want {
					t.Errorf("%s: got %s, want %s", path, cmd.CommandPath(), want)
				}
			}
		}
	})
}

func TestCanonDeployRegistry(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		r, err := canon.Effective(compat.V15())
		if err != nil {
			t.Fatal(err)
		}
		for _, old := range []string{"dev", "promote", "ops", "access", "security", "verify-sbom"} {
			entry, ok := r.Commands[old]
			if !ok {
				t.Errorf("missing %s", old)
				continue
			}
			want := "pending"
			if compat.V15() {
				want = "deprecated-shim"
			}
			if entry.Canon != want {
				t.Errorf("%s canon=%q, want %q", old, entry.Canon, want)
			}
		}
	})
}
