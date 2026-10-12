package build

// nginx_sites_prune_shared_test.go — P7-ADOPT-03: the owner's prune leaves every
// file a registration lists (EPIC ADOPT D2, review F3).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/sharedproxy"
)

func TestPruneSkipsRegisteredFiles(t *testing.T) {
	root := t.TempDir()
	sites := filepath.Join(root, "nginx", "sites")
	shared := filepath.Join(root, "nginx", "shared")
	put := func(path, body string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	marked := nginxGeneratedMarker + " (project bproj)\nserver {}\n"
	put(filepath.Join(sites, "hasura.conf"), nginxGeneratedMarker+" - DO NOT EDIT MANUALLY\n")
	put(filepath.Join(sites, "old.conf"), nginxGeneratedMarker+" - DO NOT EDIT MANUALLY\n")
	put(filepath.Join(sites, "bproj.hasura.conf"), marked)
	put(filepath.Join(sites, "hand.conf"), "server {}\n")
	reg := sharedproxy.New("bproj", filepath.Join(root, "backend"))
	reg.Files = []string{"nginx/sites/bproj.hasura.conf"}
	data, err := sharedproxy.Marshal(reg)
	if err != nil {
		t.Fatal(err)
	}
	put(filepath.Join(shared, "bproj.json"), string(data))

	removed, foreign, err := pruneGeneratedNginxSitesVia(newDiskSink(""), sites)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 || strings.Join(foreign, ",") != "hand.conf" {
		t.Errorf("removed=%d foreign=%v, want 2 and [hand.conf] (registered files are neither)", removed, foreign)
	}
	if _, err := os.Stat(filepath.Join(sites, "bproj.hasura.conf")); err != nil {
		t.Fatalf("a registered file was pruned: %v", err)
	}

	// An unreadable registration refuses the prune instead of guessing.
	put(filepath.Join(shared, "broken.json"), "{")
	if _, _, err := pruneGeneratedNginxSitesVia(newDiskSink(""), sites); err == nil {
		t.Fatal("a registration that does not validate must refuse the prune")
	}
}
