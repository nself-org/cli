package promote

// Purpose: preserve source compose platforms and normalize the target host architecture.
// Inputs: the captured source runner/listing, registry seam, or a host architecture probe.
// Outputs: running source identities and a normalized target architecture.
// Constraints: source config reads use its full manifest; all commands use injected runners.

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/deploy/archguard"
)

// CaptureManifestSource inspects the captured listing using the source's resolved declarations.
func CaptureManifestSource(ctx context.Context, run Runner, raw []byte, lookup archguard.Lookup) ([]SourceImage, error) {
	model, err := run(ctx, "docker compose config --format json --no-env-resolution")
	if err != nil {
		return nil, fmt.Errorf("resolve source compose: %w", err)
	}
	images, err := parseResolvedImages(model)
	if err != nil {
		return nil, err
	}
	replay := func(c context.Context, command string) ([]byte, error) {
		if command == "docker compose images --format json" {
			return raw, nil
		}
		return run(c, command)
	}
	return CaptureSource(ctx, replay, images, lookup)
}

// TargetArchitecture normalizes an inventory value or probes the target when absent.
func TargetArchitecture(ctx context.Context, configured string, run Runner) (string, error) {
	arch := strings.TrimSpace(configured)
	if arch == "" {
		raw, err := run(ctx, "uname -m")
		if err != nil {
			return "", err
		}
		arch = strings.TrimSpace(string(raw))
	}
	switch arch {
	case "x86_64":
		arch = "amd64"
	case "aarch64":
		arch = "arm64"
	}
	return arch, nil
}
