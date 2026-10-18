package docker

// run_like_test.go — RunLike argv order, failure classification and cleanup
// (fake docker that logs every call to $FAKE_DIR/calls).

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// likeDocker logs each call; `start -a` exits $START_RC, and inspect of the
// throwaway reports $STATUS.
const likeDocker = `#!/bin/sh
echo "$@" >> "$FAKE_DIR/calls"
case "$1" in
  inspect) case "$3" in '{{.Config.Image}}') echo nginx:test ;; *) echo "${STATUS:-exited}" ;; esac ;;
  create) echo cid9 ;;
  start) echo "nginx: bad conf" >&2; exit ${START_RC:-0} ;;
esac
`

func installLikeDocker(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(likeDocker), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DIR", dir)
	return dir
}

func TestRunLike_ArgvOrderAndCleanup(t *testing.T) {
	dir := installLikeDocker(t)
	_, _, err := RunLike(context.Background(), LikeSpec{Container: "own", Networks: []string{"n1", "n2"}, Args: []string{"nginx", "-t"}})
	if err != nil {
		t.Fatalf("RunLike: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "calls"))
	want := []string{
		"inspect --format {{.Config.Image}} -- own",
		"create --volumes-from own --network n1 nginx:test nginx -t",
		"network connect -- n2 cid9",
		"start -a -- cid9",
		"rm -f -- cid9",
	}
	if got := strings.Split(strings.TrimSpace(string(b)), "\n"); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestRunLike_CommandFailureVersusSetupFailure(t *testing.T) {
	dir := installLikeDocker(t)
	t.Setenv("START_RC", "1")
	_, stderr, err := RunLike(context.Background(), LikeSpec{Container: "own", Args: []string{"nginx", "-t"}})
	var exit *RunLikeExit
	if !errors.As(err, &exit) || !strings.Contains(stderr, "bad conf") || exit.Last != "nginx: bad conf" {
		t.Fatalf("a command that ran and failed is a *RunLikeExit: %v", err)
	}

	t.Setenv("STATUS", "created")
	_, _, err = RunLike(context.Background(), LikeSpec{Container: "own", Args: []string{"nginx", "-t"}})
	if err == nil || errors.As(err, &exit) {
		t.Fatalf("a container that never started is a setup failure: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "calls"))
	if strings.Count(string(b), "rm -f -- cid9") != 2 {
		t.Errorf("the throwaway must be removed on every path:\n%s", b)
	}
}
