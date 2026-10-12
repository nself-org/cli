package docker

// compose_envfiles_test.go — ProjectEnvFiles and the D-0550 compose calls keep
// the -f/--env-file flags, the -p project and the extra environment.

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestProjectEnvFiles(t *testing.T) {
	for _, tc := range []struct {
		name          string
		dotenv, built bool
		want          []string // relative to the project dir
	}{
		{"neither", false, false, nil},
		{"dotenv only", true, false, nil},
		{"compose.env only", false, true, []string{".nself/compose.env"}},
		{"both", true, true, []string{".env", ".nself/compose.env"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.dotenv {
				writeFile(t, filepath.Join(dir, ".env"), "A=1\n")
			}
			if tc.built {
				writeFile(t, filepath.Join(dir, ".nself", "compose.env"), "B=2\n")
			}
			var want []string
			for _, w := range tc.want {
				want = append(want, filepath.Join(dir, filepath.FromSlash(w)))
			}
			if got := ProjectEnvFiles(dir); !reflect.DeepEqual(got, want) {
				t.Errorf("ProjectEnvFiles = %q, want %q", got, want)
			}
		})
	}
}

func TestComposeUpBuildArgs(t *testing.T) {
	bin, argv := fakeDockerArgs(t)
	c := NewCompose("/m/base.yml")
	c.DockerPath, c.EnvFiles = bin, []string{"/p/.env", "/p/.nself/compose.env"}
	if err := c.ComposeUpBuild(context.Background(), t.TempDir(), "plug"); err != nil {
		t.Fatal(err)
	}
	want := []string{"compose", "-f", "/m/base.yml", "--env-file", "/p/.env", "--env-file", "/p/.nself/compose.env",
		"up", "-d", "--no-deps", "--build", "--", "plug"}
	if got := argv(); !reflect.DeepEqual(got, want) {
		t.Errorf("argv = %q, want %q", got, want)
	}
}

// fakeProjectDocker writes a stub that records its argv, then NSELF_FAKE_PROBE
// from its environment, prints "out" on stdout and "boom" on stderr, and exits
// with code.
func fakeProjectDocker(t *testing.T, code string) (string, func() []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a shell script")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	bin := filepath.Join(dir, "docker")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" \"probe=${NSELF_FAKE_PROBE-}\" > '" + log + "'\necho out\necho boom >&2\nexit " + code + "\n"
	writeExec(t, bin, script)
	return bin, func() []string {
		b, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	}
}

func TestRunProjectArgsAndEnv(t *testing.T) {
	bin, argv := fakeProjectDocker(t, "0")
	c := NewCompose()
	c.DockerPath, c.EnvFiles = bin, []string{"/p/.nself/compose.env"}
	var out strings.Builder
	run := ProjectRun{Project: "nself-green", Env: []string{"NSELF_FAKE_PROBE=green"}, Stdout: &out}
	if err := c.RunProject(context.Background(), t.TempDir(), run, "up", "-d"); err != nil {
		t.Fatal(err)
	}
	want := []string{"compose", "--env-file", "/p/.nself/compose.env", "-p", "nself-green", "up", "-d", "probe=green"}
	if got := argv(); !reflect.DeepEqual(got, want) {
		t.Errorf("argv = %q, want %q", got, want)
	}
	if out.String() != "out\n" {
		t.Errorf("stdout = %q, want the stub's output", out.String())
	}
}

func TestRunProjectNoProjectAndFailure(t *testing.T) {
	bin, argv := fakeProjectDocker(t, "3")
	c := NewCompose()
	c.DockerPath, c.EnvFiles = bin, []string{"/p/.nself/compose.env"}
	err := c.RunProject(context.Background(), t.TempDir(), ProjectRun{}, "pull")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want the stderr tail", err)
	}
	want := []string{"compose", "--env-file", "/p/.nself/compose.env", "pull", "probe="}
	if got := argv(); !reflect.DeepEqual(got, want) {
		t.Errorf("argv = %q, want %q (no -p without a project)", got, want)
	}
}

func TestOutputProject(t *testing.T) {
	bin, argv := fakeProjectDocker(t, "0")
	c := NewCompose()
	c.DockerPath, c.EnvFiles = bin, []string{"/p/.nself/compose.env"}
	out, err := c.OutputProject(context.Background(), t.TempDir(), ProjectRun{Project: "nself-blue"}, "ps")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "out\n" {
		t.Errorf("out = %q", out)
	}
	if got := strings.Join(argv(), " "); !strings.HasPrefix(got, "compose --env-file /p/.nself/compose.env -p nself-blue ps") {
		t.Errorf("argv = %q", got)
	}

	failing, _ := fakeProjectDocker(t, "1")
	c.DockerPath = failing
	if _, err := c.OutputProject(context.Background(), t.TempDir(), ProjectRun{}, "ps"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want the last stderr line", err)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeExec(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatal(err)
	}
}
