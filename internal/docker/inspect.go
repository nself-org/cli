package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// dockerInspectRaw mirrors the subset of `docker inspect` JSON we care about.
type dockerInspectRaw struct {
	ID      string `json:"Id"`
	Name    string `json:"Name"`
	Created string `json:"Created"`
	Image   string `json:"Image"`
	State   struct {
		Status    string `json:"Status"`
		StartedAt string `json:"StartedAt"`
		Health    *struct {
			Status string `json:"Status"`
		} `json:"Health,omitempty"`
	} `json:"State"`
	Config struct {
		Image  string            `json:"Image"`
		Env    []string          `json:"Env"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	NetworkSettings struct {
		Ports map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
	Mounts []struct {
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		Type        string `json:"Type"`
		RW          bool   `json:"RW"`
	} `json:"Mounts"`
}

// InspectContainer shells out to `docker inspect` and returns parsed ContainerInfo.
func InspectContainer(ctx context.Context, name string) (*ContainerInfo, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{json .}}", name)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if strings.Contains(stderr, "No such object") || strings.Contains(stderr, "not found") {
				return nil, fmt.Errorf("container %q not found", name)
			}
			return nil, fmt.Errorf("docker inspect %q failed: %s", name, stderr)
		}
		return nil, fmt.Errorf("docker inspect %q: %w", name, err)
	}

	var raw dockerInspectRaw
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parsing docker inspect output for %q: %w", name, err)
	}

	info := &ContainerInfo{
		ID:        raw.ID,
		Name:      strings.TrimPrefix(raw.Name, "/"),
		Image:     raw.Image,
		ImageRef:  raw.Config.Image,
		State:     raw.State.Status,
		Health:    "none",
		Env:       parseEnvSlice(raw.Config.Env),
		Labels:    raw.Config.Labels,
		CreatedAt: raw.Created,
		StartedAt: raw.State.StartedAt,
		Ports:     formatPorts(raw.NetworkSettings.Ports),
		Mounts:    formatMounts(raw.Mounts),
	}

	if raw.State.Health != nil {
		info.Health = raw.State.Health.Status
	}

	return info, nil
}

// GetHealthStatus returns the health status of a container: "healthy", "unhealthy",
// "starting", "none" (no healthcheck configured), or "not_found".
func GetHealthStatus(ctx context.Context, name string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect",
		"--format", "{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}", name)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if strings.Contains(stderr, "No such object") || strings.Contains(stderr, "not found") {
				return "not_found", nil
			}
			if strings.Contains(stderr, "Cannot connect to the Docker daemon") {
				return "", fmt.Errorf("docker is not running — start Docker Desktop or run: systemctl start docker")
			}
			return "", fmt.Errorf("docker inspect health for %q: %s", name, stderr)
		}
		// Handle cases where docker is not installed or the daemon error appears outside stderr.
		if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") {
			return "", fmt.Errorf("docker is not running — start Docker Desktop or run: systemctl start docker")
		}
		return "", fmt.Errorf("docker inspect health for %q: %w", name, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// GetContainerLogs shells out to `docker logs --tail N` and returns the combined output.
func GetContainerLogs(ctx context.Context, name string, tail int) (string, error) {
	tailStr := "all"
	if tail > 0 {
		tailStr = strconv.Itoa(tail)
	}

	cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", tailStr, name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if strings.Contains(stderr, "No such container") || strings.Contains(stderr, "not found") {
				return "", fmt.Errorf("container %q not found", name)
			}
			// Some output may still be useful even on non-zero exit.
			if len(out) > 0 {
				return string(out), nil
			}
			return "", fmt.Errorf("docker logs %q failed: %s", name, stderr)
		}
		return "", fmt.Errorf("docker logs %q: %w", name, err)
	}
	return string(out), nil
}

// parseEnvSlice converts ["KEY=value", ...] into a map.
func parseEnvSlice(envs []string) map[string]string {
	m := make(map[string]string, len(envs))
	for _, e := range envs {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m
}

// formatPorts converts the Docker port map into a human-readable slice like "0.0.0.0:8080->80/tcp".
func formatPorts(ports map[string][]struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}) []string {
	var result []string
	for containerPort, bindings := range ports {
		if len(bindings) == 0 {
			result = append(result, containerPort)
			continue
		}
		for _, b := range bindings {
			if b.HostIP == "" {
				b.HostIP = "0.0.0.0"
			}
			result = append(result, fmt.Sprintf("%s:%s->%s", b.HostIP, b.HostPort, containerPort))
		}
	}
	return result
}

// formatMounts converts raw Docker mount data into the package Mount type.
func formatMounts(raw []struct {
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Type        string `json:"Type"`
	RW          bool   `json:"RW"`
}) []Mount {
	mounts := make([]Mount, len(raw))
	for i, m := range raw {
		mounts[i] = Mount{
			Source:      m.Source,
			Destination: m.Destination,
			Type:        m.Type,
			ReadOnly:    !m.RW,
		}
	}
	return mounts
}
