package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
	"github.com/spf13/cobra"
)

func TestUpdateImages_DigestIdentity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stubs docker with a /bin/sh script, which Windows cannot exec")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	script := "#!/bin/sh\nprintf '%s\\n' '{\"Descriptor\":{\"digest\":\"sha256:" + digest + "\"}}'\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	old := compose.ImageDigests
	t.Cleanup(func() { compose.ImageDigests = old })
	cmd := &cobra.Command{}
	cmd.Flags().Bool("dry-run", false, "")
	if err := runUpdateImages(cmd, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, compose.DigestConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]string
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"auth", "auth-v14", "redis", "valkey"} {
		entry, ok := compose.LockedRef(name)
		if !ok {
			t.Fatalf("missing lock entry %s", name)
		}
		if saved[entry.LegacyRef] != digest {
			t.Errorf("saved[%q] = %q, want %q", entry.LegacyRef, saved[entry.LegacyRef], digest)
		}
		if _, exists := saved[name]; exists {
			t.Errorf("ambiguous entry key %q saved", name)
		}
	}
}
