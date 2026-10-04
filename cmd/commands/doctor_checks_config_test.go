package commands

// Tests for the doctor port check (D-0063): a port held by the project's own
// running stack is not a conflict, an unrelated holder still is, and a failed
// ownership query falls back to today's unfiltered result plus one warning.
// The probes are replaced with a fake port owner; no socket or docker is used.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/ports"
)

// fakePortWorld models a host: held is every port with a listener, owned is
// the subset held by the project's own compose stack, ownerErr makes the
// ownership query fail.
type fakePortWorld struct {
	held     map[int]bool
	owned    map[int]bool
	ownerErr error

	filteredCalls int
	allCalls      int
	gotWorkdir    string
	gotEnvFiles   []string
	gotPorts      []int
}

// install swaps the doctor seams for the fake and restores them on cleanup.
func (f *fakePortWorld) install(t *testing.T, projectDir string) {
	t.Helper()
	origF, origA, origH, origP := doctorPortsFiltered, doctorPortsAll, doctorPortHolder, doctorPortProject
	t.Cleanup(func() {
		doctorPortsFiltered, doctorPortsAll, doctorPortHolder, doctorPortProject = origF, origA, origH, origP
	})
	doctorPortProject = func() (string, error) { return projectDir, nil }
	doctorPortsAll = func(list []int) ([]docker.PortConflict, error) {
		f.allCalls++
		return f.conflicts(list, false), nil
	}
	doctorPortsFiltered = func(_ context.Context, list []int, workdir string, envFiles []string, _ ...string) ([]docker.PortConflict, error) {
		f.filteredCalls++
		f.gotWorkdir, f.gotEnvFiles, f.gotPorts = workdir, envFiles, list
		if f.ownerErr != nil {
			return nil, f.ownerErr
		}
		return f.conflicts(list, true), nil
	}
	doctorPortHolder = func(port int) (*ports.Holder, error) {
		if !f.held[port] {
			return nil, nil
		}
		return &ports.Holder{PID: 4242, Name: "apache2"}, nil
	}
}

// conflicts returns the held ports in list, minus owned ones when filter is set.
func (f *fakePortWorld) conflicts(list []int, filter bool) []docker.PortConflict {
	var out []docker.PortConflict
	for _, p := range list {
		if f.held[p] && !(filter && f.owned[p]) {
			out = append(out, docker.PortConflict{Port: p, InUse: true})
		}
	}
	return out
}

func portSet(ps ...int) map[int]bool {
	m := map[int]bool{}
	for _, p := range ps {
		m[p] = true
	}
	return m
}

// conflictText is what doctor printed for a foreign holder of port before D-0063.
func conflictText(port int) string {
	return ports.FormatConflictMessage(port, &ports.Holder{PID: 4242, Name: "apache2"})
}

func TestDoctorPortOwnHolder(t *testing.T) {
	f := &fakePortWorld{held: portSet(80, 443), owned: portSet(80, 443)}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".env", filepath.Join(".nself", "compose.env")} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("A=1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f.install(t, dir)

	res := checkPorts(false)

	want := []doctorCheckResult{{
		Name: "Reserved ports", Status: "pass",
		Message: fmt.Sprintf("all %d reserved ports available", len(docker.ReservedPorts)),
	}}
	if !reflect.DeepEqual(res, want) {
		t.Fatalf("own stack on 80/443 reported as conflict:\n got %+v\nwant %+v", res, want)
	}
	if f.filteredCalls != 1 || f.allCalls != 0 {
		t.Fatalf("filtered=%d all=%d, want 1 and 0", f.filteredCalls, f.allCalls)
	}
	if f.gotWorkdir != dir {
		t.Fatalf("workdir = %q, want %q", f.gotWorkdir, dir)
	}
	wantEnv := []string{filepath.Join(dir, ".env"), filepath.Join(dir, ".nself", "compose.env")}
	sort.Strings(wantEnv)
	gotEnv := append([]string(nil), f.gotEnvFiles...)
	sort.Strings(gotEnv)
	if !reflect.DeepEqual(gotEnv, wantEnv) {
		t.Fatalf("env files = %v, want %v (start's resolution)", gotEnv, wantEnv)
	}
	if !reflect.DeepEqual(f.gotPorts, docker.ReservedPorts) {
		t.Fatalf("probed %v, want docker.ReservedPorts", f.gotPorts)
	}
}

