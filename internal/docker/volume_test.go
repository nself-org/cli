package docker

// Tests for volume.go and the Name/Labels fields of RunSpec. internal/docker has
// no exec seam, so a fake `docker` shell script goes first on PATH and logs its calls.

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// fakeVolumeDocker installs a docker whose stdout is out and whose stderr line is
// msg (exit 1) when msg is set; it returns the call log path.
func fakeVolumeDocker(t *testing.T, out, msg string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + log + "'\n"
	if msg != "" {
		script += "echo '" + msg + "' >&2\nexit 1\n"
	} else {
		script += "printf '%s' '" + out + "'\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func lastCall(t *testing.T, log string) string {
	t.Helper()
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	return lines[len(lines)-1]
}

func TestVolumeCreateRendersSortedLabels(t *testing.T) {
	log := fakeVolumeDocker(t, "", "")
	err := VolumeCreate(context.Background(), "proj_pgdata", map[string]string{"b": "2", "a": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := lastCall(t, log), "volume create --label a=1 --label b=2 proj_pgdata"; got != want {
		t.Errorf("call = %q, want %q", got, want)
	}
}

func TestVolumeExistsAndRemoveTreatAbsentAsNormal(t *testing.T) {
	fakeVolumeDocker(t, "", "Error response from daemon: get x: no such volume")
	ok, err := VolumeExists(context.Background(), "x")
	if err != nil || ok {
		t.Errorf("an absent volume: exists %v err %v", ok, err)
	}
	if err := VolumeRemove(context.Background(), "x"); err != nil {
		t.Errorf("removing an absent volume must succeed: %v", err)
	}
	fakeVolumeDocker(t, "", "Error response from daemon: remove x: volume is in use")
	if err := VolumeRemove(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "in use") {
		t.Errorf("a volume in use must fail: %v", err)
	}
	if ok, err := VolumeExists(context.Background(), "x"); err == nil || ok {
		t.Errorf("an unexpected docker error must not read as absent: %v %v", ok, err)
	}
	fakeVolumeDocker(t, "x\n", "")
	if ok, err := VolumeExists(context.Background(), "x"); err != nil || !ok {
		t.Errorf("a present volume: %v %v", ok, err)
	}
}

func TestRemoveContainerAbsentIsSuccess(t *testing.T) {
	fakeVolumeDocker(t, "", "Error response from daemon: No such container: nself-imgswitch-p-dump")
	if err := RemoveContainer(context.Background(), "nself-imgswitch-p-dump"); err != nil {
		t.Errorf("removing an absent container must succeed: %v", err)
	}
	log := fakeVolumeDocker(t, "", "")
	if err := RemoveContainer(context.Background(), "c"); err != nil {
		t.Fatal(err)
	}
	if got := lastCall(t, log); got != "rm -f c" {
		t.Errorf("call = %q", got)
	}
}

func TestContainerNetworksAndVolume(t *testing.T) {
	fakeVolumeDocker(t, `{"proj_network":{"IPAddress":"1"},"bridge":{}}`, "")
	got, err := ContainerNetworks(context.Background(), "c")
	if err != nil || !reflect.DeepEqual(got, []string{"bridge", "proj_network"}) {
		t.Errorf("networks = %v, %v", got, err)
	}
	fakeVolumeDocker(t, "not json", "")
	if _, err := ContainerNetworks(context.Background(), "c"); err == nil {
		t.Error("unreadable inspect output must fail")
	}
	log := fakeVolumeDocker(t, "proj_postgres_data\n", "")
	name, err := ContainerVolume(context.Background(), "c", "/var/lib/postgresql/data")
	if err != nil || name != "proj_postgres_data" {
		t.Errorf("volume = %q, %v", name, err)
	}
	if call := lastCall(t, log); !strings.Contains(call, `eq .Destination "/var/lib/postgresql/data"`) || !strings.Contains(call, `eq .Type "volume"`) {
		t.Errorf("call = %q", call)
	}
}

func TestRunSpecNameAndLabelsAreRendered(t *testing.T) {
	args, err := buildRunArgs(RunSpec{Image: "img", Name: "n1", Labels: map[string]string{"z": "1", "a": "2"}, Args: []string{"x"}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(args, " "), "run --rm --name n1 --label a=2 --label z=1 img x"; got != want {
		t.Errorf("args = %q, want %q", got, want)
	}
}
