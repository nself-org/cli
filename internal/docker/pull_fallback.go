package docker

// Purpose: make locked image pulls diagnosable and keep Compose pointed at a
// digest-identical mirror when an upstream registry is unavailable.
// Inputs: the image lock, the resolved Compose configuration and a context.
// Outputs: image source, an actionable error, and a generated Compose override.
// Constraints: all Docker calls stay in this package; a mirror is used only at
// the lock digest, and no image is retagged.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const ImageOverrideFile = ".nself/state/image-overrides.yml"

// LockedImage is the Docker-facing projection of an image-lock entry. Keeping
// this value here preserves the L0 Docker funnel's dependency direction.
type LockedImage struct {
	Name, Repository, Version, IndexDigest string
	Mirror                                 *string
}

func (r LockedImage) String() string { return r.Repository + ":" + r.Version + "@" + r.IndexDigest }

// Diagnose turns Docker and registry errors into stable operator diagnoses.
func Diagnose(message string) string {
	s := strings.ToLower(message)
	switch {
	case hasHTTPStatus(s, "429"), strings.Contains(s, "toomanyrequests"), strings.Contains(s, "rate limit"):
		return "rate limited"
	case hasHTTPStatus(s, "401"), strings.Contains(s, "unauthorized"), strings.Contains(s, "authentication required"), strings.Contains(s, "access denied"):
		return "authentication required or repository gone"
	case hasHTTPStatus(s, "404"), strings.Contains(s, "manifest unknown"), strings.Contains(s, "no such manifest"), strings.Contains(s, "name unknown"), strings.Contains(s, "not found"):
		return "repository or tag missing"
	case strings.Contains(s, "timeout"), strings.Contains(s, "unreachable"), strings.Contains(s, "connection refused"), strings.Contains(s, "no such host"), strings.Contains(s, "temporary failure"):
		return "network unavailable"
	default:
		return "unknown registry error: " + strings.TrimSpace(message)
	}
}

func hasHTTPStatus(message, code string) bool {
	return strings.HasSuffix(message, " "+code) ||
		strings.Contains(message, " "+code+" ") ||
		strings.Contains(message, " "+code+":") ||
		strings.Contains(message, "("+code+")")
}

// EnsureImage inspects the exact upstream digest, then pulls upstream and the
// same digest from its mirror in that order. It never tags a digest reference.
func EnsureImage(ctx context.Context, ref LockedImage) (string, error) {
	upstream := ref.Repository + "@" + ref.IndexDigest
	if _, err := runCapture(ctx, "image", "inspect", upstream); err == nil {
		return "cached", nil
	}
	if _, err := runCapture(ctx, "pull", upstream); err == nil {
		return "upstream", nil
	} else {
		diagnosis := Diagnose(err.Error())
		if ref.Mirror == nil || *ref.Mirror == "" {
			return "", fmt.Errorf("%s (%s): %s: %w", ref.Name, upstream, diagnosis, err)
		}
		mirror := *ref.Mirror + "@" + ref.IndexDigest
		if _, mirrorErr := runCapture(ctx, "pull", mirror); mirrorErr == nil {
			return "mirror", nil
		} else {
			return "", fmt.Errorf("%s (%s): %s; mirror %s: %s: %w", ref.Name, upstream, diagnosis, mirror, Diagnose(mirrorErr.Error()), mirrorErr)
		}
	}
}

type composeImageDoc struct {
	Services map[string]struct {
		Image string `json:"image"`
	} `json:"services"`
}

// EnsureComposeImages checks every locked image used by the resolved stack on
// every start, including after a lock change on a previously started project.
// It writes or removes the mirror override and reports whether the file changed.
func EnsureComposeImages(ctx context.Context, workdir string, c *Compose, refs []LockedImage) (bool, error) {
	args := append(c.buildBaseArgs(), "config", "--format", "json")
	cmd := exec.CommandContext(ctx, c.dockerBin(), args...)
	cmd.Dir = workdir
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("docker compose config: %w: %s", err, strings.TrimSpace(string(raw)))
	}
	var doc composeImageDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		return false, fmt.Errorf("parse docker compose config: %w", err)
	}
	services := make([]string, 0, len(doc.Services))
	for service := range doc.Services {
		services = append(services, service)
	}
	sort.Strings(services)
	overrides := make(map[string]map[string]string)
	for _, service := range services {
		image := doc.Services[service].Image
		for _, ref := range refs {
			mirror := ""
			if ref.Mirror != nil {
				mirror = *ref.Mirror + "@" + ref.IndexDigest
			}
			if image != ref.String() && image != ref.Repository+"@"+ref.IndexDigest && image != mirror {
				continue
			}
			source, err := EnsureImage(ctx, ref)
			if err != nil {
				return false, err
			}
			fmt.Fprintf(os.Stderr, "Image %s: %s (%s)\n", ref.Name, source, ref.IndexDigest)
			if source == "mirror" {
				overrides[service] = map[string]string{"image": mirror}
			}
			break
		}
	}
	return writeImageOverrides(workdir, overrides)
}

// writeImageOverrides updates the generated file atomically so every Compose
// call sees the same final image mapping. An empty mapping removes stale state.
func writeImageOverrides(workdir string, services map[string]map[string]string) (bool, error) {
	path := filepath.Join(workdir, ImageOverrideFile)
	if len(services) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return false, fmt.Errorf("remove image override: %w", err)
		} else if err == nil {
			return true, nil
		}
		return false, nil
	}
	body, err := yaml.Marshal(map[string]any{"services": services})
	if err != nil {
		return false, err
	}
	data := append([]byte("# GENERATED BY nself — DO NOT HAND EDIT\n"), body...)
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".image-overrides-*")
	if err != nil {
		return false, err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return false, err
	}
	if err := f.Close(); err != nil {
		return false, err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return false, err
	}
	return true, nil
}
