package commands

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/spf13/pflag"
)

func TestCanonConfigResolution(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		undo := prepareTreeWith(&canonTable, RootCmd, compat.V15(), false)
		defer undo()

		tests := []struct {
			args []string
			v14  string
			v15  string
		}{
			{[]string{"env"}, "nself env", "nself config env"},
			{[]string{"env", "list"}, "nself env list", "nself config env list"},
			{[]string{"secrets"}, "nself secrets", "nself config secrets"},
			{[]string{"oauth", "refresh"}, "nself oauth refresh", "nself config oauth refresh"},
			{[]string{"telemetry", "status"}, "nself telemetry status", "nself config telemetry status"},
			{[]string{"trust", "status"}, "nself trust status", "nself config trust status"},
			{[]string{"config", "env"}, "nself config", "nself config env"},
			{[]string{"config", "secrets"}, "nself config", "nself config secrets"},
			{[]string{"config", "oauth", "refresh"}, "nself config", "nself config oauth refresh"},
			{[]string{"config", "telemetry"}, "nself config", "nself config telemetry"},
			{[]string{"config", "trust"}, "nself config", "nself config trust"},
		}
		for _, tt := range tests {
			t.Run(strings.Join(tt.args, "_"), func(t *testing.T) {
				args, _, err := rewriteCanonArgsWith(&canonTable, RootCmd, tt.args, compat.V15())
				if err != nil {
					t.Fatalf("rewrite: %v", err)
				}
				cmd, _, err := RootCmd.Find(args)
				if err != nil {
					t.Fatalf("find: %v", err)
				}
				want := tt.v15
				if !compat.V15() {
					want = tt.v14
				}
				if cmd.CommandPath() != want {
					t.Errorf("got %q, want %q", cmd.CommandPath(), want)
				}
			})
		}
	})
}

func TestCanonConfigInheritedFlags(t *testing.T) {
	undo := prepareTreeWith(&canonTable, RootCmd, true, false)
	defer undo()

	moves := []string{"config env", "config secrets", "config oauth", "config telemetry", "config trust"}
	wantInherited := []string{"env", "json", "reveal"}

	for _, p := range moves {
		args := strings.Split(p, " ")
		cmd, _, err := RootCmd.Find(args)
		if err != nil {
			t.Fatalf("find %s: %v", p, err)
		}

		// Also check the parent directly to debug
		if cmd.Parent() == nil {
			t.Fatalf("%s parent is nil", p)
		}

		want := wantInherited
		if p == "config secrets" {
			want = []string{"json", "reveal"} // --env is shadowed
		}

		var inherited []string
		cmd.InheritedFlags().VisitAll(func(f *pflag.Flag) {
			if f.Name == "env" || f.Name == "json" || f.Name == "reveal" {
				inherited = append(inherited, f.Name)
			}
		})
		sort.Strings(inherited)
		if !reflect.DeepEqual(inherited, want) {
			t.Errorf("%s inherited flags = %v, want %v (parent: %s)", p, inherited, want, cmd.Parent().Name())
		}
	}
}

func TestCanonConfigRegistry(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		r, err := canon.Effective(compat.V15())
		if err != nil {
			t.Fatalf("canon.Effective: %v", err)
		}
		shims := []string{"env", "secrets", "oauth", "telemetry", "trust"}
		if compat.V15() {
			for _, s := range shims {
				entry, ok := r.Commands[s]
				if !ok {
					t.Errorf("missing registry entry for %s", s)
					continue
				}
				if entry.Canon != "deprecated-shim" {
					t.Errorf("%s canon = %q, want deprecated-shim", s, entry.Canon)
				}
			}
		} else {
			// in v1.4 they are core or pending (depending on what it was).
			// previously they were pending, so we check they are not missing.
			for _, s := range shims {
				_, ok := r.Commands[s]
				if !ok {
					t.Errorf("missing registry entry for %s", s)
				}
			}
		}
	})
}
