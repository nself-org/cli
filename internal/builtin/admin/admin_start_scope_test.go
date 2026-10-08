package admin

import (
	"os"
	"strings"
	"testing"
)

// TestAdminStart_DoesNotShellOutToNselfStart guards the one-service fallback.
func TestAdminStart_DoesNotShellOutToNselfStart(t *testing.T) {
	src, err := os.ReadFile("cmd_start.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	for _, bad := range []string{`"nself", "start"`, `"nself","start"`} {
		if strings.Contains(body, bad) {
			t.Errorf("admin start shells out to %s", bad)
		}
	}
	if !strings.Contains(body, "ComposeUpNoDeps") {
		t.Error("admin start no longer starts only the admin service")
	}
}
