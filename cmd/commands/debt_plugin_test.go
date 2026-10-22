package commands

import (
	"github.com/spf13/cobra"
	"reflect"
	"testing"
)

func TestRemotePluginFlagValue(t *testing.T) {
	for _, name := range []string{"claw", "../invalid"} {
		t.Run(name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "up"}
			cmd.Flags().String("plugin", name, "")
			cmd.Flags().String("migration-dir", "", "")
			in := []string{"db", "migrate", "up", "--migration-dir", "--plugin"}
			got, err := withRemotePluginFlag(cmd, in)
			if name == "../invalid" {
				if err == nil {
					t.Fatal("invalid plugin was not validated")
				}
				return
			}
			want := append(append([]string{}, in...), "--plugin", name)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("got %q, %v; want %q", got, err, want)
			}
		})
	}
}
