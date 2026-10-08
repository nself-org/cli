package commands

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
)

func TestCanonDataResolution(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		undo := prepareTreeWith(&canonTable, RootCmd, compat.V15(), false)
		defer undo()

		rows := []struct{ from, to string }{
			{"generate", "db generate"},
			{"migrate detect", "update project detect"},
			{"migrate firebase", "db import firebase"},
			{"migrate from-bash", "update project from-bash"},
			{"migrate from-v099", "update project from-v099"},
			{"migrate generate", "db migrate generate"},
			{"migrate rollback", "update project rollback"},
			{"migrate run", "update project run"},
			{"migrate supabase", "db import supabase"},
			{"migrate watch", "db migrate watch"},
			{"template", "init template"},
			{"template info", "init template info"},
			{"template list", "init template list"},
			{"template publish", "init template publish"},
			{"template update", "init template update"},
			{"migrate", "db"},
		}
		for _, row := range rows {
			t.Run(strings.ReplaceAll(row.from, " ", "_"), func(t *testing.T) {
				args, _, err := rewriteCanonArgsWith(&canonTable, RootCmd, strings.Fields(row.from), compat.V15())
				if err != nil {
					t.Fatal(err)
				}
				cmd, _, err := RootCmd.Find(args)
				if err != nil {
					t.Fatal(err)
				}
				want := "nself " + row.from
				if compat.V15() {
					want = "nself " + row.to
				}
				if got := cmd.CommandPath(); got != want {
					t.Errorf("path = %q, want %q", got, want)
				}
			})
		}
	})
}

func TestCanonDataInheritedFlags(t *testing.T) {
	undo := prepareTreeWith(&canonTable, RootCmd, true, false)
	defer undo()

	for _, path := range []string{"db migrate generate", "db migrate watch"} {
		t.Run(strings.ReplaceAll(path, " ", "_"), func(t *testing.T) {
			cmd, _, err := RootCmd.Find(strings.Fields(path))
			if err != nil {
				t.Fatal(err)
			}
			if cmd.CommandPath() != "nself "+path {
				t.Fatalf("path = %q", cmd.CommandPath())
			}
			flag := cmd.InheritedFlags().Lookup("plugin")
			if flag == nil || flag.Value.Type() != "string" {
				t.Errorf("%s does not inherit db migrate's string --plugin flag", path)
			}
		})
	}
}

func TestCanonDataRegistry(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		r, err := canon.Effective(compat.V15())
		if err != nil {
			t.Fatal(err)
		}
		rows := map[string]string{
			"generate": "db generate", "migrate": "db",
			"migrate detect":    "update project detect",
			"migrate firebase":  "db import firebase",
			"migrate from-bash": "update project from-bash",
			"migrate from-v099": "update project from-v099",
			"migrate generate":  "db migrate generate",
			"migrate rollback":  "update project rollback",
			"migrate run":       "update project run",
			"migrate supabase":  "db import supabase",
			"migrate watch":     "db migrate watch",
			"template":          "init template",
		}
		for from, to := range rows {
			entry, ok := r.Commands[from]
			if !ok {
				t.Errorf("missing source %q", from)
				continue
			}
			if compat.V15() {
				if entry.Canon != canon.CanonShim || entry.Target != to {
					t.Errorf("%s = (%s, %s), want shim to %s", from, entry.Canon, entry.Target, to)
				}
				if _, ok := r.Commands[to]; !ok {
					t.Errorf("missing target %q", to)
				}
			} else if entry.Canon == canon.CanonShim {
				t.Errorf("%s unexpectedly shimmed in v1.4", from)
			}
		}
		if compat.V15() {
			if got := r.Commands["update project run"].SideEffect; got != canon.SideEffectWrite {
				t.Errorf("update project run side_effect = %q, want write", got)
			}
			if got := r.Commands["update project rollback"].SideEffect; got != canon.SideEffectDestructive {
				t.Errorf("update project rollback side_effect = %q, want destructive", got)
			}
		}
	})
}
