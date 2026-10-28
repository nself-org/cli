package docker

// Purpose: report whether the Docker daemon remaps container uids (D-0771).
// Inputs: the daemon `docker info` reaches through the usual DOCKER_HOST rules.
// Outputs: the SecurityOptions entries, and whether they name a rootless or
// user-namespace-remapped daemon.
// Constraints: one read-only `docker info`, ten second timeout.

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

// RemapsUIDs reports whether the daemon is rootless or runs with a
// user-namespace remap: a container uid then maps to a sub-uid that does not
// own the host's files. A daemon that cannot be asked counts as not remapping,
// the behaviour before it was consulted.
func RemapsUIDs() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{json .SecurityOptions}}").Output()
	var opts []string
	if err != nil || json.Unmarshal(out, &opts) != nil {
		return false
	}
	return remappedOptions(opts)
}

func remappedOptions(opts []string) bool {
	for _, opt := range opts {
		for _, field := range strings.Split(opt, ",") {
			if field == "name=rootless" || field == "name=userns" {
				return true
			}
		}
	}
	return false
}
