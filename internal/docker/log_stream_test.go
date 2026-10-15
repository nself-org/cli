package docker

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func streamTestDocker(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a shell script")
	}
	bin := filepath.Join(t.TempDir(), "docker")
	writeExec(t, bin, "#!/bin/sh\n"+script)
	return bin
}

func TestStreamLogsForwardsPipedOutput(t *testing.T) {
	bin := streamTestDocker(t, "printf 'stdout log\\n'\nprintf 'stderr log\\n' >&2\n")
	c := &Compose{DockerPath: bin}
	var stdout, stderr bytes.Buffer
	if err := c.StreamLogs(context.Background(), t.TempDir(), []string{"logs", "-f", "app"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "stdout log\n" || stderr.String() != "stderr log\n" {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestStreamLogsFailure(t *testing.T) {
	bin := streamTestDocker(t, "echo failed >&2\nexit 7\n")
	err := (&Compose{DockerPath: bin}).StreamLogs(context.Background(), "", []string{"logs", "app"}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "exit status 7") {
		t.Fatalf("got %v, want child failure", err)
	}
}

func TestStreamLogLinesBothStreamsAndConsumerError(t *testing.T) {
	bin := streamTestDocker(t, "echo out\necho err >&2\n")
	c := &Compose{DockerPath: bin}
	var lines []string
	err := c.StreamLogLines(context.Background(), "", []string{"logs", "app"}, nil, func(line string) error {
		lines = append(lines, line)
		return nil
	})
	if err != nil || len(lines) != 2 {
		t.Fatalf("lines=%q error=%v", lines, err)
	}
	stop := errors.New("stop")
	err = c.StreamLogLines(context.Background(), "", []string{"logs", "app"}, nil, func(string) error { return stop })
	if !errors.Is(err, stop) {
		t.Fatalf("got %v, want consumer error", err)
	}
}
