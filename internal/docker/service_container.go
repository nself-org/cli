package docker

// service_container.go — locate the running container of one compose service
// by its compose labels, and read its mounts.
//
// Purpose: TLS tooling must act on the nginx that actually serves a stack,
// which on a fronted layout (prod: /opt/nself-web fronting /opt/nself-web/
// backend) runs from a different compose project than the one the CLI was
// started in. Matching on the compose service label plus the project working
// directory (or project name) finds that container without guessing names.
// Inputs: a ServiceMatch (service name plus the working directories and/or
// project name that identify the stack).
// Outputs: the matching container's name; sentinel errors for zero or several
// matches (errors.Is); a container's mounts as the existing Mount type.
// Constraints: only running containers match (a stopped nginx serves nothing);
// every docker call goes through this package (the docker funnel); a docker
// failure is returned as-is and is never reported as "not found".

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// composeWorkingDirLabel is the label Docker Compose stamps on every container
// with the directory the project was started from.
const composeWorkingDirLabel = "com.docker.compose.project.working_dir"

// ErrServiceContainerNotFound is returned (wrapped) when no running container
// matches a ServiceMatch. Match with errors.Is.
var ErrServiceContainerNotFound = errors.New("service container not found")

// ErrServiceContainerAmbiguous is returned (wrapped, naming every candidate)
// when more than one running container matches a ServiceMatch. Match with
// errors.Is.
var ErrServiceContainerAmbiguous = errors.New("service container is ambiguous")

// ServiceMatch selects a compose service's container. A container matches
// when its com.docker.compose.service label equals Service and either its
// com.docker.compose.project.working_dir label is one of WorkingDirs or its
// com.docker.compose.project label equals Project (ignored when empty).
type ServiceMatch struct {
	Service     string
	WorkingDirs []string
	Project     string
}

// FindServiceContainer returns the name of the single running container that
// matches m. Zero matches return an error wrapping ErrServiceContainerNotFound;
// several return one wrapping ErrServiceContainerAmbiguous that names them.
// Docker being unreachable returns a different error, so callers can tell
// "not running" from "cannot tell".
func FindServiceContainer(ctx context.Context, m ServiceMatch) (string, error) {
	if m.Service == "" {
		return "", errors.New("find service container: empty service name")
	}
	ids, err := findContainersByLabel(ctx, composeServiceLabel+"="+m.Service)
	if err != nil {
		return "", fmt.Errorf("find service container %q: %w", m.Service, err)
	}

	dirs := make(map[string]bool, len(m.WorkingDirs))
	for _, d := range m.WorkingDirs {
		if d != "" {
			dirs[filepath.Clean(d)] = true
		}
	}

	var names []string
	for _, id := range ids {
		info, ierr := InspectContainer(ctx, id)
		if ierr != nil {
			if vanished(ctx, id, composeServiceLabel+"="+m.Service, ierr) {
				continue // removed between the listing and the inspect
			}
			return "", fmt.Errorf("find service container %q: %w", m.Service, ierr)
		}
		if info.State != "running" || info.Labels[composeServiceLabel] != m.Service {
			continue
		}
		wd := info.Labels[composeWorkingDirLabel]
		byDir := wd != "" && dirs[filepath.Clean(wd)]
		byProject := m.Project != "" && info.Labels[composeProjectLabel] == m.Project
		if byDir || byProject {
			names = append(names, info.Name)
		}
	}

	switch len(names) {
	case 0:
		return "", fmt.Errorf("%w: no running %q container in working dirs %v or project %q",
			ErrServiceContainerNotFound, m.Service, m.WorkingDirs, m.Project)
	case 1:
		return names[0], nil
	default:
		sort.Strings(names)
		return "", fmt.Errorf("%w: %q matches %s", ErrServiceContainerAmbiguous,
			m.Service, strings.Join(names, ", "))
	}
}

// vanished reports whether inspecting id failed because the container was
// removed after it was listed. InspectContainer reports any daemon stderr
// containing "not found" as `container "<id>" not found`, so that text alone
// is not proof (a daemon error such as "page not found" produces it too).
// The error must have exactly that shape for this id AND a fresh listing for
// label must no longer contain id; otherwise the error is real.
func vanished(ctx context.Context, id, label string, err error) bool {
	if err.Error() != fmt.Sprintf("container %q not found", id) {
		return false
	}
	ids, lerr := findContainersByLabel(ctx, label)
	if lerr != nil {
		return false
	}
	for _, cur := range ids {
		if cur == id {
			return false
		}
	}
	return true
}

// ContainerMounts returns the mounts of the named container via docker
// inspect, as Mount values (Source, Destination, Type, ReadOnly).
func ContainerMounts(ctx context.Context, name string) ([]Mount, error) {
	info, err := InspectContainer(ctx, name)
	if err != nil {
		return nil, err
	}
	return info.Mounts, nil
}
