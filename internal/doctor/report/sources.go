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
//         child has a 120 s timeout; each container log read has its own 60 s
//         deadline and an expiry becomes a manifest note; all output is
//         scrubbed by Collect.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/version"
)

// DockerSetup tells the real log source which stack to read.
type DockerSetup struct {
	ProjectDir   string
	DockerPath   string // empty: docker from PATH
	ComposeFiles []string
	EnvFiles     []string
}

// ContainerLogTimeout bounds one container's log read (N3).
const ContainerLogTimeout = 60 * time.Second

// RealSources returns the production seams.
func RealSources(ds DockerSetup) Sources {
	var mu sync.Mutex
	var logNotes []string
	note := func(n string) {
		mu.Lock()
		defer mu.Unlock()
		logNotes = append(logNotes, n)
	}
	return Sources{
		LogNotes: func() []string {
			mu.Lock()
			defer mu.Unlock()
			return append([]string(nil), logNotes...)
		},
		Versions: func(ctx context.Context) map[string]string { return realVersions(ctx) },
		Doctor:   func(ctx context.Context) (string, error) { return RunDoctorChild(ctx, ds.ProjectDir, os.Executable) },
		Logs:     func(ctx context.Context, tail int) (map[string]string, error) { return dockerLogs(ctx, ds, tail, note) },
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

// dockerLogs reads the log tail of every container of every compose service.
func dockerLogs(ctx context.Context, ds DockerSetup, tail int, note func(string)) (map[string]string, error) {
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
			text := readContainerLog(ctx, ContainerLogTimeout, st.Name, note, func(c context.Context) (string, error) {
				return docker.GetContainerLogs(c, st.Name, tail)
			})
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

// readContainerLog runs fetch under its own deadline. A failure becomes the
// log text; an expiry also becomes a manifest note (via note) naming the
// container, so a hung daemon cannot hang the report (N3).
func readContainerLog(ctx context.Context, timeout time.Duration, container string, note func(string), fetch func(context.Context) (string, error)) string {
	lctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	text, err := fetch(lctx)
	// docker logs killed by the deadline may return partial output and no error.
	switch {
	case errors.Is(lctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil:
		note(fmt.Sprintf("log read for container %s timed out after %s: its log is incomplete", container, timeout))
		return text + "[log read timed out after " + timeout.String() + "]\n"
	case err == nil:
		return text
	}
	return "log read failed: " + err.Error() + "\n"
}
