package exporter

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/portable"
)

func fakeTools(t *testing.T, output string, fail bool) {
	t.Helper()
	oldRun, oldInspect := runTool, inspectContainer
	t.Cleanup(func() { runTool = oldRun; inspectContainer = oldInspect })
	inspectContainer = func(context.Context, string) (*docker.ContainerInfo, error) {
		return &docker.ContainerInfo{Image: "fake-postgres-image"}, nil
	}
	runTool = func(_ context.Context, spec docker.RunSpec) (string, string, error) {
		if spec.EnvPass["PGPASSWORD"] != "FAKE-DATABASE-PASSWORD" || spec.User == "" || spec.Network != "fake-project_network" {
			t.Fatal("unsafe tool spec")
		}
		if strings.Contains(strings.Join(spec.Args, " "), "FAKE-DATABASE-PASSWORD") {
			t.Fatal("secret in argv")
		}
		if fail {
			return "", "FAKE-TOOL-SECRET", errors.New("FAKE-TOOL-SECRET")
		}
		for i, a := range spec.Args {
			if a == "-f" {
				path := strings.TrimPrefix(spec.Args[i+1], "/stage/")
				if err := os.WriteFile(filepath.Join(spec.Mounts[0].Source, path), []byte(output), 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
		return "", "", nil
	}
}

func fakeConfig() *config.Config {
	return &config.Config{ProjectName: "fake-project", Postgres: config.PostgresConfig{Password: "FAKE-DATABASE-PASSWORD"}}
}

func TestScrubArchive(t *testing.T) {
	for _, tc := range []struct {
		output    string
		fail, hit bool
	}{{"normal SQL", false, false}, {"COPY public.notes FROM stdin;\nFAKE-ROW-SECRET\n", false, true}, {"", true, false}} {
		t.Run(tc.output, func(t *testing.T) {
			fakeTools(t, tc.output, tc.fail)
			dir := filepath.Join(t.TempDir(), "bundle")
			w, err := portable.NewWriter(dir)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = w.WriteFile("db/data.dump", strings.NewReader("fake opaque dump")); err != nil {
				t.Fatal(err)
			}
			m, err := w.Finish(portable.Manifest{})
			if err != nil {
				t.Fatal(err)
			}
			err = scrubBundle(context.Background(), fakeConfig(), dir, m, []string{"FAKE-ROW-SECRET"})
			if tc.hit {
				assertCode(t, err, "E526")
				if !strings.Contains(err.Error(), "db/data.dump") {
					t.Fatal("file unnamed")
				}
			} else if tc.fail {
				if err == nil || strings.Contains(err.Error(), "FAKE-TOOL-SECRET") {
					t.Fatal("tool failed open or leaked")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(filepath.Join(dir, ".scan.sql")); !os.IsNotExist(err) {
				t.Fatal("SQL scan not removed")
			}
		})
	}
}

func TestScrubAllTextMembers(t *testing.T) {
	for _, member := range []string{"db/schema.sql", "hasura/metadata.json", "config/keys.txt", "auth/users.jsonl"} {
		t.Run(member, func(t *testing.T) {
			fakeTools(t, "innocent SQL", false)
			dir := filepath.Join(t.TempDir(), "bundle")
			w, err := portable.NewWriter(dir)
			if err != nil {
				t.Fatal(err)
			}
			text := "FAKE-TEXT-SECRET"
			if member == "auth/users.jsonl" {
				text = `{"metadata":"FAKE-TEXT-SECRET"}`
			}
			if _, err = w.WriteFile(member, strings.NewReader(text)); err != nil {
				t.Fatal(err)
			}
			m, err := w.Finish(portable.Manifest{})
			if err != nil {
				t.Fatal(err)
			}
			assertCode(t, scrubBundle(context.Background(), fakeConfig(), dir, m, []string{"FAKE-TEXT-SECRET"}), "E526")
			if err = os.Remove(filepath.Join(dir, filepath.FromSlash(member))); err != nil {
				t.Fatal(err)
			}
			if err = scrubBundle(context.Background(), fakeConfig(), dir, m, nil); err == nil {
				t.Fatal("missing member accepted")
			}
		})
	}
}

func TestScrubManifestAndObjectExemption(t *testing.T) {
	for _, manifestSecret := range []bool{false, true} {
		fakeTools(t, "innocent SQL", false)
		w, err := portable.NewWriter(filepath.Join(t.TempDir(), "bundle"))
		if err != nil {
			t.Fatal(err)
		}
		member, err := portable.StorageMember("fixture", "fake-user-file")
		if err != nil {
			t.Fatal(err)
		}
		f, err := w.WriteFile(member, strings.NewReader("FAKE-USER-OBJECT-SECRET"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.WriteFile("db/data.dump", strings.NewReader("fake opaque dump")); err != nil {
			t.Fatal(err)
		}
		m := portable.Manifest{Storage: portable.Storage{Buckets: []portable.Bucket{{Name: "fixture", Objects: 1, Bytes: f.Bytes}}, Objects: []portable.Object{{Bucket: "fixture", Key: "fake-user-file", Member: member, Bytes: f.Bytes, SHA256: f.SHA256}}}}
		if manifestSecret {
			m.Producer.Version = "FAKE-USER-OBJECT-SECRET"
		}
		m, err = w.Finish(m)
		if err != nil {
			t.Fatal(err)
		}
		err = scrubBundle(context.Background(), fakeConfig(), w.Dir(), m, []string{"FAKE-USER-OBJECT-SECRET"})
		if manifestSecret {
			assertCode(t, err, "E526")
		} else if err != nil {
			t.Fatal("object exemption failed", err)
		}
	}
}

func TestArchiveCanonicalHeader(t *testing.T) {
	for _, minor := range []byte{14, 15, 16} {
		b := make([]byte, 100)
		copy(b, "PGDMP")
		b[5] = 1
		b[6] = minor
		b[8] = 4
		b[9] = 8
		b[10] = 1
		start := 12
		if minor == 14 {
			start = 16
		}
		for i := start; i < start+35; i++ {
			b[i] = 22
		}
		path := filepath.Join(t.TempDir(), "dump")
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := canonicalArchive(path); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got[:start], b[:start]) || !bytes.Equal(got[start+35:], b[start+35:]) {
			t.Fatal("changed non-timestamp bytes")
		}
		if got[start+16] != 1 || got[start+26] != 70 {
			t.Fatal("timestamp not epoch")
		}
		if err := os.WriteFile(path, []byte("bad"), 0o600); err != nil {
			t.Fatal(err)
		}
		if canonicalArchive(path) == nil {
			t.Fatal("bad header accepted")
		}
	}
	long := strings.Repeat("x", 200<<10)
	if got, err := io.ReadAll(newSQLReader(strings.NewReader("INSERT " + long + ";\n"))); err != nil || len(got) != len(long)+9 {
		t.Fatal("long schema line failed", err)
	}
	r := &sqlReader{scanner: bufio.NewScanner(strings.NewReader("-- schema\n\\restrict random\nSELECT 1;\n\\unrestrict random\n"))}
	b, err := io.ReadAll(r)
	if err != nil || string(b) != "-- schema\nSELECT 1;\n" {
		t.Fatal("random guards retained", err)
	}
}
