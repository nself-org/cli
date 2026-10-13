package imageswitch

// runtime_docker.go — the Runtime over the docker funnel (internal/docker).
//
// Purpose: production wiring of the Runtime seam. Tests use a fake.
// Constraints: no exec.Command("docker") here; every call goes through internal/docker.

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/docker"
)

// dataMount is where the postgres image keeps its data.
const dataMount = "/var/lib/postgresql/data"

// DockerRuntime is the Runtime backed by the docker CLI.
type DockerRuntime struct{}

// Inspect reads the postgres container.
func (DockerRuntime) Inspect(ctx context.Context, name string) (ContainerInfo, error) {
	info, err := docker.InspectContainer(ctx, name)
	if err != nil {
		return ContainerInfo{}, err
	}
	ci := ContainerInfo{
		Image: info.ImageRef, Running: strings.EqualFold(info.State, "running"),
		ComposeProject: info.Labels["com.docker.compose.project"],
	}
	if ci.DataVolume, err = docker.ContainerVolume(ctx, name, dataMount); err != nil {
		return ContainerInfo{}, err
	}
	if ci.Networks, err = docker.ContainerNetworks(ctx, name); err != nil {
		return ContainerInfo{}, err
	}
	return ci, nil
}

// ExecSQL runs psql in the container over the local socket (trust inside the container).
func (DockerRuntime) ExecSQL(ctx context.Context, container, user, db, sql string) (string, error) {
	out, stderr, err := docker.ExecCapture(ctx, container, []string{"psql", "-X", "-q", "-At", "-v", "ON_ERROR_STOP=1", "-U", user, "-d", db, "-c", sql})
	if err != nil {
		return "", fmt.Errorf("%s", redactTail(stderr+"\n"+err.Error(), 3))
	}
	return out, nil
}

// RunOneShot runs a throwaway container.
func (DockerRuntime) RunOneShot(ctx context.Context, spec docker.RunSpec) (string, string, error) {
	return docker.RunOneShot(ctx, spec)
}

// RemoveContainer force-removes a container.
func (DockerRuntime) RemoveContainer(ctx context.Context, name string) error {
	return docker.RemoveContainer(ctx, name)
}

// VolumeCreate creates a labelled volume.
func (DockerRuntime) VolumeCreate(ctx context.Context, name string, labels map[string]string) error {
	return docker.VolumeCreate(ctx, name, labels)
}

// VolumeExists reports whether a volume exists.
func (DockerRuntime) VolumeExists(ctx context.Context, name string) (bool, error) {
	return docker.VolumeExists(ctx, name)
}

// VolumeRemove removes a volume.
func (DockerRuntime) VolumeRemove(ctx context.Context, name string) error {
	return docker.VolumeRemove(ctx, name)
}
