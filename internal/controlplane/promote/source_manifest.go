package promote

// Purpose: resolve the exact compose manifest model and capture the host release.
// Inputs: validated manifest paths, injected command runner, resolved declarations.
// Outputs: Docker's interpolated image model and manifest-aware source capture.
// Constraints: no host commands execute except through the caller's runner.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/deploy"
	remotesdk "github.com/nself-org/cli/sdk/go/v2/remote"
)

// ResolvedComposeImages reads Docker's merged, interpolated config model.
func ResolvedComposeImages(ctx context.Context, run Runner, files, envFiles []string) ([]Image, error) {
	base, err := composeLocalCommand(files, envFiles)
	if err != nil {
		return nil, err
	}
	// Service env_file paths name target files. Their container environment does
	// not affect image interpolation, which uses the ordered CLI env-file list.
	raw, err := run(ctx, base+" config --format json --no-env-resolution")
	if err != nil {
		return nil, fmt.Errorf("resolve promotion compose: %w", err)
	}
	return parseResolvedImages(raw)
}

// parseResolvedImages preserves service overrides from Docker's resolved model.
func parseResolvedImages(raw []byte) ([]Image, error) {
	var doc struct {
		Services map[string]struct {
			Image    string          `json:"image"`
			Platform string          `json:"platform"`
			Build    json.RawMessage `json:"build"`
		} `json:"services"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse promotion compose: %w", err)
	}
	images := make([]Image, 0, len(doc.Services))
	for service, value := range doc.Services {
		images = append(images, Image{Service: service, Ref: value.Image, Platform: value.Platform, Build: len(value.Build) > 0 && string(value.Build) != "null"})
	}
	sort.Slice(images, func(i, j int) bool { return images[i].Service < images[j].Service })
	return images, nil
}

// CaptureSourceManifest reads the source manifest and captures its image listing.
// Inspection is delayed until declarations from the scratch model are available.
func CaptureSourceManifest(ctx context.Context, run Runner, root string) (Runner, []byte, error) {
	if err := deploy.ValidateRemotePath(root); err != nil {
		return nil, nil, err
	}
	files, err := sourceManifest(ctx, run, root, "compose-files.txt", []string{path.Join(root, "docker-compose.yml")})
	if err != nil {
		return nil, nil, err
	}
	envs, err := sourceManifest(ctx, run, root, "compose-env-files.txt", nil)
	if err != nil {
		return nil, nil, err
	}
	base, err := composeManifestCommand(files, envs)
	if err != nil {
		return nil, nil, err
	}
	captured := func(c context.Context, command string) ([]byte, error) {
		if strings.HasPrefix(command, "docker compose ") {
			command = base + strings.TrimPrefix(command, "docker compose")
		}
		return run(c, command)
	}
	raw, err := captured(ctx, "docker compose images --format json")
	if err != nil {
		return captured, raw, err
	}
	// Images rows without a service are mapped through ps. Capture that listing
	// now and replay it, so a container recreated or renamed before the compare
	// cannot be mapped to another service.
	if rows, perr := parseComposeImages(raw); perr == nil && missingService(rows) {
		ps, err := captured(ctx, composePSCommand)
		if err != nil {
			return captured, raw, fmt.Errorf("list source compose containers: %w", err)
		}
		replay := func(c context.Context, command string) ([]byte, error) {
			if command == composePSCommand {
				return ps, nil
			}
			return captured(c, command)
		}
		return replay, raw, nil
	}
	return captured, raw, nil
}

func missingService(images []composeImage) bool {
	for _, image := range images {
		if image.Service == "" {
			return true
		}
	}
	return false
}

// sourceManifest reads optional host manifests; every entry is validated before use.
func sourceManifest(ctx context.Context, run Runner, root, name string, fallback []string) ([]string, error) {
	manifest := path.Join(root, ".nself", name)
	raw, err := run(ctx, "if [ -f "+remotesdk.ShellQuote(manifest)+" ]; then cat "+remotesdk.ShellQuote(manifest)+"; else printf '__NSELF_ABSENT__'; fi")
	if err != nil {
		return nil, err
	}
	if string(raw) == "__NSELF_ABSENT__" {
		return fallback, nil
	}
	var entries []string
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !path.IsAbs(line) {
			line = path.Join(root, line)
		}
		if err := deploy.ValidateRemotePath(line); err != nil {
			return nil, err
		}
		entries = append(entries, line)
	}
	if len(entries) == 0 && len(fallback) > 0 {
		return nil, fmt.Errorf("empty source compose manifest %s", manifest)
	}
	return entries, nil
}

// composeManifestCommand builds the compose prefix for host-side manifest paths.
func composeManifestCommand(files, envFiles []string) (string, error) {
	return composeCommand(files, envFiles, deploy.ValidateRemotePath)
}

// composeLocalCommand builds the compose prefix for paths on the operator machine.
// Local paths keep their platform form (spaces, drive letters, backslashes), so
// the remote charset does not apply; single quoting is the shell boundary.
func composeLocalCommand(files, envFiles []string) (string, error) {
	return composeCommand(files, envFiles, validateLocalPath)
}

// validateLocalPath refuses only what quoting cannot carry: empty paths and NUL.
func validateLocalPath(p string) error {
	if p == "" || strings.ContainsRune(p, 0) {
		return fmt.Errorf("invalid local compose path %q", p)
	}
	return nil
}

func composeCommand(files, envFiles []string, validate func(string) error) (string, error) {
	if len(files) == 0 {
		return "", fmt.Errorf("no compose files for promotion")
	}
	base := "docker compose"
	for _, list := range []struct {
		flag  string
		paths []string
	}{{" -f ", files}, {" --env-file ", envFiles}} {
		for _, file := range list.paths {
			if err := validate(file); err != nil {
				return "", err
			}
			base += list.flag + remotesdk.ShellQuote(file)
		}
	}
	return base, nil
}

// ResolvedArtifactImages resolves exactly the staged compose and environment set.
// Host-owned env files are read privately and removed after interpolation.
func ResolvedArtifactImages(ctx context.Context, local, host Runner, set deploy.ArtifactSet) ([]Image, error) {
	files, envs := []string{}, []string{}
	for _, remote := range set.Compose {
		if err := deploy.ValidateRemotePath(remote); err != nil {
			return nil, err
		}
		rel := strings.TrimPrefix(remote, set.RemotePath+"/")
		if rel == remote {
			return nil, fmt.Errorf("compose path outside artifact root: %s", remote)
		}
		files = append(files, filepath.Join(set.StageDir, filepath.FromSlash(rel)))
	}
	envManifest := set.EnvManifest
	if len(envManifest) == 0 {
		envManifest = []string{path.Join(set.RemotePath, ".env")}
	}
	for _, remote := range envManifest {
		if err := deploy.ValidateRemotePath(remote); err != nil {
			return nil, err
		}
		rel := strings.TrimPrefix(remote, set.RemotePath+"/")
		if rel == remote {
			return nil, fmt.Errorf("env path outside artifact root: %s", remote)
		}
		file := filepath.Join(set.StageDir, filepath.FromSlash(rel))
		if rel == ".env" && (set.KeepHostEnv || len(set.EnvManifest) == 0) {
			raw, err := host(ctx, "if [ -f "+remotesdk.ShellQuote(remote)+" ]; then cat "+remotesdk.ShellQuote(remote)+"; fi")
			if err != nil {
				return nil, err
			}
			tmp, err := os.CreateTemp("", "promote-host-env-*")
			if err != nil {
				return nil, err
			}
			defer func() { _ = os.Remove(tmp.Name()) }()
			if _, err := tmp.Write(raw); err != nil {
				_ = tmp.Close()
				return nil, err
			}
			if err := tmp.Close(); err != nil {
				return nil, err
			}
			file = tmp.Name()
		}
		envs = append(envs, file)
	}
	return ResolvedComposeImages(ctx, local, files, envs)
}
