package commands

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// stubPluginLogsDocker puts a fake `docker` first on PATH. inspect succeeds so the
// container resolves; `logs` runs logsBody.
func stubPluginLogsDocker(t *testing.T, logsBody string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in\n  inspect) echo /nself-plugin-demo; exit 0;;\n  logs) " + logsBody + ";;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func runPluginLogsWith(t *testing.T, grep string) error {
	t.Helper()
	pluginLogsCmd.Flags().Set("grep", grep)
	t.Cleanup(func() { pluginLogsCmd.Flags().Set("grep", "") })
	pluginLogsCmd.SetContext(context.Background())
	return runPluginLogs(pluginLogsCmd, []string{"demo"})
}

func TestPluginLogsGrepSurfacesDockerFailure(t *testing.T) {
	stubPluginLogsDocker(t, "echo 'no such container' >&2; exit 1")
	err := runPluginLogsWith(t, "ERROR")
	if err == nil || !strings.Contains(err.Error(), "docker logs") {
		t.Fatalf("want docker logs error, got %v", err)
	}
}

func TestPluginLogsGrepNoMatchIsNotAnError(t *testing.T) {
	stubPluginLogsDocker(t, "echo hello; exit 0")
	if err := runPluginLogsWith(t, "ERROR"); err != nil {
		t.Fatalf("no match must not error, got %v", err)
	}
}
