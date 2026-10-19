package service

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
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
	bin = writeFile(t, d, "docker", "#!/bin/sh\nfor a in \"$@\"; do echo \"$a\" >> '"+log+"'; done\n")
	if err := os.Chmod(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, log
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
