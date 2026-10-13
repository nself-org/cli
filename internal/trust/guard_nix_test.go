//go:build !darwin

package trust

import (
	"context"

	"github.com/nself-org/cli/internal/trust/sysexec"
)

func adminProbe(ctx context.Context) error {
	return sysexec.Run(ctx, "osascript", "-e", "return 1")
}
