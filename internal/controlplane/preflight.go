package controlplane

// Purpose: refuse incompatible image/host architectures before deployment.
// Inputs: target inventory, environment, and generated compose files.
// Outputs: nil or E491 naming every unverified image and host.
// Constraints: only local file reads, SSH uname, and registry manifest reads.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/deploy/archguard"
	"github.com/nself-org/cli/internal/errs"
	"gopkg.in/yaml.v3"
)

type archReader interface {
	Architecture(context.Context, Server) (string, error)
}

// Preflight checks all remote hosts in one environment against the compose images.
func Preflight(ctx context.Context, inv *Inventory, env string, composeFiles []string) error {
	root := "."
	if len(composeFiles) > 0 {
		root = filepath.Dir(composeFiles[0])
	}
	return preflightWith(ctx, inv, env, composeFiles, NewSSHProber(root, false), archguard.RegistryLookup)
}

// ComposeFilesForDeploy includes every generated compose fragment in its
// recorded merge order. A missing listed file fails closed in composeImages.
func ComposeFilesForDeploy(base string) ([]string, error) {
	manifest := filepath.Join(filepath.Dir(base), ".nself", "compose-files.txt")
	data, err := os.ReadFile(manifest)
	if os.IsNotExist(err) {
		return []string{base}, nil
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if !filepath.IsAbs(line) {
				line = filepath.Join(filepath.Dir(base), line)
			}
			files = append(files, line)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("empty compose manifest %s", manifest)
	}
	return files, nil
}

func preflightWith(ctx context.Context, inv *Inventory, env string, files []string, reader archReader, lookup archguard.Lookup) error {
	scoped, err := ScopeToEnv(inv, env)
	if err != nil {
		return err
	}
	if scoped.Environments[env].Kind == "local" {
		return nil
	}
	images, err := composeImages(files)
	if err != nil {
		return errs.New("E491", fmt.Sprintf("cannot read deploy images: %v", err))
	}
	needsCheck := false
	for _, image := range images {
		needsCheck = needsCheck || !image.Build
	}
	if !needsCheck {
		return nil
	}
	if lookup == nil {
		lookup = archguard.RegistryLookup
	}
	type lookupResult struct {
		platforms map[string]string
		err       error
	}
	lookedUp := map[string]lookupResult{}
	cachedLookup := func(ctx context.Context, ref string) (map[string]string, error) {
		if prior, ok := lookedUp[ref]; ok {
			return prior.platforms, prior.err
		}
		platforms, err := lookup(ctx, ref)
		lookedUp[ref] = lookupResult{platforms, err}
		return platforms, err
	}
	// Check shares a lookup cache per architecture across hosts by checking
	// all image references only once for each distinct architecture.
	byArch := map[string][]archguard.Mismatch{}
	var failures []string
	for _, server := range scoped.Environments[env].Servers {
		if server.Host == "" {
			continue
		}
		arch, archErr := reader.Architecture(ctx, server)
		if archErr != nil {
			failures = append(failures, fmt.Sprintf("host %s (%s): architecture unknown: %v", server.Name, server.Host, archErr))
			continue
		}
		mismatches, ok := byArch[arch]
		if !ok {
			mismatches, err = archguard.Check(ctx, images, arch, cachedLookup)
			if err != nil {
				failures = append(failures, fmt.Sprintf("host %s (%s): %v", server.Name, server.Host, err))
				continue
			}
			byArch[arch] = mismatches
		}
		for _, mismatch := range mismatches {
			failures = append(failures, fmt.Sprintf("host %s (%s, %s): image %s (%s): %s", server.Name, server.Host, arch, mismatch.Image, mismatch.Service, mismatch.Reason))
		}
	}
	if len(failures) > 0 {
		return errs.New("E491", strings.Join(failures, "; "))
	}
	return nil
}

func composeImages(files []string) ([]archguard.ImageRef, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("no compose files")
	}
	services := map[string]archguard.ImageRef{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		var doc struct {
			Services map[string]struct {
				Image string `yaml:"image"`
				Build any    `yaml:"build"`
			} `yaml:"services"`
		}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		// Compose merges later files over earlier ones per key: an override
		// fragment without image or build keeps the service's earlier values.
		for name, service := range doc.Services {
			ref := services[name]
			ref.Service = name
			if service.Image != "" {
				ref.Ref = service.Image
			}
			if service.Build != nil {
				ref.Build = true
			}
			services[name] = ref
		}
	}
	images := make([]archguard.ImageRef, 0, len(services))
	for _, image := range services {
		images = append(images, image)
	}
	sort.Slice(images, func(i, j int) bool { return images[i].Service < images[j].Service })
	return images, nil
}
