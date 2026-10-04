package cmdregistry

import (
	"fmt"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/spf13/cobra"
)

// syntheticTree returns a 400-command tree (20 hubs x 19 children + 20 hubs)
// with three flags per command, and its canon data.
func syntheticTree() (*cobra.Command, *canon.File) {
	root := &cobra.Command{Use: "nself", Short: "bench root"}
	f := &canon.File{SchemaVersion: 1, Commands: map[string]canon.Entry{}}
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("hub%02d", i)
		f.Verbs = append(f.Verbs, name)
		hub := &cobra.Command{Use: name, Short: "hub"}
		f.Commands[name] = canon.Entry{Canon: canon.CanonCore}
		for j := 0; j < 19; j++ {
			sub := &cobra.Command{Use: fmt.Sprintf("sub%02d <key> [key...]", j), Short: "sub", RunE: noop}
			sub.Flags().Bool("json", false, "json")
			sub.Flags().String("format", "text", "format")
			sub.Flags().StringP("name", "n", "", "name")
			hub.AddCommand(sub)
			f.Commands[fmt.Sprintf("%s sub%02d", name, j)] = canon.Entry{SideEffect: canon.SideEffectRead}
		}
		root.AddCommand(hub)
	}
	return root, f
}

// BenchmarkBuild measures Build on a 400-command synthetic tree (recorded for
// the P7-GUARD startup budgets).
func BenchmarkBuild(b *testing.B) {
	root, f := syntheticTree()
	if _, err := Build(root, f, nil, BuildOptions{}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Build(root, f, nil, BuildOptions{}); err != nil {
			b.Fatal(err)
		}
	}
}