func TestDoctorPortForeignHolder(t *testing.T) {
	// 80 belongs to an unrelated process, 443 to the project's own nginx.
	f := &fakePortWorld{held: portSet(80, 443), owned: portSet(443)}
	f.install(t, t.TempDir())

	res := checkPorts(false)

	want := []doctorCheckResult{{Name: "Port 80", Status: "warn", Message: conflictText(80)}}
	if !reflect.DeepEqual(res, want) {
		t.Fatalf("foreign holder on 80:\n got %+v\nwant %+v", res, want)
	}
	if f.allCalls != 0 {
		t.Fatal("unfiltered probe used although ownership was known")
	}
}

func TestDoctorPortOwnerQueryFails(t *testing.T) {
	t.Run("conflicts get today's result plus one warning", func(t *testing.T) {
		f := &fakePortWorld{held: portSet(80, 443), owned: portSet(80, 443), ownerErr: errors.New("docker compose ps: exit status 1")}
		f.install(t, t.TempDir())

		res := checkPorts(false)

		warn := doctorCheckResult{
			Name: "Port ownership", Status: "warn",
			Message: "could not read this project's own ports (docker compose ps: exit status 1); conflicts may include its containers",
		}
		want := []doctorCheckResult{
			warn,
			{Name: "Port 80", Status: "warn", Message: conflictText(80)},
			{Name: "Port 443", Status: "warn", Message: conflictText(443)},
		}
		if !reflect.DeepEqual(res, want) {
			t.Fatalf("fallback result:\n got %+v\nwant %+v", res, want)
		}
		if f.filteredCalls != 1 || f.allCalls != 1 {
			t.Fatalf("filtered=%d all=%d, want 1 and 1", f.filteredCalls, f.allCalls)
		}
	})

	t.Run("no conflicts is today's pass with no warning", func(t *testing.T) {
		f := &fakePortWorld{held: portSet(), ownerErr: errors.New("docker not running")}
		f.install(t, t.TempDir())
		res := checkPorts(false)
		if len(res) != 1 || res[0].Status != "pass" || res[0].Name != "Reserved ports" {
			t.Fatalf("got %+v, want the single pass result", res)
		}
	})

	t.Run("project directory cannot be resolved", func(t *testing.T) {
		f := &fakePortWorld{held: portSet(80)}
		f.install(t, "")
		doctorPortProject = func() (string, error) { return "", errors.New("getwd: no such directory") }
		res := checkPorts(false)
		if f.filteredCalls != 0 || f.allCalls != 1 {
			t.Fatalf("filtered=%d all=%d, want 0 and 1", f.filteredCalls, f.allCalls)
		}
		if len(res) != 2 || res[0].Name != "Port ownership" || res[1].Name != "Port 80" {
			t.Fatalf("got %+v", res)
		}
	})

	t.Run("a failing unfiltered probe keeps today's error result", func(t *testing.T) {
		f := &fakePortWorld{ownerErr: errors.New("x")}
		f.install(t, t.TempDir())
		doctorPortsAll = func([]int) ([]docker.PortConflict, error) { return nil, errors.New("probe broke") }
		res := checkPorts(false)
		want := []doctorCheckResult{{Name: "Port check", Status: "warn", Message: "error checking ports: probe broke"}}
		if !reflect.DeepEqual(res, want) {
			t.Fatalf("got %+v want %+v", res, want)
		}
	})
}
