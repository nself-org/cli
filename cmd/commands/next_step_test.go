package commands

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
)

const nextStepDockerStub = `#!/bin/sh
case "$1" in
  info)
    case "$NEXTSTEP_DOCKER" in down) exit 1;; slow) sleep 10;; esac
    echo 1.0;;
  compose)
    case "$NEXTSTEP_DOCKER" in
      built) echo '[]';;
      running) printf '%s\n' '[{"Service":"postgres","Health":"healthy"},{"Service":"hasura","Health":"healthy"},{"Service":"auth","Health":"healthy"},{"Service":"nginx","Health":"healthy"}]';;
      unhealthy) printf '%s\n' '[{"Service":"postgres","Health":"unhealthy"},{"Service":"hasura","Health":"healthy"},{"Service":"auth","Health":"healthy"},{"Service":"nginx","Health":"healthy"}]';;
      *) exit 2;;
    esac;;
  inspect)
    case "$NEXTSTEP_DOCKER" in service) echo healthy;; *) echo 'No such object' >&2; exit 1;; esac;;
  *) exit 2;;
esac
`

// On Windows, a copied test executable is the Docker stub. A batch wrapper
// would leave its child running after context cancellation and lock TempDir.
func init() {
	if os.Getenv("NSELF_TEST_DOCKER_HELPER") != "1" {
		return
	}
	mode := os.Getenv("NEXTSTEP_DOCKER")
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	switch os.Args[1] {
	case "info":
		if mode == "down" {
			os.Exit(1)
		}
		if mode == "slow" {
			time.Sleep(10 * time.Second)
		}
		fmt.Println("1.0")
	case "compose":
		switch mode {
		case "built":
			fmt.Println("[]")
		case "running", "unhealthy":
			health := "healthy"
			if mode == "unhealthy" {
				health = "unhealthy"
			}
			fmt.Printf("[{\"Service\":\"postgres\",\"Health\":%q},{\"Service\":\"hasura\",\"Health\":\"healthy\"},{\"Service\":\"auth\",\"Health\":\"healthy\"},{\"Service\":\"nginx\",\"Health\":\"healthy\"}]\n", health)
		default:
			os.Exit(2)
		}
	case "inspect":
		if mode != "service" {
			fmt.Fprintln(os.Stderr, "No such object")
			os.Exit(1)
		}
		fmt.Println("healthy")
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

func nextStepFixture(t *testing.T, stage string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", filepath.Join(dir, "home"))
	if err := os.MkdirAll(filepath.Join(dir, "home"), 0o755); err != nil {
		t.Fatal(err)
	}
	if stage != "empty" {
		for _, name := range []string{".env", "docker-compose.yml"} {
			if stage == "initialised" && name == "docker-compose.yml" {
				break
			}
			data := []byte("PROJECT_NAME=nextstep\n")
			if name == "docker-compose.yml" {
				data = []byte("services:\n  postgres:\n    image: postgres:16\n")
			}
			if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		copyDockerTestBinary(t, filepath.Join(bin, "docker.exe"))
		t.Setenv("NSELF_TEST_DOCKER_HELPER", "1")
	} else if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(nextStepDockerStub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func copyDockerTestBinary(t *testing.T, path string) {
	t.Helper()
	source, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNextStep(t *testing.T) {
	for _, tc := range []struct{ stage, mode, want string }{
		{"empty", "built", "init"}, {"initialised", "built", "start"},
		{"project", "built", "start"}, {"project", "running", "status"},
		{"project", "unhealthy", "doctor"}, {"project", "down", "doctor"},
		{"project", "slow", "doctor"},
	} {
		t.Run(tc.stage+"_"+tc.mode, func(t *testing.T) {
			nextStepFixture(t, tc.stage)
			t.Setenv("NEXTSTEP_DOCKER", tc.mode)
			start := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), nextStepTimeout)
			defer cancel()
			if got := nextStep(ctx); got != "Next: nself "+tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
			if elapsed := time.Since(start); elapsed > 4*time.Second {
				t.Errorf("detection took %v", elapsed)
			}
		})
	}
}

func TestBareNself(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		nextStepFixture(t, "empty")
		var out bytes.Buffer
		root := *RootCmd
		root.SetOut(&out)
		root.SetContext(context.Background())
		if err := root.RunE(&root, nil); err != nil {
			t.Fatal(err)
		}
		got := out.String()
		if strings.Contains(got, "Next: nself") != strings.Contains(t.Name(), "v1.5") {
			t.Fatalf("unexpected next line in %s: %q", t.Name(), got)
		}
		if strings.Contains(t.Name(), "v1.5") && !strings.HasPrefix(got, "Next: nself init\n\n") {
			t.Fatalf("missing next line and blank line: %q", got)
		}
	})
}
