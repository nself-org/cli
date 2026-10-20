//go:build darwin || linux

package docker

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFollowLogsStreamsAndCancels(t *testing.T) {
	for _, blocking := range []bool{false, true} {
		t.Run(map[bool]string{false: "exit", true: "cancel"}[blocking], func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "docker")
			script := "#!/bin/sh\nprintf 'out'\nprintf 'err' >&2\n"
			if blocking {
				script += "sleep 60 &\nwait\n"
			}
			if err := os.WriteFile(bin, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			c := NewCompose()
			c.DockerPath = bin
			var out, stderr bytes.Buffer
			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			started := time.Now()
			err := c.FollowLogs(ctx, dir, &out, &stderr)
			if blocking && err == nil {
				t.Fatal("canceled follow succeeded")
			}
			if !blocking && err != nil {
				t.Fatal(err)
			}
			if time.Since(started) > 3*time.Second {
				t.Fatal("child survived cancellation")
			}
			if !strings.Contains(out.String(), "out") || !strings.Contains(stderr.String(), "err") {
				t.Fatalf("stdout=%q stderr=%q", out.String(), stderr.String())
			}
		})
	}
}
