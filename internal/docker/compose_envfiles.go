package docker

// compose_envfiles.go: compose calls that keep the project's --env-file list.
//
// Purpose: give `nself dev` and the blue/green pipeline the compose calls they
// need (up --build, a named compose project with extra environment, captured
// output) without dropping the -f/--env-file flags every Compose call carries,
// so ${VAR} values that live only in .nself/compose.env resolve (D-0550).
// Inputs: a Compose (files, env files, docker path), a workdir, a ProjectRun.
// Outputs: the docker compose exit status, or its stdout for OutputProject.
// Constraints: argv always starts with buildBaseArgs; nothing here writes to
// os.Stdout itself (the stdout ratchet): callers pass the writers.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// composeEnvFile is the interpolation file `nself build` writes, relative to
// the project root (internal/build composeEnvFile).
const composeEnvFile = ".nself/compose.env"

// ProjectEnvFiles returns the --env-file list for the project in workdir by
// the rule of build.ComposeEnvFiles: the project .env (when present) then
// .nself/compose.env, or nil when compose.env is absent (compose then reads
// .env on its own). It serves packages that may not import internal/build;
// cmd/commands dev_test.go pins the two functions to each other.
func ProjectEnvFiles(workdir string) []string {
	computed := filepath.Join(workdir, composeEnvFile)
	if _, err := os.Stat(computed); err != nil {
		return nil
	}
	dotenv := filepath.Join(workdir, ".env")
	if _, err := os.Stat(dotenv); err == nil {
		return []string{dotenv, computed}
	}
	return []string{computed}
}

// ComposeUpBuild runs `docker compose up -d --no-deps --build [services...]`:
// rebuild the named services' images and recreate only those containers.
func (c *Compose) ComposeUpBuild(ctx context.Context, workdir string, services ...string) error {
	args := c.buildBaseArgs()
	args = withServices(append(args, "up", "-d", "--no-deps", "--build"), services...)
	return c.Run(ctx, workdir, args...)
}

// ProjectRun scopes one compose call to a named compose project with extra
// process environment (blue/green runs a second copy of the stack under its
// own -p name).
type ProjectRun struct {
	Project string    // -p value; empty keeps compose's default project name
	Env     []string  // KEY=VALUE pairs appended to the inherited environment
	Stdout  io.Writer // nil discards
	Stderr  io.Writer // nil discards; the stderr tail still reaches the error
}

// projectCmd builds `docker compose <-f/--env-file flags> [-p project] args...`.
func (c *Compose) projectCmd(ctx context.Context, workdir string, p ProjectRun, args []string) *exec.Cmd {
	full := c.buildBaseArgs()
	if p.Project != "" {
		full = append(full, "-p", p.Project)
	}
	cmd := exec.CommandContext(ctx, c.dockerBin(), append(full, args...)...)
	cmd.Dir = workdir
	cmd.Env = append(os.Environ(), p.Env...)
	return cmd
}

// RunProject runs args against p.Project in workdir. A failure carries the
// tail of compose's stderr.
func (c *Compose) RunProject(ctx context.Context, workdir string, p ProjectRun, args ...string) error {
	cmd := c.projectCmd(ctx, workdir, p, args)
	errTail := &tailBuffer{limit: stderrTailLimit}
	cmd.Stdout = orDiscard(p.Stdout)
	cmd.Stderr = io.MultiWriter(orDiscard(p.Stderr), errTail)
	if err := cmd.Run(); err != nil {
		if detail := errTail.String(); detail != "" {
			return fmt.Errorf("docker compose %s: %w: %s", strings.Join(args, " "), err, detail)
		}
		return fmt.Errorf("docker compose %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// OutputProject runs args against p.Project in workdir and returns stdout
// (p.Stdout and p.Stderr are not used). A failure carries the last stderr line.
func (c *Compose) OutputProject(ctx context.Context, workdir string, p ProjectRun, args ...string) ([]byte, error) {
	cmd := c.projectCmd(ctx, workdir, p, args)
	var se bytes.Buffer
	cmd.Stderr = &se
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker compose %s: %w: %s", strings.Join(args, " "), err, lastLine(se.String()))
	}
	return out, nil
}

func orDiscard(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}
