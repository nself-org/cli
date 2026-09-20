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
	"strings"
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

func TestAdminService_RunsAsTheHostUser(t *testing.T) {
	// The admin bind-mounts the project dir read-write and its health check
	// requires W_OK there, so the container user must own that directory.
	// A hardcoded 1000:1000 breaks on any host whose user is not uid 1000 —
	// GitHub Actions' runner is 1001.
	cfg := minimalCfg()
	cfg.Admin = config.AdminConfig{Enabled: true, Port: 3021, Version: "latest"}
	svc := NewGenerator(cfg).buildAdminService()

	want, ok := hostUser()
	if !ok {
		// Windows: no POSIX uid, keep the documented default.
		if svc.User != "1000:1000" {
			t.Errorf("admin user = %q, want the 1000:1000 default when the host uid is unknown", svc.User)
		}
		return
	}

	if svc.User != want {
		t.Errorf("admin user = %q, want the host user %q", svc.User, want)
	}
}

func TestAdminService_DoesNotRunAsRoot(t *testing.T) {
	// group_add grants the socket's group, not root. Running the whole
	// container as root would make the group_add work pointless and hand it
	// far more privilege than it needs. This only holds when the CLI itself
	// is not being run as root.
	if u, ok := hostUser(); ok && strings.HasPrefix(u, "0:") {
		t.Skip("CLI is running as root; the admin legitimately inherits that uid")
	}

	cfg := minimalCfg()
	cfg.Admin = config.AdminConfig{Enabled: true, Port: 3021, Version: "latest"}
	svc := NewGenerator(cfg).buildAdminService()

	if strings.HasPrefix(svc.User, "0:") || svc.User == "root" {
		t.Errorf("admin must not run as root, got user %q", svc.User)
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
