package commands

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
	"github.com/spf13/cobra"
)

func TestPluginInstallPassesNewDependenciesToReadiness(t *testing.T) {
	compattest.Set(t, true)
	cmd := &cobra.Command{Use: "install"}
	cmd.Flags().AddFlagSet(pluginInstallCmd.Flags())
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	p := extensionFixture(t, "")
	key, signer := signaturetest.NewKey(t, signature.TierFree)
	signaturetest.UseKeys(t, []signature.Key{key}, nil)
	signaturetest.UseFloor(t, map[string]map[string]string{})
	archives := map[string][]byte{}
	var entries []map[string]any
	for _, name := range []string{"zzroot", "zzdep"} {
		manifest := map[string]any{"name": name, "version": "1.0.0", "description": "fixture", "category": "utility", "tier": "free", "license": "MIT"}
		if name == "zzroot" {
			manifest["dependencies"] = []string{"zzdep"}
		}
		body, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		var archive bytes.Buffer
		gz := gzip.NewWriter(&archive)
		tw := tar.NewWriter(gz)
		if err := tw.WriteHeader(&tar.Header{Name: "plugin.json", Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
		archives[name] = archive.Bytes()
		sum := sha256.Sum256(archive.Bytes())
		checksum := hex.EncodeToString(sum[:])
		manifest["checksum"] = checksum
		rel := signaturetest.Sign(t, signer, name, "1.0.0", checksum)
		manifest["release_signature"] = map[string]any{"key_id": rel.KeyID, "alg": rel.Alg, "sig": rel.Sig}
		entries = append(entries, manifest)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tarball") {
			for name, archive := range archives {
				if strings.Contains(r.URL.Path, "/"+name+"/") {
					_, _ = w.Write(archive)
					return
				}
			}
			t.Errorf("unknown archive URL: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"plugins": entries})
	}))
	defer srv.Close()
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "fixture-key")
	t.Setenv("NSELF_SKIP_SBOM_CHECK", "1")
	oldTrack := pluginTrack
	var got []string
	pluginTrack = func(_ *cobra.Command, _ string, names []string) error {
		got = append([]string(nil), names...)
		return nil
	}
	t.Cleanup(func() { pluginTrack = oldTrack })
	if err := runPluginInstall(cmd, []string{"zzroot"}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"zzroot", "zzdep"}) {
		t.Fatalf("readiness/tracking got %v, want explicit plugin and fresh dependency", got)
	}
	if _, err := os.Stat(filepath.Join(p.plugins, "zzdep", "plugin.json")); err != nil {
		t.Fatal(err)
	}
}

func TestInstallStageReadinessIgnoresNewBinaryDirectory(t *testing.T) {
	dir := t.TempDir()
	stageWritePlugin(t, dir, "existing")
	stage := beginInstallStage(dir)
	if err := stage.install(context.Background(), func() error {
		stageWritePlugin(t, dir, "dep")
		return os.Mkdir(filepath.Join(dir, "bin"), 0o755)
	}); err != nil {
		t.Fatal(err)
	}
	if got := stage.readinessNames([]string{"root"}); !reflect.DeepEqual(got, []string{"root", "dep"}) {
		t.Fatalf("readiness names = %v", got)
	}
}
