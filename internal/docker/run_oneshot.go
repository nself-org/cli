package docker

// run_oneshot.go — run a throwaway container to completion and capture its output.
//
// Purpose: tools that run once and exit (the ACME client that issues a
// certificate) need `docker run --rm` through the docker funnel, with
// secrets handed over safely.
// Inputs: a RunSpec (image, arguments, bind mounts, user, network, and the
// environment to pass through).
// Outputs: the container's stdout and stderr; an error carrying the
// stderr tail when docker or the container exits non-zero.
// Constraints: a secret value is never placed in argv. EnvPass names are
// forwarded with `-e NAME` and their values live only in the docker client's
// own environment. The Docker socket is never mounted: a bind mount whose
// source is the socket is refused.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// cancelCleanupTimeout bounds the `docker rm -f` that runs after a cancelled
// run; the run's own context is already done, so the cleanup gets a fresh one.
const cancelCleanupTimeout = 30 * time.Second

// RunSpec describes one `docker run --rm` invocation.
type RunSpec struct {
	// Image is the full image reference (a digest-pinned one for production use).
	Image string
	// Args are the arguments handed to the image's entrypoint.
	Args []string
	// EnvPass maps an environment variable name to its value. Only the name
	// reaches argv (`-e NAME`); the value is set in the docker client's env.
	EnvPass map[string]string
	// Mounts are bind mounts; Source is a host path, Destination the in-container path.
	Mounts []Mount
	// User is "uid:gid"; empty leaves the image default.
	User string
	// Network is a docker network name; empty leaves the default bridge.
	Network string
	// PullNever refuses to pull a missing image (`--pull never`): the image must already exist locally.
	PullNever bool
	// ReadOnly mounts the root filesystem read-only (`--read-only`).
	ReadOnly bool
	// CapDrop lists capabilities to drop (`--cap-drop X`); "ALL" drops every one.
	CapDrop []string
	// SecurityOpt lists `--security-opt` values, e.g. "no-new-privileges:true".
	SecurityOpt []string
	// Name is the container name (`--name`). RunOneShot generates one when it is
	// empty, so a cancelled run can always force-remove its container.
	Name string
	// Labels are `--label key=value` pairs, rendered in key order.
	Labels map[string]string
}

// dockerSocketPath is the daemon socket no one-shot container may mount.
const dockerSocketPath = "docker.sock"

// buildRunArgs renders spec as `docker run` arguments. Env names are sorted so
// the argv is deterministic. It refuses a mount of the Docker socket.
func buildRunArgs(spec RunSpec) ([]string, error) {
	args := []string{"run", "--rm"}
	if spec.Name != "" {
		args = append(args, "--name", spec.Name)
	}
	labels := make([]string, 0, len(spec.Labels))
	for k := range spec.Labels {
		labels = append(labels, k)
	}
	sort.Strings(labels)
	for _, k := range labels {
		args = append(args, "--label", k+"="+spec.Labels[k])
	}
	if spec.User != "" {
		args = append(args, "--user", spec.User)
	}
	if spec.Network != "" {
		args = append(args, "--network", spec.Network)
	}
	if spec.PullNever {
		args = append(args, "--pull", "never")
	}
	if spec.ReadOnly {
		args = append(args, "--read-only")
	}
	for _, c := range spec.CapDrop {
		args = append(args, "--cap-drop", c)
	}
	for _, o := range spec.SecurityOpt {
		args = append(args, "--security-opt", o)
	}
	for _, m := range spec.Mounts {
		if strings.HasSuffix(m.Source, dockerSocketPath) {
			return nil, fmt.Errorf("docker run: refusing to mount the Docker socket %s", m.Source)
		}
		v := m.Source + ":" + m.Destination
		if m.ReadOnly {
			v += ":ro"
		}
		args = append(args, "-v", v)
	}
	names := make([]string, 0, len(spec.EnvPass))
	for n := range spec.EnvPass {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		args = append(args, "-e", n)
	}
	args = append(args, spec.Image)
	return append(args, spec.Args...), nil
}

// RunOneShot runs spec to completion (`docker run --rm`) and returns what the
// container wrote to stdout and stderr. A non-zero exit returns the captured
// output together with an error that quotes the last stderr line. The container
// always has a name; when ctx is cancelled the docker client is killed and the
// container is force-removed, so it does not keep running after the caller gave
// up (killing only the client leaves it running).
func RunOneShot(ctx context.Context, spec RunSpec) (stdout, stderr string, err error) {
	if spec.Name == "" {
		spec.Name = oneShotName()
	}
	args, err := buildRunArgs(spec)
	if err != nil {
		return "", "", err
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	// Own process group: a terminal Ctrl-C reaches nself only, which then removes
	// the container, instead of racing the client's own signal proxy.
	setProcGroupAttr(cmd)
	cmd.Cancel = func() error {
		killProcessGroup(cmd)
		return cmd.Process.Kill()
	}
	cmd.WaitDelay = 5 * time.Second
	cmd.Env = os.Environ()
	for n, v := range spec.EnvPass {
		cmd.Env = append(cmd.Env, n+"="+v)
	}
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	if runErr := cmd.Run(); runErr != nil {
		if ctx.Err() != nil {
			rmCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cancelCleanupTimeout)
			defer cancel()
			if rmErr := RemoveContainer(rmCtx, spec.Name); rmErr != nil {
				runErr = fmt.Errorf("%w (removing container %s: %v)", runErr, spec.Name, rmErr)
			}
		}
		return so.String(), se.String(), fmt.Errorf("docker run %s: %w: %s", spec.Image, runErr, lastLine(se.String()))
	}
	return so.String(), se.String(), nil
}

// oneShotName returns a unique container name for a one-shot run.
func oneShotName() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("nself-oneshot-%d", time.Now().UnixNano())
	}
	return "nself-oneshot-" + hex.EncodeToString(b[:])
}

// lastLine returns the last non-empty line of s, for error messages.
func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
