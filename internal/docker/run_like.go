package docker

// run_like.go — run one command in a throwaway container shaped like an
// existing one.
//
// Purpose: test a configuration for a container that cannot run it yet. The
// shared host proxy (P7-ADOPT-21) must know that a cold owner's `nginx -t`
// passes before it starts that nginx: `docker exec` needs a running container,
// and a start with a broken conf crash-loops it.
// Inputs: the existing container (any state), the networks the throwaway joins,
// and the command.
// Outputs: stdout and stderr of the command. A command that ran and exited
// non-zero returns a *RunLikeExit; any other error means the throwaway could
// not be set up or started.
// Constraints: the throwaway reuses the image and every mount of the source
// (`--volumes-from`), publishes no port and copies no environment. It is always
// removed, also after a cancelled context. Names follow "--".

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// LikeSpec describes one RunLike call.
type LikeSpec struct {
	// Container is the existing container whose image and mounts are reused.
	Container string
	// Networks are the networks the throwaway joins; empty leaves the default
	// bridge. Docker allows one at create time, the rest are connected after.
	Networks []string
	// Args are the command and arguments (they replace the image's CMD).
	Args []string
}

// RunLikeExit is returned when the command ran in the throwaway and exited
// non-zero. The captured output comes back next to it.
type RunLikeExit struct {
	Last string // last stderr line
}

func (e *RunLikeExit) Error() string { return "the command exited non-zero: " + e.Last }

// RunLike creates a container from the image of spec.Container with its
// mounts, joins spec.Networks, runs spec.Args to completion and removes it.
func RunLike(ctx context.Context, spec LikeSpec) (stdout, stderr string, err error) {
	img, err := runCapture(ctx, "inspect", "--format", "{{.Config.Image}}", "--", spec.Container)
	if err != nil {
		return "", "", err
	}
	image := strings.TrimSpace(string(img))
	args := []string{"create", "--volumes-from", spec.Container}
	if len(spec.Networks) > 0 {
		args = append(args, "--network", spec.Networks[0])
	}
	out, err := runCapture(ctx, append(append(args, image), spec.Args...)...)
	if err != nil {
		return "", "", err
	}
	id := strings.TrimSpace(string(out))
	defer func() { _, _ = runCapture(context.WithoutCancel(ctx), "rm", "-f", "--", id) }()
	if len(spec.Networks) > 1 {
		for _, n := range spec.Networks[1:] {
			if _, err := runCapture(ctx, "network", "connect", "--", n, id); err != nil {
				return "", "", err
			}
		}
	}
	c := exec.CommandContext(ctx, "docker", "start", "-a", "--", id)
	var so, se bytes.Buffer
	c.Stdout, c.Stderr = &so, &se
	if runErr := c.Run(); runErr != nil {
		var exit *exec.ExitError
		if !errors.As(runErr, &exit) {
			return so.String(), se.String(), fmt.Errorf("docker start: %w", runErr)
		}
		// Docker also exits non-zero when the container never started.
		if st, serr := runCapture(ctx, "inspect", "--format", "{{.State.Status}}", "--", id); serr != nil || strings.TrimSpace(string(st)) != "exited" {
			return so.String(), se.String(), fmt.Errorf("docker start: %w: %s", runErr, lastLine(se.String()))
		}
		return so.String(), se.String(), &RunLikeExit{Last: lastLine(se.String())}
	}
	return so.String(), se.String(), nil
}
