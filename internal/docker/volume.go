package docker

// volume.go — named volumes, container removal and container networks.
//
// Purpose: the postgres image switch (P7-ADOPT-22) creates a new data volume,
// removes only that volume when it aborts, clears the one-shot containers a
// killed run left behind, and needs the network the postgres container sits
// on. All docker calls stay in this package (the funnel).
// Inputs: volume and container names, labels.
// Outputs: errors that quote the last stderr line; "absent" is reported as a
// bool or as success, never as a failure.
// Constraints: VolumeRemove refuses a volume that a container still uses (docker
// does); names are passed after "--" or as a final argument, so a name that
// starts with "-" is never read as a flag.

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// VolumeCreate creates the named volume with the given labels. Creating a
// volume that exists with the same driver is a no-op in docker.
func VolumeCreate(ctx context.Context, name string, labels map[string]string) error {
	args := []string{"volume", "create"}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--label", k+"="+labels[k])
	}
	_, err := runCapture(ctx, append(args, name)...)
	return err
}

// VolumeExists reports whether the named volume exists.
func VolumeExists(ctx context.Context, name string) (bool, error) {
	_, err := runCapture(ctx, "volume", "inspect", "--format", "{{.Name}}", name)
	if err == nil {
		return true, nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "no such volume") {
		return false, nil
	}
	return false, err
}

// VolumeRemove removes the named volume. An absent volume is success.
func VolumeRemove(ctx context.Context, name string) error {
	_, err := runCapture(ctx, "volume", "rm", name)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "no such volume") {
		return nil
	}
	return err
}

// RemoveContainer force-removes the named container. An absent container is success.
func RemoveContainer(ctx context.Context, name string) error {
	_, err := runCapture(ctx, "rm", "-f", name)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "no such container") {
		return nil
	}
	return err
}

// ContainerNetworks returns the names of the networks the container is on,
// sorted.
func ContainerNetworks(ctx context.Context, name string) ([]string, error) {
	out, err := runCapture(ctx, "inspect", "--format", "{{json .NetworkSettings.Networks}}", name)
	if err != nil {
		return nil, err
	}
	var nets map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &nets); err != nil {
		return nil, fmt.Errorf("container networks: unreadable inspect output: %w", err)
	}
	names := make([]string, 0, len(nets))
	for n := range nets {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

// ContainerVolume returns the name of the named volume mounted at dest in the
// container, "" when nothing named is mounted there.
func ContainerVolume(ctx context.Context, container, dest string) (string, error) {
	tmpl := `{{range .Mounts}}{{if and (eq .Destination "` + dest + `") (eq .Type "volume")}}{{.Name}}{{end}}{{end}}`
	out, err := runCapture(ctx, "inspect", "--format", tmpl, container)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// VolumeLabels returns the labels of the named volume; exists is false when the volume is absent.
func VolumeLabels(ctx context.Context, name string) (labels map[string]string, exists bool, err error) {
	out, err := runCapture(ctx, "volume", "inspect", "--format", "{{json .Labels}}", name)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such volume") {
			return nil, false, nil
		}
		return nil, false, err
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(out))), &labels); err != nil {
		return nil, true, fmt.Errorf("volume labels: unreadable inspect output: %w", err)
	}
	return labels, true, nil
}

// VolumeListByLabels returns the names of the volumes that carry every given
// label filter ("key" or "key=value"; docker ANDs repeated filters). An empty
// result is not an error. Docker daemon unavailability is returned as an error
// so callers can distinguish "no matches" from "cannot list". With no filters
// it returns nothing, never every volume on the host.
func VolumeListByLabels(ctx context.Context, labels ...string) ([]string, error) {
	if len(labels) == 0 {
		return nil, nil
	}
	args := []string{"volume", "ls"}
	for _, l := range labels {
		args = append(args, "--filter", "label="+l)
	}
	out, err := runCapture(ctx, append(args, "--format", "{{.Name}}")...)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			names = append(names, line)
		}
	}
	return names, nil
}
