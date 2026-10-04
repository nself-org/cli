package docker

// Tests for FindServiceContainer and ContainerMounts. internal/docker has no
// exec seam, so these put a fake `docker` shell script first on PATH. The
// script serves `docker ps` from ps.txt and `docker inspect <id>` from
// inspect-<id>.json, and logs every call to calls.log.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

const fakeDockerScript = `#!/bin/sh
echo "$@" >> "$FAKE_DOCKER_DIR/calls.log"
case "$1" in
  ps)
    [ -f "$FAKE_DOCKER_DIR/ps.fail" ] && { echo "Cannot connect to the Docker daemon" >&2; exit 1; }
    cat "$FAKE_DOCKER_DIR/ps.txt" ;;
  inspect)
    for a; do id=$a; done
    f="$FAKE_DOCKER_DIR/inspect-$id.json"
    if [ -f "$f" ]; then cat "$f"; else echo "Error: No such object: $id" >&2; exit 1; fi ;;
  *) exit 2 ;;
esac
`

type fakeCtr struct {
	id, name, state string
	labels          map[string]string
	mounts          []map[string]any
}

// installFakeDocker writes the script and fixtures and puts the script on PATH.
func installFakeDocker(t *testing.T, ctrs ...fakeCtr) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(fakeDockerScript), 0o755); err != nil {
		t.Fatal(err)
	}
	var ps []string
	for _, c := range ctrs {
		ps = append(ps, c.id)
		raw := map[string]any{
			"Id":     c.id,
			"Name":   "/" + c.name,
			"State":  map[string]any{"Status": c.state},
			"Config": map[string]any{"Labels": c.labels},
			"Mounts": c.mounts,
		}
		b, err := json.Marshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		// Reachable by id (from ps) and by name (ContainerMounts callers).
		for _, key := range []string{c.id, c.name} {
			if err := os.WriteFile(filepath.Join(dir, "inspect-"+key+".json"), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "ps.txt"), []byte(strings.Join(ps, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_DOCKER_DIR", dir)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func nginxLabels(project, workdir string) map[string]string {
	return map[string]string{
		composeServiceLabel:    "nginx",
		composeProjectLabel:    project,
		composeWorkingDirLabel: workdir,
	}
}

// TestFindServiceContainer covers working-dir match, project match, zero and
// several matches, stopped containers, and docker failure.
func TestFindServiceContainer(t *testing.T) {
	t.Run("matches service plus working dir", func(t *testing.T) {
		dir := installFakeDocker(t,
			fakeCtr{id: "a1", name: "web-nginx", state: "running", labels: nginxLabels("web", "/opt/nself-web")},
			fakeCtr{id: "b2", name: "other-nginx", state: "running", labels: nginxLabels("other", "/srv/other")},
		)
		got, err := FindServiceContainer(t.Context(), ServiceMatch{
			Service: "nginx", WorkingDirs: []string{"/opt/nself-web/", "/opt/nself-web/backend"},
		})
		if err != nil || got != "web-nginx" {
			t.Fatalf("got %q, %v; want web-nginx", got, err)
		}
		log, _ := os.ReadFile(filepath.Join(dir, "calls.log"))
		if !strings.Contains(string(log), "label=com.docker.compose.service=nginx") {
			t.Errorf("docker ps must filter on the service label; calls:\n%s", log)
		}
	})

	t.Run("matches service plus project label", func(t *testing.T) {
		installFakeDocker(t,
			fakeCtr{id: "a1", name: "web-nginx", state: "running", labels: nginxLabels("web", "/elsewhere")},
		)
		got, err := FindServiceContainer(t.Context(), ServiceMatch{Service: "nginx", Project: "web"})
		if err != nil || got != "web-nginx" {
			t.Fatalf("got %q, %v", got, err)
		}
	})

	t.Run("zero matches is ErrServiceContainerNotFound", func(t *testing.T) {
		installFakeDocker(t,
			fakeCtr{id: "a1", name: "web-nginx", state: "running", labels: nginxLabels("web", "/opt/nself-web")},
			fakeCtr{id: "c3", name: "web-hasura", state: "running", labels: map[string]string{
				composeServiceLabel: "hasura", composeProjectLabel: "web", composeWorkingDirLabel: "/opt/x"}},
		)
		_, err := FindServiceContainer(t.Context(), ServiceMatch{
			Service: "nginx", WorkingDirs: []string{"/nowhere"}, Project: "nope"})
		if !errors.Is(err, ErrServiceContainerNotFound) {
			t.Fatalf("err = %v, want ErrServiceContainerNotFound", err)
		}
		if errors.Is(err, ErrServiceContainerAmbiguous) {
			t.Error("must not also be ambiguous")
		}
	})

	t.Run("empty project never matches an unlabelled container", func(t *testing.T) {
		installFakeDocker(t, fakeCtr{id: "a1", name: "x", state: "running",
			labels: map[string]string{composeServiceLabel: "nginx"}})
		_, err := FindServiceContainer(t.Context(), ServiceMatch{Service: "nginx"})
		if !errors.Is(err, ErrServiceContainerNotFound) {
			t.Fatalf("err = %v, want not found", err)
		}
	})

	t.Run("two matches is ErrServiceContainerAmbiguous naming both", func(t *testing.T) {
		installFakeDocker(t,
			fakeCtr{id: "a1", name: "web-nginx-1", state: "running", labels: nginxLabels("web", "/opt/nself-web")},
			fakeCtr{id: "b2", name: "web-nginx-2", state: "running", labels: nginxLabels("web2", "/opt/nself-web")},
		)
		_, err := FindServiceContainer(t.Context(), ServiceMatch{
			Service: "nginx", WorkingDirs: []string{"/opt/nself-web"}})
		if !errors.Is(err, ErrServiceContainerAmbiguous) {
			t.Fatalf("err = %v, want ErrServiceContainerAmbiguous", err)
		}
		for _, n := range []string{"web-nginx-1", "web-nginx-2"} {
			if !strings.Contains(err.Error(), n) {
				t.Errorf("error %q does not name %s", err, n)
			}
		}
	})

	t.Run("stopped containers do not match", func(t *testing.T) {
		installFakeDocker(t,
			fakeCtr{id: "a1", name: "old-nginx", state: "exited", labels: nginxLabels("web", "/opt/nself-web")},
			fakeCtr{id: "b2", name: "web-nginx", state: "running", labels: nginxLabels("web", "/opt/nself-web")},
		)
		got, err := FindServiceContainer(t.Context(), ServiceMatch{Service: "nginx", Project: "web"})
		if err != nil || got != "web-nginx" {
			t.Fatalf("got %q, %v; the exited container must be ignored", got, err)
		}
	})

	t.Run("docker failure is neither sentinel", func(t *testing.T) {
		dir := installFakeDocker(t)
		if err := os.WriteFile(filepath.Join(dir, "ps.fail"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := FindServiceContainer(t.Context(), ServiceMatch{Service: "nginx", Project: "web"})
		if err == nil || errors.Is(err, ErrServiceContainerNotFound) || errors.Is(err, ErrServiceContainerAmbiguous) {
			t.Fatalf("err = %v, want a plain docker error", err)
		}
	})

	t.Run("empty service is rejected", func(t *testing.T) {
		if _, err := FindServiceContainer(t.Context(), ServiceMatch{}); err == nil {
			t.Fatal("expected an error")
		}
	})
}

// TestContainerMounts checks mounts come back as docker.Mount values.
func TestContainerMounts(t *testing.T) {
	installFakeDocker(t, fakeCtr{
		id: "a1", name: "web-nginx", state: "running", labels: nginxLabels("web", "/opt/nself-web"),
		mounts: []map[string]any{
			{"Type": "bind", "Source": "/opt/nself-web/ssl", "Destination": "/etc/nginx/ssl", "RW": false},
			{"Type": "volume", "Source": "/var/lib/docker/volumes/logs/_data", "Destination": "/var/log/nginx", "RW": true},
		},
	})
	got, err := ContainerMounts(t.Context(), "web-nginx")
	if err != nil {
		t.Fatal(err)
	}
	want := []Mount{
		{Source: "/opt/nself-web/ssl", Destination: "/etc/nginx/ssl", Type: "bind", ReadOnly: true},
		{Source: "/var/lib/docker/volumes/logs/_data", Destination: "/var/log/nginx", Type: "volume", ReadOnly: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mounts = %+v, want %+v", got, want)
	}
	if _, err := ContainerMounts(t.Context(), "missing"); err == nil {
		t.Error("expected an error for an unknown container")
	}
}
