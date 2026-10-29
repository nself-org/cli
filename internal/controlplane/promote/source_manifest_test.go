package promote

import (
	"context"
	"fmt"
	"github.com/nself-org/cli/internal/deploy"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvedComposeImagesIgnoresServiceEnvFiles(t *testing.T) {
	run := func(_ context.Context, command string) ([]byte, error) {
		if !strings.Contains(command, "--no-env-resolution") {
			return nil, fmt.Errorf("service env_file names target paths unavailable locally")
		}
		return []byte(`{"services":{"api":{"image":"example.invalid/api:fixture"}}}`), nil
	}
	images, err := ResolvedComposeImages(context.Background(), run, []string{"/srv/fixture/docker-compose.yml"}, []string{"/srv/fixture/.env.prod"})
	if err != nil || len(images) != 1 || images[0].Ref != "example.invalid/api:fixture" {
		t.Fatalf("image resolution: %#v, %v", images, err)
	}
}

func TestCaptureSourceFullManifest(t *testing.T) {
	var commands []string
	run := func(_ context.Context, command string) ([]byte, error) {
		commands = append(commands, command)
		switch {
		case strings.Contains(command, "compose-files.txt"):
			return []byte("docker-compose.yml\n.nself/fragments/api.yml\n"), nil
		case strings.Contains(command, "compose-env-files.txt"):
			return []byte(".env\n.env.prod\n"), nil
		case strings.Contains(command, " images --format json"):
			if !strings.Contains(command, " -f '/srv/fixture/.nself/fragments/api.yml'") || !strings.Contains(command, " --env-file '/srv/fixture/.env.prod'") {
				return nil, fmt.Errorf("missing full manifest: %s", command)
			}
			return []byte(`[{"Service":"api"}]`), nil
		}
		return nil, fmt.Errorf("unexpected command %s", command)
	}
	_, raw, err := CaptureSourceManifest(context.Background(), run, "/srv/fixture")
	if err != nil || string(raw) != `[{"Service":"api"}]` {
		t.Fatalf("capture: %s %v commands=%v", raw, err, commands)
	}
}

func TestCaptureSourceManifestRejectsUnsafePath(t *testing.T) {
	ran := false
	run := func(_ context.Context, command string) ([]byte, error) {
		if strings.Contains(command, "compose-files.txt") {
			return []byte("/srv/fixture/unsafe;touch-pwn\n"), nil
		}
		ran = true
		return nil, nil
	}
	_, _, err := CaptureSourceManifest(context.Background(), run, "/srv/fixture")
	if err == nil || ran {
		t.Fatalf("unsafe path reached command: %v ran=%v", err, ran)
	}
}

func TestResolvedArtifactEnvSnapshot(t *testing.T) {
	stage := t.TempDir()
	snapshot := filepath.Join(stage, ".env.prod")
	if err := os.WriteFile(snapshot, []byte("IMAGE=target-digest\n"), 0600); err != nil {
		t.Fatal(err)
	}
	set := deploy.ArtifactSet{StageDir: stage, RemotePath: "/srv/fixture", Compose: []string{"/srv/fixture/docker-compose.yml", "/srv/fixture/override.yml"}, EnvManifest: []string{"/srv/fixture/.env", "/srv/fixture/.env.prod"}, KeepHostEnv: true}
	var private string
	host := func(_ context.Context, command string) ([]byte, error) {
		if !strings.Contains(command, "cat '/srv/fixture/.env'") {
			t.Fatalf("host env read: %s", command)
		}
		return []byte("IMAGE=host-digest\n"), nil
	}
	local := func(_ context.Context, command string) ([]byte, error) {
		for _, part := range strings.Split(command, "'") {
			if strings.Contains(part, "promote-host-env-") {
				private = part
			}
		}
		raw, err := os.ReadFile(private)
		if err != nil || string(raw) != "IMAGE=host-digest\n" {
			t.Fatalf("private env: %s %v", raw, err)
		}
		if !strings.Contains(command, " -f '"+filepath.Join(stage, "override.yml")+"'") || !strings.Contains(command, " --env-file '"+snapshot+"'") {
			t.Fatalf("staged model missing snapshot/fragment: %s", command)
		}
		return []byte(`{"services":{"api":{"image":"target-digest"}}}`), nil
	}
	images, err := ResolvedArtifactImages(context.Background(), local, host, set)
	if err != nil || len(images) != 1 || images[0].Ref != "target-digest" {
		t.Fatalf("staged images: %v %v", images, err)
	}
	if _, err := os.Stat(private); !os.IsNotExist(err) {
		t.Fatalf("private env survived: %v", err)
	}
}

// Local staged paths keep their platform form: a space, a quote or a Windows
// drive path must reach the local model quoted, never fail the remote charset.
func TestResolvedComposeImagesLocalPaths(t *testing.T) {
	files := []string{`/Users/jane doe/it's/docker-compose.yml`, `C:\Users\RUNNER~1\Temp\001\override.yml`}
	envs := []string{`/Users/jane doe/.env`}
	var got string
	run := func(_ context.Context, command string) ([]byte, error) {
		got = command
		return []byte(`{"services":{"api":{"image":"fixture"}}}`), nil
	}
	images, err := ResolvedComposeImages(context.Background(), run, files, envs)
	if err != nil || len(images) != 1 {
		t.Fatalf("local paths refused: %v %v", images, err)
	}
	want := `docker compose -f '/Users/jane doe/it'\''s/docker-compose.yml' -f 'C:\Users\RUNNER~1\Temp\001\override.yml' --env-file '/Users/jane doe/.env' config`
	if !strings.HasPrefix(got, want) {
		t.Fatalf("local command = %s", got)
	}
	for _, bad := range [][]string{{""}, {"/srv/a\x00b.yml"}} {
		if _, err := ResolvedComposeImages(context.Background(), run, bad, nil); err == nil {
			t.Fatalf("invalid local path %q accepted", bad[0])
		}
	}
}

// D-0701: the ps listing that maps containers to services is read at capture,
// next to the images listing, and replayed at compare. A container that moves
// to another service in between cannot change the mapping.
func TestCaptureSourceManifestReplaysPS(t *testing.T) {
	ctx := context.Background()
	psReads := 0
	ps := []string{`{"Name":"proj-api-1","Service":"api"}` + "\n" + `{"Name":"proj-db-1","Service":"db"}`, `{"Name":"proj-api-1","Service":"db"}` + "\n" + `{"Name":"proj-db-1","Service":"api"}`}
	run := func(_ context.Context, command string) ([]byte, error) {
		switch {
		case strings.Contains(command, ".nself/compose"):
			return []byte("__NSELF_ABSENT__"), nil
		case strings.HasSuffix(command, " images --format json"):
			return []byte(`[{"ID":"sha256:a","ContainerName":"proj-api-1","Repository":"example.invalid/api","Tag":"v1"},{"ID":"sha256:d","ContainerName":"proj-db-1","Repository":"example.invalid/db","Tag":"v1"}]`), nil
		case strings.HasSuffix(command, " ps --all --format json"):
			psReads++
			return []byte(ps[min(psReads-1, len(ps)-1)]), nil // the second read is the swapped one
		case strings.Contains(command, " config --format json"):
			return []byte(`{"services":{"api":{"image":"example.invalid/api:v1"},"db":{"image":"example.invalid/db:v1"}}}`), nil
		case strings.HasPrefix(command, "docker image inspect"):
			repo := "example.invalid/api"
			if strings.Contains(command, "sha256:d") {
				repo = "example.invalid/db"
			}
			return []byte(`["` + repo + `@sha256:` + strings.Repeat("c", 64) + `"]|linux/amd64`), nil
		}
		return nil, fmt.Errorf("unexpected command %s", command)
	}
	lookup := func(_ context.Context, ref string) (map[string]string, error) {
		return map[string]string{"linux/amd64": "sha256:" + strings.Repeat(map[bool]string{true: "a", false: "b"}[strings.Contains(ref, "/api")], 64)}, nil
	}
	captured, raw, err := CaptureSourceManifest(ctx, run, "/srv/fixture")
	if err != nil || psReads != 1 {
		t.Fatalf("capture: %v, ps reads %d", err, psReads)
	}
	images, err := CaptureManifestSource(ctx, captured, raw, lookup)
	if err != nil || psReads != 1 {
		t.Fatalf("compare: %v, ps reads %d (the compare must not list containers again)", err, psReads)
	}
	got := map[string]string{}
	for _, image := range images {
		got[image.Service] = image.Ref
	}
	if got["api"] != "example.invalid/api:v1" || got["db"] != "example.invalid/db:v1" {
		t.Fatalf("mapping followed the later ps listing: %#v", got)
	}
}
