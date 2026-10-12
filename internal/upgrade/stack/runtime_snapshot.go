package stack

// Purpose: retain the fully resolved old Compose specification with immutable images.
// Inputs: running containers and their project Compose document. Outputs: private snapshot.
// Constraints: snapshot files never contain mutable image tags or printed secrets.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/docker"
)

// privateState refuses symlinked state ancestors before creating recovery artifacts.
func (r *Runtime) privateState() (string, error) {
	return stateDirectory(r.dir, true)
}

// snapshot pins every running service to its actual Docker image content ID.
func (r *Runtime) snapshot(ctx context.Context, j *Journal) error {
	c, err := r.compose()
	if err != nil {
		return err
	}
	path, data, err := r.captureSnapshot(ctx, c)
	if err != nil {
		return err
	}
	images, err := r.previousImages(ctx, c)
	if err != nil {
		return err
	}
	pinned, err := pinSnapshot(data, images)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, pinned, 0600); err != nil {
		return fmt.Errorf("write pinned previous compose: %w", err)
	}
	j.PreviousCompose = path
	sum := sha256.Sum256(pinned)
	j.PreviousComposeSHA256 = hex.EncodeToString(sum[:])
	return nil
}

// captureSnapshot retains resolved Compose configuration in a private artifact.
// Inputs: project compose. Outputs: path and bytes. Constraints: never print secrets.
func (r *Runtime) captureSnapshot(ctx context.Context, c *docker.Compose) (string, []byte, error) {
	dir, err := r.privateState()
	if err != nil {
		return "", nil, err
	}
	file, err := os.CreateTemp(dir, "update-stack-previous-*.json")
	if err != nil {
		return "", nil, fmt.Errorf("create recovery snapshot: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		return "", nil, fmt.Errorf("close recovery snapshot: %w", err)
	}
	args := []string{"compose"}
	for _, f := range c.ComposeFiles {
		args = append(args, "-f", f)
	}
	for _, f := range c.EnvFiles {
		args = append(args, "--env-file", f)
	}
	args = append(args, "config", "--format", "json", "--output", path)
	if err := c.Run(ctx, r.dir, args...); err != nil {
		return "", nil, fmt.Errorf("capture previous compose: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return "", nil, fmt.Errorf("secure previous compose: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("read previous compose: %w", err)
	}
	return path, data, nil
}

// previousImages reads immutable image identities including exited one-shot services.
// Inputs: declared project services. Outputs: pinned images. Constraints: rejects mixed replicas.
func (r *Runtime) previousImages(ctx context.Context, c *docker.Compose) (map[string]string, error) {
	specs, err := docker.ComposeServiceSpecs(ctx, c, r.dir)
	if err != nil {
		return nil, fmt.Errorf("inspect previous services: %w", err)
	}
	images := map[string]string{}
	for _, spec := range specs {
		states, err := c.ComposeServiceStates(ctx, r.dir, spec.Name)
		if err != nil {
			return nil, fmt.Errorf("inspect previous service %s: %w", spec.Name, err)
		}
		if len(states) == 0 {
			return nil, fmt.Errorf("previous service %s has no container evidence", spec.Name)
		}
		for _, container := range states {
			info, err := docker.InspectContainer(ctx, container.ID)
			if err != nil {
				return nil, fmt.Errorf("inspect previous image: %w", err)
			}
			service := info.Labels["com.docker.compose.service"]
			if service == "" {
				return nil, fmt.Errorf("previous container lacks service identity")
			}
			if !strings.HasPrefix(info.Image, "sha256:") {
				return nil, fmt.Errorf("previous image is not immutable")
			}
			if prior := images[service]; prior != "" && prior != info.Image {
				return nil, fmt.Errorf("previous service %s replicas have different images", service)
			}
			images[service] = info.Image
		}
	}
	return images, nil
}

// pinSnapshot replaces mutable references and refuses incomplete recovery evidence.
func pinSnapshot(data []byte, images map[string]string) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse previous compose: %w", err)
	}
	services, ok := doc["services"].(map[string]any)
	if !ok || len(services) == 0 {
		return nil, fmt.Errorf("previous compose has no services")
	}
	for name, value := range services {
		service, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid previous service %q", name)
		}
		image := images[name]
		if image == "" {
			return nil, fmt.Errorf("previous service %q has no immutable running image", name)
		}
		service["image"] = image
		service["pull_policy"] = "never"
		delete(service, "build")
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode previous compose: %w", err)
	}
	return out, nil
}

// verifySnapshot binds retained private bytes and immutable images to this journal.
func (r *Runtime) verifySnapshot(j *Journal) error {
	dir, err := r.privateState()
	if err != nil {
		return err
	}
	if filepath.Dir(j.PreviousCompose) != dir || !strings.HasPrefix(filepath.Base(j.PreviousCompose), "update-stack-previous-") {
		return fmt.Errorf("invalid recovery snapshot path")
	}
	info, err := os.Lstat(j.PreviousCompose)
	if err != nil {
		return fmt.Errorf("stat recovery snapshot: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("recovery snapshot must be a private regular file")
	}
	data, err := os.ReadFile(j.PreviousCompose)
	if err != nil {
		return fmt.Errorf("read recovery snapshot: %w", err)
	}
	sum := sha256.Sum256(data)
	if j.PreviousComposeSHA256 == "" || hex.EncodeToString(sum[:]) != j.PreviousComposeSHA256 {
		return fmt.Errorf("recovery snapshot digest changed")
	}
	var doc struct {
		Services map[string]struct {
			Image      string `json:"image"`
			Build      any    `json:"build"`
			PullPolicy string `json:"pull_policy"`
		} `json:"services"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("decode recovery snapshot: %w", err)
	}
	if len(doc.Services) == 0 {
		return fmt.Errorf("recovery snapshot has no services")
	}
	for name, service := range doc.Services {
		if !strings.HasPrefix(service.Image, "sha256:") || service.Build != nil || service.PullPolicy != "never" {
			return fmt.Errorf("recovery service %s is not pinned", name)
		}
	}
	return nil
}
