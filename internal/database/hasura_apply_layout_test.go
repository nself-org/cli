package database

// Purpose: which applier reads hasura/metadata/ (debt D-0564). With a fake
// hasura binary on PATH, nself's config.yaml index layout must still go
// through the in-process include resolver, and only a real Hasura CLI project
// goes to the binary.
// Inputs: temp project trees, a fake hasura shell script, a loopback httptest
// Hasura that records the request type.
// Outputs: pass/fail on which applier ran.
// Constraints: no real Hasura, no network beyond loopback. The fake binary is
// a POSIX shell script, so the PATH cases skip on Windows.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// fakeHasuraOnPath puts a hasura script first on PATH that records that it ran
// and exits 0. It returns the marker path the script writes.
func fakeHasuraOnPath(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake hasura binary is a POSIX shell script")
	}
	bin := t.TempDir()
	marker := filepath.Join(bin, "called")
	script := "#!/bin/sh\necho \"$@\" > '" + marker + "'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "hasura"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return marker
}

// metadataRecorder is a loopback Hasura that records each request's type.
func metadataRecorder(t *testing.T) (*config.Config, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var types []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Type string `json:"type"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		types = append(types, req.Type)
		mu.Unlock()
		_, _ = w.Write([]byte(`{"message":"success"}`))
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	cfg := &config.Config{}
	cfg.Hasura.Port = port
	return cfg, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), types...)
	}
}

// TestApplyNselfLayoutIgnoresHasuraCLI: nself's export layout (config.yaml
// index plus <key>.yaml) applies through the include resolver even with a
// hasura binary on PATH, also when a hasura/config.yaml names the same
// metadata directory.
func TestApplyNselfLayoutIgnoresHasuraCLI(t *testing.T) {
	for _, withProjectConfig := range []bool{false, true} {
		marker := fakeHasuraOnPath(t)
		cfg, seen := metadataRecorder(t)
		project := t.TempDir()
		if err := writeMetadataTree(filepath.Join(project, "hasura", "metadata"), layoutInput(t)); err != nil {
			t.Fatal(err)
		}
		if withProjectConfig {
			writeFixtureFile(t, project, "hasura/config.yaml", "version: 3\nmetadata_directory: metadata\n")
		}
		if err := HasuraApplyMetadata(context.Background(), cfg, project); err != nil {
			t.Fatalf("project config %v: apply: %v", withProjectConfig, err)
		}
		if _, err := os.Stat(marker); err == nil {
			t.Fatalf("project config %v: nself's layout must not go to the hasura CLI", withProjectConfig)
		}
		if got := seen(); len(got) != 1 || got[0] != "replace_metadata" {
			t.Fatalf("project config %v: want one replace_metadata request, got %v", withProjectConfig, got)
		}
	}
}

// TestApplyHasuraCLIProjectUsesBinary: a real Hasura CLI project
// (hasura/config.yaml with metadata_directory, no nself index) goes to the
// hasura binary on PATH, and nothing is sent to the metadata API directly.
func TestApplyHasuraCLIProjectUsesBinary(t *testing.T) {
	marker := fakeHasuraOnPath(t)
	cfg, seen := metadataRecorder(t)
	project := t.TempDir()
	writeFixtureFile(t, project, "hasura/config.yaml", "version: 3\nmetadata_directory: metadata\n")
	writeFixtureFile(t, project, "hasura/metadata/version.yaml", "version: 3\n")
	writeFixtureFile(t, project, "hasura/metadata/databases/databases.yaml", "[]\n")
	if err := HasuraApplyMetadata(context.Background(), cfg, project); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("a Hasura CLI project must be applied by the hasura binary on PATH")
	}
	if got := seen(); len(got) != 0 {
		t.Fatalf("the binary path must not also post metadata, got %v", got)
	}
}

// TestIsHasuraCLIProject covers the detector's edges without a binary.
func TestIsHasuraCLIProject(t *testing.T) {
	cases := []struct {
		name   string
		files  map[string]string
		expect bool
	}{
		{"cli project", map[string]string{"hasura/config.yaml": "metadata_directory: metadata\n"}, true},
		{"cli project ./metadata", map[string]string{"hasura/config.yaml": "metadata_directory: ./metadata\n"}, true},
		{"no project config", map[string]string{"hasura/metadata/tables.yaml": "[]\n"}, false},
		{"default metadata_directory", map[string]string{"hasura/config.yaml": "version: 3\n"}, true},
		{"other metadata_directory", map[string]string{"hasura/config.yaml": "metadata_directory: md\n"}, false},
		{"unparsable config", map[string]string{"hasura/config.yaml": "metadata_directory: [\n"}, false},
		{"nself index wins", map[string]string{
			"hasura/config.yaml":          "metadata_directory: metadata\n",
			"hasura/metadata/config.yaml": "tables: !include tables.yaml\n",
		}, false},
	}
	for _, c := range cases {
		project := t.TempDir()
		if err := os.MkdirAll(filepath.Join(project, "hasura", "metadata"), 0o755); err != nil {
			t.Fatal(err)
		}
		for name, body := range c.files {
			writeFixtureFile(t, project, name, body)
		}
		hasuraDir := filepath.Join(project, "hasura")
		if got := isHasuraCLIProject(hasuraDir, filepath.Join(hasuraDir, "metadata")); got != c.expect {
			t.Errorf("%s: isHasuraCLIProject = %v, want %v", c.name, got, c.expect)
		}
	}
}

func TestIsHasuraCLIProjectSymlinkDirectory(t *testing.T) {
	project := t.TempDir()
	metadata := filepath.Join(project, "metadata")
	if err := os.Mkdir(metadata, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(project, "linked-metadata")
	if err := os.Symlink(metadata, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	writeFixtureFile(t, project, "config.yaml", "metadata_directory: linked-metadata\n")
	if !isHasuraCLIProject(project, metadata) {
		t.Fatal("metadata directory aliases must identify the same CLI project")
	}
}
