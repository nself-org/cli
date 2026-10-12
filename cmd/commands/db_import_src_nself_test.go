package commands

import (
	"context"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/importer"
	"github.com/spf13/cobra"
	"strings"
	"testing"
)

func TestNselfBundleArgument(t *testing.T) {
	var cmd *cobra.Command
	for _, child := range migrateCmd.Commands() {
		if child.Name() == "nself" {
			cmd = child
		}
	}
	if cmd == nil {
		t.Fatal("nself source command missing")
	}
	if !strings.HasPrefix(cmd.Use, "nself <dir>") {
		t.Fatalf("usage does not show the directory: %q", cmd.Use)
	}
	if cmd.Args(cmd, nil) == nil || cmd.Args(cmd, []string{"a", "b"}) == nil || cmd.Args(cmd, []string{"a"}) != nil {
		t.Fatal("the command must take exactly one directory")
	}
	if flag := cmd.Flags().Lookup("dir"); flag == nil || !flag.Hidden {
		t.Fatal("the dir value flag must exist and stay hidden")
	}
	old := importRun
	defer func() {
		importRun = old
		_ = cmd.Flags().Set("dir", "")
		_ = cmd.Flags().Set("json", "false")
	}()
	var got string
	importRun = func(_ context.Context, _ *config.Config, s importer.Source, in importer.Input, _ importer.RunOptions) (importer.Report, error) {
		got = in.Values["dir"]
		return importer.Report{Source: s.Name(), Result: "pass"}, nil
	}
	cmd.SetContext(context.Background())
	_ = cmd.Flags().Set("json", "true")
	for _, preset := range []string{"", "/ignored"} {
		got = ""
		_ = cmd.Flags().Set("dir", preset)
		if _, err := captureHasuraOutput(func() error { return cmd.RunE(cmd, []string{"/some/bundle"}) }); err != nil {
			t.Fatal(err)
		}
		if got != "/some/bundle" {
			t.Fatalf("the positional argument must win over --dir %q, got %q", preset, got)
		}
	}
}
