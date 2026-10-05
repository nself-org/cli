package docker

// exec_stdin_test.go — ExecStdin argv, stdin delivery, output, error and hook (fake docker).

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// installStdinDocker puts a fake docker first on PATH that records argv and
// stdin, prints out-text/err-text and exits with $FAKE_RC.
func installStdinDocker(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"$@\" > \"$FAKE_DIR/argv\"\ncat > \"$FAKE_DIR/stdin\" </dev/stdin\necho out-text\necho err-text >&2\nexit ${FAKE_RC:-0}\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DIR", dir)
	return dir
}

func TestExecStdin_PipesStdinWithDashI(t *testing.T) {
	dir := installStdinDocker(t)
	stdout, stderr, err := ExecStdin(context.Background(), "p_postgres", []string{"psql", "-U", "u"}, strings.NewReader("SELECT 1;"))
	if err != nil {
		t.Fatalf("ExecStdin: %v", err)
	}
	argv, _ := os.ReadFile(filepath.Join(dir, "argv"))
	if got := strings.TrimSpace(string(argv)); got != "exec -i p_postgres psql -U u" {
		t.Errorf("argv = %q", got)
	}
	in, _ := os.ReadFile(filepath.Join(dir, "stdin"))
	if string(in) != "SELECT 1;" {
		t.Errorf("stdin = %q", in)
	}
	if !strings.Contains(stdout, "out-text") || !strings.Contains(stderr, "err-text") {
		t.Errorf("output not captured: %q / %q", stdout, stderr)
	}
}

func TestExecStdin_NilStdinHasNoDashI(t *testing.T) {
	dir := installStdinDocker(t)
	if _, _, err := ExecStdin(context.Background(), "c", []string{"psql", "-tAc", "SELECT 1"}, nil); err != nil {
		t.Fatal(err)
	}
	argv, _ := os.ReadFile(filepath.Join(dir, "argv"))
	if got := strings.TrimSpace(string(argv)); got != "exec c psql -tAc SELECT 1" {
		t.Errorf("argv = %q", got)
	}
}

func TestExecStdin_NonZeroExitReturnsErrorAndStderr(t *testing.T) {
	installStdinDocker(t)
	t.Setenv("FAKE_RC", "3")
	_, stderr, err := ExecStdin(context.Background(), "c", []string{"psql"}, strings.NewReader("x"))
	if err == nil || !strings.Contains(err.Error(), "exit status 3") || !strings.Contains(stderr, "err-text") {
		t.Fatalf("want exit error and stderr, got %v / %q", err, stderr)
	}
}

func TestExecStdin_HookCountsEveryExecAndRestores(t *testing.T) {
	installStdinDocker(t)
	var seen []string
	restore := SetExecHook(func(container string, cmd []string) { seen = append(seen, container+":"+cmd[0]) })
	for i := 0; i < 3; i++ {
		if _, _, err := ExecStdin(context.Background(), "c", []string{"psql"}, nil); err != nil {
			t.Fatal(err)
		}
	}
	restore()
	if _, _, err := ExecStdin(context.Background(), "c", []string{"psql"}, nil); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 3 || seen[0] != "c:psql" {
		t.Errorf("hook saw %v, want 3 calls and none after restore", seen)
	}
}
