package imageswitch

// Purpose: D-0802. RemoveLeftovers removes the old volume a switch kept
// and the project's switch-labelled volumes, leaves every other project's
// volumes alone, and returns list/remove failures instead of swallowing them.
// Inputs: a fake docker that filters a directory of volume label files the
// way `docker volume ls --filter label=...` does, and a state.json.
// Outputs: pass/fail. Constraints: POSIX shell, temp dirs only.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/docker"
)

const fakeVolumeDockerScript = `#!/bin/sh
D="$FAKE_VOLS"
echo "$*" >> "$D/calls.log"
[ -n "$FAKE_LS_FAIL" ] && [ "$1 $2" = "volume ls" ] && { echo "Cannot connect to the Docker daemon" >&2; exit 1; }
[ -n "$FAKE_RM_FAIL" ] && [ "$1 $2" = "volume rm" ] && { echo "volume is in use" >&2; exit 1; }
case "$1 $2" in
"volume ls")
  shift 2; filters=""
  while [ $# -gt 0 ]; do case "$1" in --filter) filters="$filters ${2#label=}"; shift 2;; *) shift;; esac; done
  for f in "$D"/*.labels; do
    [ -e "$f" ] || continue
    name=$(basename "$f" .labels); labels=$(cat "$f"); ok=1
    for want in $filters; do
      case " $labels " in *" $want "*) ;; *) case "$want" in *=*) ok=0;; *) case " $labels " in *" $want="*) ;; *) ok=0;; esac;; esac;; esac
    done
    [ $ok = 1 ] && echo "$name"
  done ;;
"volume inspect")
  for a; do name=$a; done
  [ -e "$D/$name.labels" ] || { echo "Error response from daemon: get $name: no such volume" >&2; exit 1; }
  printf '{'; sep=; for t in $(cat "$D/$name.labels"); do printf '%s"%s":"%s"' "$sep" "${t%%=*}" "${t#*=}"; sep=,; done; printf '}' ;;
"volume rm") rm -f "$D/$3.labels" ;;
esac
exit 0
`

func setupLeftoverFixture(t *testing.T) (project, vols string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	bin, vols, project := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(fakeVolumeDockerScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_VOLS", vols)
	for name, labels := range map[string]string{
		// the kept old volume: compose labels only, no switch label
		"alpha_postgres_data": "com.docker.compose.project=alpha com.docker.compose.volume=postgres_data",
		// the new volume and an earlier switch's volume
		"alpha_postgres_data_pgvector_2": "com.docker.compose.project=alpha org.nself.image-switch=sw-2",
		"alpha_postgres_data_pgvector_1": "com.docker.compose.project=alpha org.nself.image-switch=sw-1",
		// another project's switched volume and unrelated volumes
		"beta_postgres_data_pgvector_1": "com.docker.compose.project=beta org.nself.image-switch=sw-9",
		"beta_postgres_data":            "com.docker.compose.project=beta com.docker.compose.volume=postgres_data",
		"other_data":                    "",
	} {
		if err := os.WriteFile(filepath.Join(vols, name+".labels"), []byte(labels), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	state := `{"version":1,"run_id":"sw-2","phase":"done","compose_project":"alpha",` +
		`"old_volume":"alpha_postgres_data","new_volume":"alpha_postgres_data_pgvector_2"}`
	sd := filepath.Join(project, ".nself", "image-switch")
	if err := os.MkdirAll(sd, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sd, "state.json"), []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	return project, vols
}

func remainingVolumes(t *testing.T, vols string) []string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(vols, "*.labels"))
	var out []string
	for _, f := range files {
		out = append(out, strings.TrimSuffix(filepath.Base(f), ".labels"))
	}
	sort.Strings(out)
	return out
}

func TestRemoveSwitchLeftoversIsScopedToTheProject(t *testing.T) {
	project, vols := setupLeftoverFixture(t)
	removed, err := RemoveLeftovers(context.Background(), project)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(removed)
	want := []string{"alpha_postgres_data", "alpha_postgres_data_pgvector_1", "alpha_postgres_data_pgvector_2"}
	if strings.Join(removed, ",") != strings.Join(want, ",") {
		t.Errorf("removed %v, want %v", removed, want)
	}
	left := remainingVolumes(t, vols)
	if strings.Join(left, ",") != "beta_postgres_data,beta_postgres_data_pgvector_1,other_data" {
		t.Errorf("other projects' volumes were touched: %v", left)
	}
}

func TestRemoveSwitchLeftoversSkipsOldVolumeOfAnotherProject(t *testing.T) {
	project, vols := setupLeftoverFixture(t)
	// A copied project directory: the state names a volume that now belongs to beta.
	if err := os.WriteFile(filepath.Join(vols, "alpha_postgres_data.labels"), []byte("com.docker.compose.project=beta"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveLeftovers(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(remainingVolumes(t, vols), ","), "alpha_postgres_data,") {
		t.Errorf("a volume labelled for another project was removed: %v", remainingVolumes(t, vols))
	}
}

func TestRemoveSwitchLeftoversNoStateTouchesNothing(t *testing.T) {
	_, vols := setupLeftoverFixture(t)
	removed, err := RemoveLeftovers(context.Background(), t.TempDir())
	if err != nil || len(removed) != 0 {
		t.Fatalf("removed %v, err %v", removed, err)
	}
	if _, statErr := os.Stat(filepath.Join(vols, "calls.log")); statErr == nil {
		t.Error("docker was called for a project that never switched")
	}
}

func TestRemoveSwitchLeftoversReturnsListAndRemoveErrors(t *testing.T) {
	project, _ := setupLeftoverFixture(t)
	t.Setenv("FAKE_LS_FAIL", "1")
	if _, err := RemoveLeftovers(context.Background(), project); err == nil || !strings.Contains(err.Error(), "listing image-switch volumes") {
		t.Fatalf("list failure not returned: %v", err)
	}
	t.Setenv("FAKE_LS_FAIL", "")
	t.Setenv("FAKE_RM_FAIL", "1")
	removed, err := RemoveLeftovers(context.Background(), project)
	if err == nil || len(removed) != 0 || !strings.Contains(err.Error(), "alpha_postgres_data") {
		t.Fatalf("remove failure not returned: removed %v err %v", removed, err)
	}
}

// ComposeDown with volumes is what reset, stop --volumes and uninstall --purge
// run; the hook this package registers must make it remove the kept volume.
func TestComposeDownVolumesRemovesSwitchLeftovers(t *testing.T) {
	project, vols := setupLeftoverFixture(t)
	c := &docker.Compose{DockerPath: "docker"}
	if err := c.ComposeDown(context.Background(), project, docker.DownOptions{RemoveVolumes: true, RemoveOrphans: true}); err != nil {
		t.Fatal(err)
	}
	if left := strings.Join(remainingVolumes(t, vols), ","); left != "beta_postgres_data,beta_postgres_data_pgvector_1,other_data" {
		t.Errorf("remaining volumes = %s", left)
	}
}
