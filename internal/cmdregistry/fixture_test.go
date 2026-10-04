package cmdregistry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/spf13/cobra"
)

func noop(*cobra.Command, []string) error { return nil }

// fixtureRoot builds the fixture cobra tree: a hub with runnable and
// non-runnable children, a hidden shim with a target, persistent, local,
// required, hidden and deprecated flags, and the three Use forms
// `get <key>`, `status [SERVICE]`, `add <key> [key...]`. With reverse the
// children are added in the opposite order.
func fixtureRoot(reverse bool) *cobra.Command {
	root := &cobra.Command{Use: "nself", Short: "fixture root"}
	root.PersistentFlags().BoolP("verbose", "v", false, "verbose output")
	root.Flags().String("config", "", "config file")
	root.AddGroup(&cobra.Group{ID: "core", Title: "Core"})

	status := &cobra.Command{Use: "status [SERVICE]", Short: "show status", RunE: noop}
	status.Flags().Bool("json", false, "print JSON")
	status.Flags().BoolP("watch", "w", false, "watch")
	status.Flags().String("debugx", "", "debug")
	_ = status.Flags().MarkHidden("debugx")
	status.Flags().String("old", "", "old flag")
	_ = status.Flags().MarkDeprecated("old", "use --watch")
	status.PersistentFlags().String("scope", "all", "scope")

	doctor := &cobra.Command{Use: "doctor", Short: "diagnose", RunE: noop}
	doctor.Flags().Bool("fix", false, "apply fixes")
	doctor.Flags().Bool("ai", false, "ai mode")
	doctor.Flags().Bool("install-check", false, "install check")

	logs := &cobra.Command{Use: "logs [SERVICE]", Short: "tail logs", RunE: noop}
	logs.Flags().Bool("follow", false, "follow")

	add := &cobra.Command{Use: "add <key> [key...]", Short: "add things", RunE: noop}
	add.Flags().String("name", "", "name")
	_ = add.MarkFlagRequired("name")

	hub := &cobra.Command{Use: "hub", Short: "a hub", GroupID: "core"}
	get := &cobra.Command{Use: "get <key>", Short: "get a key", RunE: noop}
	get.Flags().String("format", "text", "output format")
	hub.AddCommand(get, &cobra.Command{Use: "list", Short: "list keys", RunE: noop})

	oldstatus := &cobra.Command{Use: "oldstatus", Short: "old status", Hidden: true, Deprecated: "use status", Aliases: []string{"ost", "os"}, RunE: noop}

	deploy := &cobra.Command{Use: "deploy", Short: "deploy", RunE: noop}
	deploy.Flags().Bool("stream", false, "stream progress")

	all := []*cobra.Command{status, doctor, logs, add, hub, oldstatus, deploy}
	if reverse {
		for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
			all[i], all[j] = all[j], all[i]
		}
	}
	root.AddCommand(all...)
	root.InitDefaultHelpCmd()
	return root
}

func fixtureCanon(t testing.TB) *canon.File {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "canon.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := canon.Parse(data)
	if err != nil {
		t.Fatalf("fixture canon: %v", err)
	}
	return f
}

func fixtureTypes() map[string]any {
	return map[string]any{"hub get": struct{}{}, "status": struct{}{}}
}

func fixtureOpts(v15 bool) BuildOptions {
	return BuildOptions{V15: v15, V15OnlyEnvelope: map[string]bool{"hub get": true}}
}

func mustBuild(t testing.TB, root *cobra.Command, v15 bool) *Registry {
	t.Helper()
	r, err := Build(root, fixtureCanon(t), fixtureTypes(), fixtureOpts(v15))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return r
}

func mustMarshal(t testing.TB, r *Registry) []byte {
	t.Helper()
	b, err := r.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return b
}
