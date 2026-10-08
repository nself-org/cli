package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/spf13/cobra"
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

func TestCanonRetiredHubsBareHelp(t *testing.T) {
	raw, err := canon.LoadRaw()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw.RetiredHubs) == 0 || len(raw.RetiredHubs) != len(canonTable.RetiredHubs) {
		t.Fatalf("raw retired rows=%d, generated rows=%d", len(raw.RetiredHubs), len(canonTable.RetiredHubs))
	}
	for _, row := range raw.RetiredHubs {
		t.Run(row.From, func(t *testing.T) {
			root := &cobra.Command{Use: "nself", SilenceUsage: true, SilenceErrors: true}
			old := &cobra.Command{Use: row.From, RunE: func(c *cobra.Command, _ []string) error { return c.Help() }}
			oldChild := &cobra.Command{Use: "child", RunE: func(*cobra.Command, []string) error { return nil }}
			old.AddCommand(oldChild)
			root.AddCommand(old)
			parent := root
			called := false
			for _, part := range strings.Fields(row.To) {
				child := &cobra.Command{Use: part}
				parent.AddCommand(child)
				parent = child
			}
			parent.AddCommand(&cobra.Command{Use: "child"})
			table := canonTableT{RetiredHubs: []canonRowT{{From: strings.Fields(row.From), To: strings.Fields(row.To)}}}
			for _, move := range canonTable.Moves {
				if strings.Join(move.To, " ") == row.To && strings.HasPrefix(strings.Join(move.From, " "), row.From+" ") {
					oldChild.Use = strings.TrimPrefix(strings.Join(move.From, " "), row.From+" ")
					table.Moves = append(table.Moves, move)
					parent.RunE = func(*cobra.Command, []string) error { called = true; return nil }
				}
			}
			args, notes, err := rewriteCanonArgsWith(&table, root, strings.Fields(row.From), true)
			if err != nil || len(notes) != 1 || notes[0].Kind != "retired" {
				t.Fatalf("rewrite %q: %v, notes %+v", row.From, err, notes)
			}
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(args)
			if err := root.Execute(); err != nil || called || !strings.Contains(out.String(), "Usage:") {
				t.Fatalf("bare %q: err=%v RunE=%v output=%q", row.From, err, called, out.String())
			}
		})
	}
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
