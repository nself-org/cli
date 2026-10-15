package commands

// start_lock_admin_test.go — every CLI-owned service of a real render has a lock
// key, the admin UI included (P7-LIVE-09, round 9).
//
// Purpose: the admin UI runs as the compose service nself-admin but is pinned as
// "admin". Before round 9 nothing mapped one to the other, so the lock had no
// admin key and --frozen never looked at it. These tests build the lock from the
// real generator output (no hand remap) and compare it with the compose.
// Inputs: a stack rendered with every optional service on, in lock mode.
// Outputs: failures naming the service or the key.
// Constraints: hermetic (clean image variables, temp project dir and plugin dir).

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/lock"
	"github.com/nself-org/cli/internal/version"
)

// adminProject renders the full stack with the admin UI on, writes its compose
// and the lock computed from it, and returns the project dir and the rendered
// service -> image map.
func adminProject(t *testing.T) (string, map[string]string) {
	t.Helper()
	blankImageVars(t)
	t.Setenv("IMAGE_PINNING", "lock")
	dir := t.TempDir()
	t.Setenv("NSELF_PLUGIN_DIR", filepath.Join(dir, "plugins"))
	oldVersion := version.Version
	version.Version = "1.4.0"
	t.Cleanup(func() { version.Version = oldVersion })
	stack := renderStack(t, map[string]string{
		"REDIS_ENABLED": "true", "MINIO_ENABLED": "true", "MAILPIT_ENABLED": "true", "NSELF_ADMIN_ENABLED": "true",
		"FUNCTIONS_ENABLED": "true", "FUNCTIONS_RUNTIME": "node", "SEARCH_ENABLED": "true", "SEARCH_ENGINE": "meilisearch",
	})
	if _, ok := stack[compose.AdminServiceName]; !ok {
		t.Fatalf("the render has no %s service: %v", compose.AdminServiceName, stack)
	}
	writeStack(t, dir, stack)
	l, err := build.CurrentLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := l.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock.Path(dir), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, stack
}

func writeStack(t *testing.T, dir string, stack map[string]string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("services:\n")
	for name, img := range stack {
		b.WriteString("  " + name + ":\n    image: " + img + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestLockPinsEveryCLIOwnedServiceOfARender: each CLI-owned service of a real
// render is in the lock under its pin key; the admin UI is pinned as "admin".
func TestLockPinsEveryCLIOwnedServiceOfARender(t *testing.T) {
	compattest.Set(t, false) // core MinIO exists only in v1.4 (P7-PLUG-60)
	dir, stack := adminProject(t)
	l, err := build.CurrentLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	owned := 0
	for name := range stack {
		if !compose.IsCLIOwned(name) {
			continue
		}
		owned++
		key := compose.PinKey(name)
		_, pinned := l.Images[key]
		_, unpinned := l.Unpinned[key]
		if !pinned && !unpinned {
			t.Errorf("CLI-owned service %s has no lock key %q: images %v, unpinned %v", name, key, l.Images, l.Unpinned)
		}
	}
	if owned < 8 {
		t.Fatalf("only %d CLI-owned services in the render: the fixture lost services", owned)
	}
	if _, ok := l.Images["admin"]; !ok {
		t.Errorf("the admin UI must be pinned under \"admin\": %v", l.Images)
	}
	if _, ok := l.Images[compose.AdminServiceName]; ok {
		t.Errorf("the lock must not be keyed by the compose service name %s", compose.AdminServiceName)
	}
}

// TestStartFrozenAdminImageEdited: --frozen passes on the rendered project and
// fails with E455 naming the admin image once its compose image is edited.
func TestStartFrozenAdminImageEdited(t *testing.T) {
	compattest.Set(t, false) // core MinIO exists only in v1.4 (P7-PLUG-60)
	dir, stack := adminProject(t)
	var out bytes.Buffer
	if err := checkStartLock(dir, true, &out); err != nil {
		t.Fatalf("a project matching its lock must pass --frozen: %v", err)
	}
	stack[compose.AdminServiceName] = "example.com/edited/admin:0.0.1"
	writeStack(t, dir, stack)
	err := checkStartLock(dir, true, &out)
	var ce *errs.CLIError
	if err == nil || !errors.As(err, &ce) || ce.Code != "E455" || !strings.Contains(err.Error(), "image admin") {
		t.Fatalf("an edited admin image must fail --frozen with E455 naming it, got %v", err)
	}
}

// TestStartFrozenAdminMirrorNamesFile proves that mirror service names are
// translated to project-lock pin keys before mismatches are annotated.
func TestStartFrozenAdminMirrorNamesFile(t *testing.T) {
	compattest.Set(t, false) // core MinIO exists only in v1.4 (P7-PLUG-60)
	dir, _ := adminProject(t)
	const file = ".nself/state/image-overrides.yml"
	path := filepath.Join(dir, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	stale := "sha256:" + strings.Repeat("5", 64)
	body := "# GENERATED BY nself\nservices:\n  " + compose.AdminServiceName + ":\n    image: mirror.example/nself-admin@" + stale + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := checkStartLock(dir, true, &out)
	var ce *errs.CLIError
	if err == nil || !errors.As(err, &ce) || ce.Code != "E455" || !strings.Contains(ce.What, "image admin") {
		t.Fatalf("a stale admin mirror must fail with E455 naming the admin pin, got %v", err)
	}
	if !strings.Contains(ce.What, file) || !strings.Contains(ce.Fix, file) {
		t.Fatalf("the admin mirror mismatch must name %s:\nwhat: %s\nfix: %s", file, ce.What, ce.Fix)
	}
}
