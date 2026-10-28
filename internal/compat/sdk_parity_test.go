package compat

import (
	"testing"

	sdkcompat "github.com/nself-org/cli/sdk/go/v2/compat"
)

// TestSDKCopyAgreesInCLIProcess pins D-0705. The CLI links sdk/go/compat (the
// pin store ancestor gate in sdk/go/remote reads it), and that copy reads only
// NSELF_V15. Today both copies apply the same rule, so they agree for every
// value. If P7-SHIP flips this package's default without the sdk copy following
// (or the CLI exporting its resolved mode), the CLI would print the v1.4
// ancestor warning where it must refuse. This test fails the moment they
// diverge, so the flip has to handle the sdk copy in the same change.
func TestSDKCopyAgreesInCLIProcess(t *testing.T) {
	for _, v := range []string{"<unset>", "", "0", "1", "true", "TRUE", "True", "yes", "on", " 1"} {
		t.Run(v, func(t *testing.T) {
			if v == "<unset>" {
				t.Setenv(EnvVar, "x")
				unset(t)
			} else {
				t.Setenv(EnvVar, v)
			}
			if cli, sdk := V15(), sdkcompat.V15(); cli != sdk {
				t.Fatalf("NSELF_V15=%q: internal/compat.V15() = %v but sdk/go/compat.V15() = %v; "+
					"the CLI process links both (sdk/go/remote/knownhosts_compat.go, D-0484): "+
					"export the resolved mode to the sdk copy or delete the v1.4 branch in the same change (D-0705)", v, cli, sdk)
			}
		})
	}
}
