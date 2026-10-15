//go:build darwin || linux

package docker

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestStreamLogsSignalHelper(t *testing.T) {
	if os.Getenv("NSELF_STREAM_SIGNAL_HELPER") != "1" {
		return
	}
	err := (&Compose{DockerPath: os.Getenv("NSELF_STREAM_TEST_DOCKER")}).StreamLogs(context.Background(), "", []string{"compose", "logs", "-f"}, os.Stdout, os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
}

func TestStreamLogsSignalsCancelProcessGroup(t *testing.T) {
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "orphan")
			bin := streamTestDocker(t, "(sleep 1; touch \"$NSELF_STREAM_TEST_MARKER\") &\necho ready\nwait\n")
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStreamLogsSignalHelper$")
			cmd.Env = append(os.Environ(), "NSELF_STREAM_SIGNAL_HELPER=1", "NSELF_STREAM_TEST_DOCKER="+bin, "NSELF_STREAM_TEST_MARKER="+marker)
			out, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			scanner := bufio.NewScanner(out)
			if !scanner.Scan() || scanner.Text() != "ready" {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatal("log child did not become ready")
			}
			if err := cmd.Process.Signal(sig); err != nil {
				t.Error(err)
			}
			if err := cmd.Wait(); err != nil {
				t.Fatal(fmt.Errorf("signal failed to end stream cleanly: %w", err))
			}
			time.Sleep(1500 * time.Millisecond)
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("compose child survived %s: marker stat = %v", sig, err)
			}
		})
	}
}
