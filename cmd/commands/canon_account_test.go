package commands

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestCanonAccountResolution(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		undo := prepareTreeWith(&canonTable, RootCmd, compat.V15(), false)
		defer undo()

		paths := []string{
			"account", "account devices", "account licenses", "account login",
			"account logout", "account status", "account team", "account transfer",
			"login", "logout",
		}
		for _, old := range paths {
			newPath := "license " + old
			for _, input := range []string{old, newPath} {
				t.Run(strings.ReplaceAll(input, " ", "_"), func(t *testing.T) {
					args, _, err := rewriteCanonArgsWith(&canonTable, RootCmd, strings.Fields(input), compat.V15())
					if err != nil {
						t.Fatalf("rewrite %s: %v", input, err)
					}
					cmd, _, err := RootCmd.Find(args)
					if err != nil {
						t.Fatalf("find %s: %v", input, err)
					}
					want := "nself " + old
					if compat.V15() {
						want = "nself " + newPath
					} else if input == newPath {
						// The existing runnable license command consumes unknown tails in v1.4.
						want = "nself license"
					}
					if got := cmd.CommandPath(); got != want {
						t.Errorf("resolved %s to %q, want %q", input, got, want)
					}
				})
			}
		}
		if compat.V15() {
			for _, tt := range []struct {
				path string
				want *cobra.Command
			}{
				{"license login", loginCmd},
				{"license logout", logoutCmd},
				{"license account login", accountLoginCmd},
				{"license account logout", accountLogoutCmd},
			} {
				cmd, _, err := RootCmd.Find(strings.Fields(tt.path))
				if err != nil || cmd != tt.want {
					t.Errorf("%s resolved to %v, want original command %v (err %v)", tt.path, cmd, tt.want, err)
				}
			}
		}
	})
}

func TestCanonAccountInheritedFlags(t *testing.T) {
	undo := prepareTreeWith(&canonTable, RootCmd, true, false)
	defer undo()

	gotLicenseFlags := 0
	licenseCmd.PersistentFlags().VisitAll(func(*pflag.Flag) { gotLicenseFlags++ })
	if got := gotLicenseFlags; got != 0 {
		t.Fatalf("license gained %d persistent flags; update the exact inherited flag contract", got)
	}
	var want []string
	RootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) { want = append(want, f.Name) })
	sort.Strings(want)
	for _, path := range []string{"license account", "license account status", "license account licenses", "license account login", "license account logout", "license login", "license logout"} {
		cmd, _, err := RootCmd.Find(strings.Fields(path))
		if err != nil {
			t.Fatalf("find %s: %v", path, err)
		}
		var got []string
		cmd.InheritedFlags().VisitAll(func(f *pflag.Flag) { got = append(got, f.Name) })
		sort.Strings(got)
		wantForPath := want
		if path == "license account status" || path == "license account licenses" {
			wantForPath = []string{"no-deprecation-warnings", "no-monorepo"} // local --json shadows root --json
		}
		if !reflect.DeepEqual(got, wantForPath) {
			t.Errorf("%s inherited flags = %v, want %v", path, got, wantForPath)
		}
	}
}

func TestCanonAccountRegistry(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		r, err := canon.Effective(compat.V15())
		if err != nil {
			t.Fatalf("canon.Effective: %v", err)
		}
		for old, target := range map[string]string{
			"account": "license account", "login": "license login", "logout": "license logout",
		} {
			e, ok := r.Commands[old]
			if !ok {
				t.Errorf("missing registry entry for %s", old)
				continue
			}
			if compat.V15() {
				if e.Canon != canon.CanonShim || e.Target != target {
					t.Errorf("%s = canon %q, target %q; want deprecated-shim to %s", old, e.Canon, e.Target, target)
				}
				if _, ok := r.Commands[target]; !ok {
					t.Errorf("missing canonical registry entry for %s", target)
				}
			} else if e.Canon != canon.CanonPending {
				t.Errorf("%s canon = %q, want pending in v1.4", old, e.Canon)
			}
		}
	})
}
