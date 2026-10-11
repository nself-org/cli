package promote

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy/archguard"
	"github.com/nself-org/cli/internal/errs"
	remotesdk "github.com/nself-org/cli/sdk/go/v2/remote"
)

// Runner executes a validated command locally or through a deploy.Remote.
type Runner func(context.Context, string) ([]byte, error)

// Image identifies one generated compose service and whether it has a build context.
type Image struct {
	Service  string
	Ref      string
	Platform string
	Build    bool
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
	if lookup == nil {
		lookup = archguard.RegistryLookup
	}
	declared := make(map[string]Image, len(services))
	for _, service := range services {
		declared[service.Service] = service
	}
	seen := make(map[string]bool, len(running))
	var out []SourceImage
	for _, image := range running {
		if image.Service == "" {
			return nil, fmt.Errorf("source compose image has no service name")
		}
		if seen[image.Service] {
			return nil, fmt.Errorf("source compose image repeats service %s", image.Service)
		}
		seen[image.Service] = true
		if declared[image.Service].Build {
			continue
		}
		ref := image.Repository
		if ref == "" {
			return nil, fmt.Errorf("source compose image missing repository for service %s", image.Service)
		}
		if image.Tag != "" && image.Tag != "<none>" {
			ref += ":" + image.Tag
		}
		if image.ID == "" {
			return nil, fmt.Errorf("source compose image missing id for service %s", image.Service)
		}
		identity, err := inspectSource(ctx, run, lookup, image.Service, ref, image.ID)
		if err != nil {
			return nil, err
		}
		if platform := declared[image.Service].Platform; platform != "" && platform != identity.Platform {
			return nil, errs.Newf("E490", "release drift: service %s runs %s but source compose selects %s", image.Service, identity.Platform, platform)
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
			lockedPlatform := locked.Platforms[platform]
			if inspected == locked.IndexDigest && lockedPlatform != "" {
				return lockedPlatform, nil
			}
			if inspected == lockedPlatform && inspected != "" {
				return inspected, nil
			}
			break
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
	if !manifestDigestRE.MatchString(platforms[platform]) {
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
	if len(target) == 0 {
		return nil, errs.New("E490", "release drift: target has zero services; nothing to prove")
	}
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
			delete(sourceByService, image.Service)
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
		platform := image.Platform
		if platform == "" {
			platform = "linux/" + targetArch
		}
		targetDigest, err := targetPlatformDigest(ctx, image.Ref, platform, lookup)
		if src.Platform != "" && src.Platform != platform {
			err = fmt.Errorf("source runs %s; target selects %s", src.Platform, platform)
		}
		row := DigestRow{Service: image.Service, Source: src.Digest, Target: targetDigest, Status: "equal"}
		if err != nil || src.Digest == "" || src.Digest != targetDigest {
			row.Status = "drift"
			detail := image.Service
			if err != nil {
				row.Detail = err.Error()
				detail += ": " + err.Error()
			}
			drift = append(drift, detail)
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
		_, pinnedDigest, pinned := strings.Cut(ref, "@")
		immutableLocked := ref == locked.String() || (pinned && pinnedDigest == locked.IndexDigest && sameRepository(imageRepository(ref), locked.Repository))
		if immutableLocked {
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
	if !manifestDigestRE.MatchString(platforms[platform]) {
		return "", fmt.Errorf("%s has no %s manifest", ref, platform)
	}
	return platforms[platform], nil
}
