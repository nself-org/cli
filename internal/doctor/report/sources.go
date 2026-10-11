package report

// sources.go: the real Sources used by `nself doctor report`.
//
// Purpose: wire versions, the doctor child run and per-service logs to the
// real world, through the docker funnel (internal/docker) and os/exec of this
// binary.
// Inputs: project dir, compose file/env file lists, the path of this binary.
// Outputs: Sources whose functions never print and never panic.
// Constraints: docker is reached only via internal/docker (ComposeServiceNames,
//         ComposeServiceStates, GetContainerLogs, ServerVersion); the doctor
//         child has a 120 s timeout; its output is scrubbed by Collect.

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/version"
)

// DoctorTimeout bounds the doctor child process.
const DoctorTimeout = 120 * time.Second

// DockerSetup tells the real log source which stack to read.
type DockerSetup struct {
	ProjectDir   string
	DockerPath   string // empty: docker from PATH
	ComposeFiles []string
	EnvFiles     []string
}

// RealSources returns the production seams.
func RealSources(ds DockerSetup) Sources {
	return Sources{
		Versions: func(ctx context.Context) map[string]string { return realVersions(ctx) },
		Doctor:   func(ctx context.Context) (string, error) { return RunDoctorChild(ctx, ds.ProjectDir, os.Executable) },
		Logs:     func(ctx context.Context, tail int) (map[string]string, error) { return dockerLogs(ctx, ds, tail) },
		Now:      time.Now,
	}
}

func realVersions(ctx context.Context) map[string]string {
	v := map[string]string{
		"nself":     version.GetVersion(),
		"commit":    version.GetCommit(),
		"buildDate": version.GetBuildDate(),
		"goVersion": runtime.Version(),
		"platform":  runtime.GOOS + "/" + runtime.GOARCH,
	}
	vctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if dv, err := docker.ServerVersion(vctx); err == nil && dv != "" {
		v["docker"] = dv
	}
	return v
}

// RunDoctorChild runs `<this binary> doctor` in dir and returns its combined
// output. A non-zero exit (failed checks) is not an error here. exe is the
// seam for os.Executable.
func RunDoctorChild(ctx context.Context, dir string, exe func() (string, error)) (string, error) {
	self, err := exe()
	if err != nil {
		return "", fmt.Errorf("locating this binary: %w", err)
	}
	cctx, cancel := context.WithTimeout(ctx, DoctorTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, self, "doctor") // #nosec G204 -- own binary, fixed argument
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	if cctx.Err() != nil {
		return out.String(), fmt.Errorf("doctor timed out after %s", DoctorTimeout)
	}
	if _, isExit := err.(*exec.ExitError); isExit {
		return out.String(), nil
	}
	return out.String(), err
}

// dockerLogs reads the log tail of every container of every compose service.
func dockerLogs(ctx context.Context, ds DockerSetup, tail int) (map[string]string, error) {
	c := docker.NewCompose(ds.ComposeFiles...)
	c.EnvFiles, c.DockerPath = ds.EnvFiles, ds.DockerPath
	services, err := c.ComposeServiceNames(ctx, ds.ProjectDir)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, svc := range services {
		states, serr := c.ComposeServiceStates(ctx, ds.ProjectDir, svc)
		if serr != nil {
			continue
		}
		var parts []string
		for _, st := range states {
			if st.Name == "" {
				continue
			}
			text, lerr := docker.GetContainerLogs(ctx, st.Name, tail)
			if lerr != nil {
				text = "log read failed: " + lerr.Error() + "\n"
			}
			if len(states) > 1 {
				text = "==> " + st.Name + " <==\n" + text
			}
			parts = append(parts, text)
		}
		if len(parts) > 0 {
			out[svc] = strings.Join(parts, "\n")
		}
	}
	return out, nil
}
