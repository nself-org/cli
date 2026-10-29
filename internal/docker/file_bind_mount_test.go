//go:build !windows

package docker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestFileBindMounted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("token"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, script  string
		mounted, fail bool
	}{
		{"mounted", fmt.Sprintf("if [ \"$1\" = ps ]; then echo cid; else printf '%%s' '[{\"Mounts\":[{\"Source\":\"%s\"}]}]'; fi\n", path), true, false},
		{"no containers", "exit 0\n", false, false},
		{"unavailable", "exit 1\n", false, true},
		{"bad inspection", "echo cid\n", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeDockerOnPath(t, tc.script)
			got, err := FileBindMounted(context.Background(), path)
			if got != tc.mounted || (err != nil) != tc.fail {
				t.Fatalf("got %v, %v", got, err)
			}
		})
	}
}
