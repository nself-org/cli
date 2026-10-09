// Package archguard checks image platforms before a remote deploy changes a host.
package archguard

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/docker"
)

// ImageRef is a compose service's image and build-context status.
type ImageRef struct {
	Service string
	Ref     string
	Build   bool
}

// Mismatch records an image that cannot be proved to run on a host.
type Mismatch struct {
	Service string
	Image   string
	Arch    string
	Reason  string
}

// Lookup returns the platforms published by a registry reference.
type Lookup func(context.Context, string) (map[string]string, error)

// RegistryLookup inspects an index without pulling any image.
func RegistryLookup(ctx context.Context, ref string) (map[string]string, error) {
	raw, err := docker.ImageIndexRaw(ctx, ref)
	if err == nil {
		info, parseErr := docker.ParseIndex(raw)
		if parseErr == nil {
			return info.Platforms, nil
		}
		err = parseErr
	}
	// ManifestInspect is useful when buildx is unavailable.
	raw, inspectErr := docker.ManifestInspect(ctx, ref, false)
	if inspectErr != nil {
		return nil, fmt.Errorf("imagetools: %v; manifest inspect: %w", err, inspectErr)
	}
	info, parseErr := docker.ParseIndex(raw)
	if parseErr == nil {
		return info.Platforms, nil
	}
	verbose, verboseErr := docker.ManifestInspect(ctx, ref, true)
	if verboseErr != nil {
		return nil, fmt.Errorf("manifest inspect: %v; verbose: %w", parseErr, verboseErr)
	}
	platforms, verboseErr := parseVerbosePlatforms(verbose)
	if verboseErr != nil {
		return nil, fmt.Errorf("manifest inspect: %v; verbose: %w", parseErr, verboseErr)
	}
	return platforms, nil
}

func parseVerbosePlatforms(raw []byte) (map[string]string, error) {
	type descriptor struct {
		Descriptor struct {
			Digest   string `json:"digest"`
			Platform struct {
				OS           string `json:"os"`
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"Descriptor"`
	}
	var entries []descriptor
	if err := json.Unmarshal(raw, &entries); err != nil {
		var single descriptor
		if err = json.Unmarshal(raw, &single); err != nil {
			return nil, err
		}
		entries = []descriptor{single}
	}
	platforms := map[string]string{}
	for _, entry := range entries {
		p := entry.Descriptor.Platform
		if p.OS == "linux" && p.Architecture != "" {
			platforms[p.OS+"/"+p.Architecture] = entry.Descriptor.Digest
		}
	}
	if len(platforms) == 0 {
		return nil, fmt.Errorf("no platform descriptors in verbose manifest")
	}
	return platforms, nil
}

// Check checks each distinct image. Locked references use embedded platform
// data; other references use lookup. Errors are returned as unknown mismatches.
func Check(ctx context.Context, images []ImageRef, arch string, lookup Lookup) ([]Mismatch, error) {
	if arch != "amd64" && arch != "arm64" {
		return nil, fmt.Errorf("unsupported host architecture %q", arch)
	}
	if err := compose.LockError(); err != nil {
		return nil, err
	}
	if lookup == nil {
		lookup = RegistryLookup
	}
	cache := map[string]struct {
		platforms map[string]string
		err       error
	}{}
	var mismatches []Mismatch
	for _, image := range images {
		if image.Build {
			continue
		}
		if image.Ref == "" {
			mismatches = append(mismatches, Mismatch{image.Service, image.Ref, arch, "no image reference"})
			continue
		}
		found, ok := cache[image.Ref]
		if !ok {
			for _, locked := range compose.LockedImages() {
				if image.Ref == locked.String() || image.Ref == locked.LegacyRef {
					found.platforms = locked.Platforms
					break
				}
			}
			if found.platforms == nil {
				found.platforms, found.err = lookup(ctx, image.Ref)
			}
			cache[image.Ref] = found
		}
		if found.err != nil {
			mismatches = append(mismatches, Mismatch{image.Service, image.Ref, arch, found.err.Error()})
		} else if found.platforms["linux/"+arch] == "" {
			mismatches = append(mismatches, Mismatch{image.Service, image.Ref, arch, "linux/" + arch + " manifest missing"})
		}
	}
	sort.Slice(mismatches, func(i, j int) bool {
		return strings.Compare(mismatches[i].Image+mismatches[i].Service, mismatches[j].Image+mismatches[j].Service) < 0
	})
	return mismatches, nil
}
