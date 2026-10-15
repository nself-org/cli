package docker

import (
	"context"
	"io"
)

// Executor abstracts Docker Compose and container operations for testability.
type Executor interface {
	ComposeUp(ctx context.Context, services ...string) error
	ComposeDown(ctx context.Context, opts DownOptions) error
	ComposePs(ctx context.Context) ([]Container, error)
	ComposeLogs(ctx context.Context, service string, follow bool, tail int) (io.ReadCloser, error)
	Inspect(ctx context.Context, container string) (*ContainerInfo, error)
	Exec(ctx context.Context, container string, cmd []string) error
}

// DownOptions configures behavior for ComposeDown.
type DownOptions struct {
	RemoveVolumes bool
	RemoveImages  bool // --rmi all
	RemoveOrphans bool
	Timeout       int // seconds; 0 uses Docker default
}

// Container represents a running or stopped container from ComposePs.
type Container struct {
	ID      string
	Name    string
	Service string
	State   string // running, exited, paused, etc.
	Health  string // healthy, unhealthy, starting, none
	Ports   []string
}

// ContainerInfo holds detailed inspection data for a single container.
type ContainerInfo struct {
	ID        string
	Name      string
	Service   string // Docker Compose service name (e.g. "postgres", "hasura")
	Image     string
	ImageRef  string // the reference the container was created from (Config.Image), e.g. postgres:16-alpine
	State     string
	Health    string
	Ports     []string
	Env       map[string]string
	Labels    map[string]string
	CreatedAt string
	StartedAt string
	Mounts    []Mount
}

// Mount represents a container volume mount.
type Mount struct {
	Source      string
	Destination string
	Type        string // bind, volume, tmpfs
	ReadOnly    bool
}
