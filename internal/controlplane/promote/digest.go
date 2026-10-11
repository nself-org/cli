package promote

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy/archguard"
	remotesdk "github.com/nself-org/cli/sdk/go/v2/remote"
)

// Runner executes a validated command locally or through a deploy.Remote.
type Runner func(context.Context, string) ([]byte, error)

// Image identifies one generated compose service and whether it has a build context.
type Image struct {
	Service string
	Ref     string
	Build   bool
}

// SourceImage is the per-platform identity captured from a running source.
type SourceImage struct {
	Service  string
	Ref      string
	Platform string
	Digest   string
}

type composeImage struct {
	Service    string `json:"Service"`
	Repository string `json:"Repository"`
	Tag        string `json:"Tag"`
	ID         string `json:"ID"`
}

// CaptureSource reads running compose images and inspects their platform identities.
//
// Purpose: derive release identity from what the source host actually runs.
// Inputs: an injected command runner, generated service declarations, optional registry lookup.
// Outputs: one source image identity per declared service, sorted by service.
// Constraints: commands contain only shell-quoted image references; no process is started here.
func CaptureSource(ctx context.Context, run Runner, services []Image, lookup archguard.Lookup) ([]SourceImage, error) {
	if run == nil {
		return nil, fmt.Errorf("promote: source image runner is nil")
	}
	raw, err := run(ctx, "docker compose images --format json")
	if err != nil {
		return nil, fmt.Errorf("capture source compose images: %w", err)
	}
	running, err := parseComposeImages(raw)
	if err != nil {
		return nil, err
	}
	refs := make(map[string]string, len(running))
	for _, image := range running {
		ref := image.Repository
		if image.Tag != "" && image.Tag != "<none>" {
			ref += ":" + image.Tag
		}
		if image.Service != "" {
			refs[image.Service] = ref + "\x00" + image.ID
		}
	}
	var out []SourceImage
	for _, service := range services {
		if service.Build {
			continue
		}
		captured := refs[service.Service]
		if captured == "" {
			return nil, fmt.Errorf("source compose image missing service %s", service.Service)
		}
		ref, imageID, _ := strings.Cut(captured, "\x00")
		if imageID == "" {
			return nil, fmt.Errorf("source compose image missing id for service %s", service.Service)
		}
		identity, err := inspectSource(ctx, run, lookup, service.Service, ref, imageID)
		if err != nil {
			return nil, err
		}
		out = append(out, identity)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Service < out[j].Service })
	return out, nil
}

func inspectSource(ctx context.Context, run Runner, lookup archguard.Lookup, service, ref, imageID string) (SourceImage, error) {
	command := "docker image inspect --format '{{json .RepoDigests}}|{{.Os}}/{{.Architecture}}' " + remotesdk.ShellQuote(imageID)
	raw, err := run(ctx, command)
	if err != nil {
		return SourceImage{}, fmt.Errorf("inspect source service %s: %w", service, err)
	}
	digest, platform, err := parseInspect(raw, ref)
	if err != nil {
		return SourceImage{}, fmt.Errorf("inspect source service %s: %w", service, err)
	}
	resolved, err := sourcePlatformDigest(ctx, ref, digest, platform, lookup)
	if err != nil {
		return SourceImage{}, fmt.Errorf("resolve source service %s platform: %w", service, err)
	}
	return SourceImage{Service: service, Ref: ref, Platform: platform, Digest: resolved}, nil
}

func sourcePlatformDigest(ctx context.Context, ref, inspected, platform string, lookup archguard.Lookup) (string, error) {
	if err := compose.LockError(); err != nil {
		return "", err
	}
	for _, locked := range compose.LockedImages() {
		if ref == locked.String() || ref == locked.LegacyRef {
			if digest := locked.Platforms[platform]; digest != "" {
				return digest, nil
			}
			return "", fmt.Errorf("%s has no %s manifest", ref, platform)
		}
	}
	if lookup == nil {
		return "", fmt.Errorf("registry lookup is required for unlocked image %s", ref)
	}
	manifestRef := imageRepository(ref) + "@" + inspected
	platforms, err := lookup(ctx, manifestRef)
	if err != nil {
		return "", err
	}
	if platforms[platform] == "" {
		return "", fmt.Errorf("%s has no %s manifest", manifestRef, platform)
	}
	return platforms[platform], nil
}

// CompareDigests proves that source and target resolve to equal platform manifests.
//
// Purpose: reject drift and production build-context ambiguity before snapshot.
// Inputs: captured source identities, target image declarations, arch, tier and lookup.
// Outputs: deterministic comparison rows and E490 when identity is not proven.
// Constraints: build-context services warn below prod and fail closed on prod.
func CompareDigests(ctx context.Context, source []SourceImage, target []Image, targetArch string,
	tier controlplane.Tier, lookup archguard.Lookup, warn func(string)) ([]DigestRow, error) {
	if lookup == nil {
		lookup = archguard.RegistryLookup
	}
	sourceByService := make(map[string]SourceImage, len(source))
	for _, image := range source {
		sourceByService[image.Service] = image
	}
	var rows []DigestRow
	var drift, unverifiable []string
	for _, image := range target {
		if image.Build {
			unverifiable = append(unverifiable, image.Service)
			rows = append(rows, DigestRow{Service: image.Service, Status: "unverifiable"})
			continue
		}
		src, ok := sourceByService[image.Service]
		if !ok {
			drift = append(drift, image.Service)
			rows = append(rows, DigestRow{Service: image.Service, Target: image.Ref, Status: "missing-source"})
			continue
		}
		delete(sourceByService, image.Service)
		targetDigest, err := targetPlatformDigest(ctx, image.Ref, "linux/"+targetArch, lookup)
		row := DigestRow{Service: image.Service, Source: src.Digest, Target: targetDigest, Status: "equal"}
		if err != nil || src.Digest == "" || src.Digest != targetDigest {
			row.Status = "drift"
			drift = append(drift, image.Service)
		}
		rows = append(rows, row)
	}
	for service, src := range sourceByService {
		drift = append(drift, service)
		rows = append(rows, DigestRow{Service: service, Source: src.Digest, Status: "missing-target"})
	}
	sortRows(rows)
	sort.Strings(drift)
	sort.Strings(unverifiable)
	if len(unverifiable) > 0 && tier == controlplane.TierProd {
		return rows, releaseDriftError(nil, unverifiable)
	}
	if len(unverifiable) > 0 && warn != nil {
		warn("unverifiable build-context services: " + strings.Join(unverifiable, ", "))
	}
	if len(drift) > 0 {
		return rows, releaseDriftError(drift, nil)
	}
	return rows, nil
}

func targetPlatformDigest(ctx context.Context, ref, platform string, lookup archguard.Lookup) (string, error) {
	if err := compose.LockError(); err != nil {
		return "", err
	}
	for _, locked := range compose.LockedImages() {
		if ref == locked.String() || ref == locked.LegacyRef {
			if digest := locked.Platforms[platform]; digest != "" {
				return digest, nil
			}
			return "", fmt.Errorf("%s has no %s manifest", ref, platform)
		}
	}
	platforms, err := lookup(ctx, ref)
	if err != nil {
		return "", err
	}
	if platforms[platform] == "" {
		return "", fmt.Errorf("%s has no %s manifest", ref, platform)
	}
	return platforms[platform], nil
}
