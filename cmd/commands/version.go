package commands

import (
	"fmt"
	"os"
	"runtime"

	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/ui"
	"github.com/nself-org/cli/internal/version"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version and system information",
	RunE: func(cmd *cobra.Command, args []string) error {
		short, _ := cmd.Flags().GetBool("short")
		jsonOut, _ := cmd.Flags().GetBool("json")

		// Dev-build warning: print before any output so it is always visible.
		// Goreleaser-built binaries inject a real Ed25519 pubkey via -X ldflag,
		// so IsZeroPubKey() returns false and the banner is suppressed.
		if license.IsZeroPubKey() {
			fmt.Fprintln(os.Stderr, "⚠️  WARN: dev build with no signing key — license verification is DISABLED")
			fmt.Fprintln(os.Stderr, "         Use a goreleaser-built binary in production.")
			fmt.Fprintln(os.Stderr, "")
		}

		ver := version.GetVersion()
		commit := version.GetCommit()
		buildDate := version.GetBuildDate()
		goVer := runtime.Version()
		platform := runtime.GOOS + "/" + runtime.GOARCH

		if short {
			fmt.Println(ver)
			return nil
		}

		if jsonOut {
			// capabilities is how a remote caller proves a behaviour (for
			// example db-dry-run-safe, P7-PROD-84); older builds omit it.
			return ui.PrintJSON(map[string]any{
				"version":      ver,
				"commit":       commit,
				"buildDate":    buildDate,
				"goVersion":    goVer,
				"platform":     platform,
				"capabilities": version.Capabilities(),
			})
		}

		// Default verbose output
		fmt.Printf("%s %s\n", ui.C(ui.Bold, "nSelf CLI"), ui.C(ui.Green, ver))
		fmt.Printf("  Commit:     %s\n", commit)
		fmt.Printf("  Built:      %s\n", buildDate)
		fmt.Printf("  Go version: %s\n", goVer)
		fmt.Printf("  Platform:   %s\n", platform)

		return nil
	},
}

func init() {
	versionCmd.Flags().Bool("short", false, "Print version number only")
	versionCmd.Flags().Bool("json", false, "Print version info as JSON")
	RootCmd.AddCommand(versionCmd)
}
