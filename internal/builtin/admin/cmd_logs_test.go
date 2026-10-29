package admin

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

func TestFollowLogsProducerFailureDiagnostic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	dir := t.TempDir()
	out, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	diag, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	defer diag.Close()
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = out, diag
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	cmd := &cobra.Command{Use: "logs"}
	err = runAdminLogsJSON(cmd, exec.Command("sh", "-c", "echo fixture-line; exit 7"), "fixture", true)
	var silent errs.Silencer
	if err == nil || !errors.As(err, &silent) || !silent.Silent() {
		t.Fatalf("failure not marked reported: %v", err)
	}
	raw, rerr := os.ReadFile(out.Name())
	if rerr != nil {
		t.Fatal(rerr)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want log then error envelope, got %s", raw)
	}
	var end struct {
		Error struct{ Code, Message string }
	}
	if err := json.Unmarshal([]byte(lines[1]), &end); err != nil {
		t.Fatal(err)
	}
	if end.Error.Code != "E400" || !strings.Contains(end.Error.Message, "exit status 7") {
		t.Fatalf("closing envelope: %s", lines[1])
	}
	stderr, err := os.ReadFile(diag.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stderr), "exit status 7") {
		t.Fatalf("missing stderr diagnostic: %q", stderr)
	}
}
