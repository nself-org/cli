package commands

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// switchVolumesProject builds a project that went through `nself db image
// switch` (state names the kept OLD volume and the NEW one) and a fake docker
// that logs every call; `volume ls` reports one more labeled leftover.
func switchVolumesProject(t *testing.T) (dir, log string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	dir = t.TempDir()
	bin := t.TempDir()
	log = filepath.Join(bin, "calls.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + log + "'\n" +
		"case \"$1 $2\" in 'volume ls') printf 'shop_postgres_data_pgvector_x\\n';; esac\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	state := `{"version":1,"run_id":"sw-1","phase":"done","compose_project":"shop",` +
		`"old_volume":"shop_postgres_data","new_volume":"shop_postgres_data_pgvector_x"}`
	sd := filepath.Join(dir, ".nself", "image-switch")
	if err := os.MkdirAll(sd, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sd, "state.json"), []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, log
}

func callLog(t *testing.T, log string) string {
	t.Helper()
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// D-0802: a purge must remove the volumes a postgres image switch kept, not
// only the ones the current compose file names.
func TestPurgeRemovesImageSwitchVolumes(t *testing.T) {
	dir, log := switchVolumesProject(t)
	if _, err := runDockerComposeDown(context.Background(), nil, nil, dir, true); err != nil {
		t.Fatal(err)
	}
	calls := callLog(t, log)
	for _, v := range []string{"shop_postgres_data", "shop_postgres_data_pgvector_x"} {
		if !strings.Contains(calls, "volume rm "+v+"\n") {
			t.Errorf("volume %s was not removed; docker calls:\n%s", v, calls)
		}
	}
}

// Keeping data (no purge) must leave every volume alone.
func TestKeepDataLeavesImageSwitchVolumes(t *testing.T) {
	dir, log := switchVolumesProject(t)
	if _, err := runDockerComposeDown(context.Background(), nil, nil, dir, false); err != nil {
		t.Fatal(err)
	}
	if calls := callLog(t, log); strings.Contains(calls, "volume rm") {
		t.Errorf("no-purge removed a volume:\n%s", calls)
	}
}

// A project that never switched images has nothing extra to remove.
func TestNoSwitchStateRemovesNoExtraVolumes(t *testing.T) {
	dir, log := switchVolumesProject(t)
	if err := os.RemoveAll(filepath.Join(dir, ".nself")); err != nil {
		t.Fatal(err)
	}
	if err := removeSwitchVolumes(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(log); strings.Contains(string(b), "volume rm") {
		t.Errorf("volume removed without a switch:\n%s", b)
	}
}
