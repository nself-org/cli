package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// writeFile writes body under dir (creating parents) and returns its path.
func writeFile(t *testing.T, dir, rel, body string) string {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// fakeDocker writes a docker stand-in that records its argv, one arg per line.
func fakeDocker(t *testing.T) (bin, log string) {
	t.Helper()
	d := t.TempDir()
	log = filepath.Join(d, "argv.txt")
	// Copy the test executable so exec.Command and PATH lookup use a native
	// executable on every OS, without depending on a shell or a real Docker.
	name := "docker"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	bin = filepath.Join(d, name)
	if err := os.WriteFile(bin, data, 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, log
}

// The copied executable acts only as Docker; normal test binaries continue
// into testing.Main. Recording beside the executable survives environment sealing.
func init() {
	name := strings.ToLower(filepath.Base(os.Args[0]))
	if name != "docker" && name != "docker.exe" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	dir := filepath.Dir(exe)
	if err := os.WriteFile(filepath.Join(dir, "argv.txt"), []byte(strings.Join(os.Args[1:], "\n")+"\n"), 0o600); err != nil {
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(dir, "seen.txt"), []byte("["+os.Getenv("POSTGRES_PASSWORD")+"]\n"), 0o600); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func TestFakeDockerExecutable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		lookup bool
	}{
		{name: "absolute path"},
		{name: "PATH lookup", lookup: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin, log := fakeDocker(t)
			if tc.lookup {
				t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
				bin = "docker"
			}
			args := []string{"compose", "argument with spaces", "--", "minio"}
			if out, err := exec.Command(bin, args...).CombinedOutput(); err != nil {
				t.Fatalf("stub: %v: %s", err, out)
			}
			raw, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Split(strings.TrimSuffix(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n"), "\n")
			if !reflect.DeepEqual(got, args) {
				t.Fatalf("argv = %q, want %q", got, args)
			}
		})
	}
}

// composeFor follows the build's recorded manifests (D-0703): plugin fragments
// become -f files and the project env files become --env-file, exactly as
// `nself start` composes them. Without a manifest it is the base file alone.
func TestComposeForFollowsBuildManifests(t *testing.T) {
	t.Run("legacy project without manifests", func(t *testing.T) {
		dir := t.TempDir()
		base := writeFile(t, dir, "docker-compose.yml", "services: {}\n")
		c, err := composeFor(dir)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{base}; !reflect.DeepEqual(c.ComposeFiles, want) {
			t.Fatalf("ComposeFiles = %v, want %v", c.ComposeFiles, want)
		}
		if len(c.EnvFiles) != 0 {
			t.Fatalf("EnvFiles = %v, want none", c.EnvFiles)
		}
	})

	t.Run("manifest with plugin fragment and compose.env", func(t *testing.T) {
		dir := t.TempDir()
		base := writeFile(t, dir, "docker-compose.yml", "services: {}\n")
		frag := writeFile(t, dir, "plugins/minio/docker-compose.plugin.yml", "services: {minio: {image: x}}\n")
		dotenv := writeFile(t, dir, ".env", "A=1\n")
		computed := writeFile(t, dir, ".nself/compose.env", "MINIO_ROOT_PASSWORD=from-project\n")
		writeFile(t, dir, ".nself/compose-files.txt", base+"\n"+frag+"\n")
		c, err := composeFor(dir)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{base, frag}; !reflect.DeepEqual(c.ComposeFiles, want) {
			t.Fatalf("ComposeFiles = %v, want %v", c.ComposeFiles, want)
		}
		if want := []string{dotenv, computed}; !reflect.DeepEqual(c.EnvFiles, want) {
			t.Fatalf("EnvFiles = %v, want %v", c.EnvFiles, want)
		}

		// The argv the docker binary sees carries both -f files and both env files.
		bin, log := fakeDocker(t)
		c.DockerPath = bin
		if err := c.ComposeUp(context.Background(), dir, "minio"); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		got := strings.Join(strings.Fields(string(raw)), " ")
		want := strings.Join([]string{"compose", "-f", base, "-f", frag, "--env-file", dotenv, "--env-file", computed, "up", "-d", "--", "minio"}, " ")
		if got != want {
			t.Fatalf("docker argv\n got: %s\nwant: %s", got, want)
		}
	})
}

// service.Start runs compose with the same sealed environment as `nself start`
// and restart: a POSTGRES_PASSWORD exported in the shell never reaches compose,
// so the value in --env-file wins (D-0703 review).
func TestStartDropsExportedProjectVariables(t *testing.T) {
	bin, _ := fakeDocker(t)
	seen := filepath.Join(filepath.Dir(bin), "seen.txt")
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("POSTGRES_PASSWORD", "exported-in-shell")
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "docker-compose.yml", "services:\n  postgres:\n    image: x\n")
	writeFile(t, dir, ".env", "PROJECT_NAME=t\n")
	t.Chdir(dir)
	if err := Start(context.Background(), "postgres"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(seen)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(raw)); got != "[]" {
		t.Fatalf("compose saw POSTGRES_PASSWORD %s, want it unset", got)
	}
}
