package docker

// Purpose: the init-container cleanup removes only finished, successful
//          containers (P7-PLUG-60): a running init container or a failed one
//          (kept for `docker logs`) must never be selected.

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestPostStartCleanupScopesInitContainersToProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a shell script")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "calls")
	t.Setenv("CLEANUP_CALLS", log)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	// Model two projects: an unscoped init query selects both; a scoped
	// query selects only project-a. Other cleanup queries return no IDs.
	script := `#!/bin/sh
case " $* " in
  *" label=nself.type=init-container "*)
    printf 'init %s\n' "$*" >> "$CLEANUP_CALLS"
    printf 'project-a-init\n'
    case " $* " in
      *" label=com.docker.compose.project=project-a "*) ;;
      *) printf 'project-b-init\n' ;;
    esac ;;
  " rm -f "*) printf '%s\n' "$*" >> "$CLEANUP_CALLS" ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RunPostStartCleanup(context.Background(), "project-a"); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	initQueries, removals := 0, 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(line, "init ") {
			initQueries++
			if !strings.Contains(line, "--filter label=com.docker.compose.project=project-a") {
				t.Errorf("init query missing project filter: %s", line)
			}
		} else {
			removals++
			if line != "rm -f project-a-init" {
				t.Errorf("removed another project's container: %s", line)
			}
		}
	}
	if initQueries != 3 || removals != 3 {
		t.Errorf("got %d init queries and %d removals, want 3 each", initQueries, removals)
	}
}

func TestInitContainerListArgsSelectExitedZeroOnly(t *testing.T) {
	want := []string{"ps", "-aq",
		"--filter", "label=nself.type=init-container",
		"--filter", "label=com.docker.compose.project=project-a",
		"--filter", "status=exited",
		"--filter", "exited=0",
	}
	if got := initContainerListArgs("project-a"); !reflect.DeepEqual(got, want) {
		t.Fatalf("initContainerListArgs() = %v, want %v", got, want)
	}
}
