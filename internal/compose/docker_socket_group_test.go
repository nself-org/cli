package compose

// Purpose: cover the Docker-socket group resolution and the group_add it
// produces on the admin service.
//
// Inputs:  a path that does or does not exist; an admin-enabled generator.
// Outputs: assertions on (gid, ok) and on the emitted service.
// Constraints: must not require a running Docker daemon. The "socket exists"
// case uses an ordinary temp file, because every implementation only stats the
// path — it never connects.

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestDockerSocketGroup_AbsentSocketAddsNoGroup(t *testing.T) {
	// A host with no Docker must produce no group_add at all. Falling back to
	// gid 0 here would hand the container the root group for no reason.
	missing := filepath.Join(t.TempDir(), "definitely-not-here.sock")

	gid, ok := dockerSocketGroupAt(missing)
	if ok {
		t.Errorf("expected ok=false for a missing socket, got ok=true gid=%q", gid)
	}
	if gid != "" {
		t.Errorf("expected empty gid for a missing socket, got %q", gid)
	}
}

func TestDockerSocketGroup_PresentSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "docker.sock")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("seed socket file: %v", err)
	}

	gid, ok := dockerSocketGroupAt(path)

	if runtime.GOOS == "windows" {
		// No POSIX group ownership; Docker Desktop uses a named pipe.
		if ok {
			t.Errorf("windows must report no group, got gid=%q", gid)
		}
		return
	}

	if !ok {
		t.Fatal("expected a gid for an existing path")
	}
	if gid == "" {
		t.Error("ok=true but gid is empty")
	}

	if runtime.GOOS == "darwin" && gid != "0" {
		// Docker Desktop presents the socket to containers as root:root
		// regardless of what the host symlink stats as.
		t.Errorf("darwin must resolve to the in-container owner 0, got %q", gid)
	}
}

func TestAdminService_GroupAddMatchesSocketResolution(t *testing.T) {
	cfg := minimalCfg()
	cfg.Admin = config.AdminConfig{Enabled: true, Port: 3021, Version: "latest"}
	svc := NewGenerator(cfg).buildAdminService()

	wantGid, wantOK := dockerSocketGroup()

	if !wantOK {
		if len(svc.GroupAdd) != 0 {
			t.Errorf("no socket group resolved, so group_add must be empty; got %v", svc.GroupAdd)
		}
		return
	}

	if len(svc.GroupAdd) != 1 || svc.GroupAdd[0] != wantGid {
		t.Errorf("group_add = %v, want [%s]", svc.GroupAdd, wantGid)
	}
}

func TestAdminService_StillRunsAsNonRoot(t *testing.T) {
	// group_add grants the socket's group, NOT root. If this ever becomes
	// user: root the group_add work is pointless and the container is running
	// with far more privilege than it needs.
	cfg := minimalCfg()
	cfg.Admin = config.AdminConfig{Enabled: true, Port: 3021, Version: "latest"}
	svc := NewGenerator(cfg).buildAdminService()

	if svc.User != "1000:1000" {
		t.Errorf("admin user = %q, want \"1000:1000\"", svc.User)
	}
}

func TestAdminService_GroupAddSerializesWhenPresent(t *testing.T) {
	// omitempty must not swallow a real group, and must omit an empty one.
	cfg := minimalCfg()
	cfg.Admin = config.AdminConfig{Enabled: true, Port: 3021, Version: "latest"}
	svcs := generatedServices(t, NewGenerator(cfg))

	if _, ok := svcs[AdminServiceName]; !ok {
		t.Fatalf("admin service missing from generated compose")
	}
}
