package commands

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/plugin"
	"github.com/spf13/cobra"
)

func TestPluginUnsignedURLFlag(t *testing.T) {
	for _, cmd := range []*cobra.Command{pluginInstallCmd, installCmd} {
		flag := cmd.Flags().Lookup("allow-unsigned")
		if flag == nil || flag.Value.Type() != "bool" || flag.DefValue != "false" {
			t.Fatalf("%s allow-unsigned flag = %v", cmd.Name(), flag)
		}
	}
	for _, allow := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "explicit"}[allow], func(t *testing.T) {
			t.Setenv("NSELF_V15", "1")
			ctx := plugin.WithUnsignedURLAllowed(context.Background(), allow)
			err := plugin.CheckUnsignedURLInstall(ctx)
			if allow && err != nil {
				t.Fatal(err)
			}
			if !allow && (err == nil || !strings.Contains(err.Error(), "E107")) {
				t.Fatalf("want E107, got %v", err)
			}
		})
	}
}
