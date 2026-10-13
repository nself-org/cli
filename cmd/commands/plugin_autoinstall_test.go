package commands

// Purpose: tests for autoInstallPlugin against a fixture registry served by httptest, a generated ed25519 key swapped into signature.Keys (restored by t.Cleanup) and a throwaway HOME. No network, no production key, no real catalogue data.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
)

const aiSlug, aiVersion = "ci", "1.0.0"

// aiTarball builds a plugin tarball holding plugin.json and an nself-ci file,
// plus any extra entries (name -> body).
func aiTarball(t *testing.T, extra map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	files := map[string]string{
		"plugin.json": `{"name":"ci","version":"` + aiVersion + `","pluginType":"cli","binaryName":"nself-ci"}`,
		"nself-ci":    "#!/bin/sh\necho fixture\n",
	}
	for k, v := range extra {
		files[k] = v
	}
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func aiSum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// aiServe serves a one-entry registry plus tarball and returns the tarball
// download counter. extra overrides or adds registry entry fields.
func aiServe(t *testing.T, tarball []byte, checksum string, extra map[string]any) *int32 {
	t.Helper()
	var srv *httptest.Server
	entry := map[string]any{
		"name": aiSlug, "version": aiVersion, "description": "fixture", "category": "util", "tier": "free",
		"license": "MIT", "status": "stable", "checksum": checksum, "pluginType": "cli", "binaryName": "nself-ci",
	}
	for k, v := range extra {
		entry[k] = v
	}
	var downloads int32
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".tar.gz") || strings.HasSuffix(r.URL.Path, "/tarball") {
			atomic.AddInt32(&downloads, 1)
			if strings.HasSuffix(r.URL.Path, "/tarball") {
				_, _ = w.Write(tarball)
				return
			}
			http.NotFound(w, r)
			return
		}
		entry["repository"] = srv.URL // a per-platform probe stays on the fixture server
		body, _ := json.Marshal(map[string]any{"plugins": []any{entry}})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	t.Setenv("NSELF_SKIP_SBOM_CHECK", "1")
	t.Setenv("NSELF_PLUGIN_REQUIRE_CHECKSUM", "")
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "")
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY_FORCE", "")
	return &downloads
}

func aiSigned(t *testing.T, sum string) map[string]any {
	t.Helper()
	key, signer := signaturetest.NewKey(t, signature.TierFree)
	signaturetest.UseKeys(t, []signature.Key{key}, nil)
	signaturetest.UseFloor(t, map[string]map[string]string{})
	r := signaturetest.Sign(t, signer, aiSlug, aiVersion, sum)
	return map[string]any{"release_signature": map[string]string{"key_id": r.KeyID, "alg": r.Alg, "sig": r.Sig}}
}

func aiCode(err error) string {
	var ce *errs.CLIError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func aiPublished() string { return plugin.PublishedBinaryPath("nself-ci") }

func TestAutoInstallRefusesUnsigned(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		tb := aiTarball(t, nil)
		key, _ := signaturetest.NewKey(t, signature.TierFree)
		signaturetest.UseKeys(t, []signature.Key{key}, nil)
		downloads := aiServe(t, tb, aiSum(tb), nil)
		_, err := autoInstallPlugin(aiSlug, "")
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E107" || !strings.Contains(ce.Fix, "nself add ci") {
			t.Fatalf("want E107 with a fix naming nself add ci, got %v", err)
		}
		if n := atomic.LoadInt32(downloads); n != 0 {
			t.Fatalf("%d downloads before the refusal", n)
		}
		if _, err := os.Stat(aiPublished()); err == nil {
			t.Fatal("a binary was published")
		}
	})
}

func TestAutoInstallRefusesUnpinnedTier(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		tb := aiTarball(t, nil)
		signaturetest.UseKeys(t, nil, nil)
		extra := map[string]any{"release_signature": map[string]string{"key_id": "k", "alg": signature.Alg, "sig": "AAAA"}}
		downloads := aiServe(t, tb, aiSum(tb), extra)
		_, err := autoInstallPlugin(aiSlug, "")
		if aiCode(err) != "E107" {
			t.Fatalf("want E107, got %v", err)
		}
		if n := atomic.LoadInt32(downloads); n != 0 {
			t.Fatalf("%d downloads before the refusal", n)
		}
	})
}

func TestAutoInstallSigned(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		tb := aiTarball(t, nil)
		sum := aiSum(tb)
		aiServe(t, tb, sum, aiSigned(t, sum))
		got, err := autoInstallPlugin(aiSlug, "")
		if err != nil {
			t.Fatalf("install: %v", err)
		}
		if got != aiPublished() {
			t.Fatalf("path %q, want %q", got, aiPublished())
		}
		if st, err := os.Stat(got); err != nil || st.IsDir() {
			t.Fatalf("published binary missing: %v", err)
		}
	})
}

func TestAutoInstallVersion(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		tb := aiTarball(t, nil)
		sum := aiSum(tb)
		downloads := aiServe(t, tb, sum, aiSigned(t, sum))
		_, err := autoInstallPlugin(aiSlug, "9.9.9")
		if err == nil || !strings.Contains(err.Error(), "nself add ci") || !strings.Contains(err.Error(), "9.9.9") {
			t.Fatalf("want a refusal naming the version and nself add ci, got %v", err)
		}
		if n := atomic.LoadInt32(downloads); n != 0 {
			t.Fatalf("%d downloads for a version the catalogue does not serve", n)
		}
		if _, err := autoInstallPlugin(aiSlug, "v"+aiVersion); err != nil {
			t.Fatalf("matching version refused: %v", err)
		}
	})
}

func TestAutoInstallTampered(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		tb := aiTarball(t, nil)
		want := aiSum([]byte("the published tarball"))
		tamperedSum := aiServe(t, tb, want, aiSigned(t, want))
		_, err := autoInstallPlugin(aiSlug, "")
		if aiCode(err) != "E124" || !strings.Contains(err.Error(), "nself add ci") {
			t.Fatalf("checksum mismatch: want E124 naming nself add ci, got %v", err)
		}
		if atomic.LoadInt32(tamperedSum) == 0 {
			t.Fatal("never reached the download: the refusal is not the checksum check")
		}
		if _, err := os.Stat(aiPublished()); err == nil {
			t.Fatal("a binary was published after a checksum mismatch")
		}
	})
}

func TestAutoInstallBadSignature(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		tb := aiTarball(t, nil)
		sum := aiSum(tb)
		aiServe(t, tb, sum, aiSigned(t, aiSum([]byte("other bytes")))) // signs different bytes
		_, err := autoInstallPlugin(aiSlug, "")
		if aiCode(err) != "E121" || !strings.Contains(err.Error(), "nself add ci") {
			t.Fatalf("want E121 naming nself add ci, got %v", err)
		}
		if _, err := os.Stat(aiPublished()); err == nil {
			t.Fatal("a binary was published after a bad signature")
		}
	})
}

func TestAutoInstallTarSlip(t *testing.T) {
	tb := aiTarball(t, map[string]string{"../escape.txt": "x"})
	sum := aiSum(tb)
	aiServe(t, tb, sum, aiSigned(t, sum)) // signed and checksummed, so only extraction can refuse it
	dir := os.Getenv("NSELF_PLUGIN_DIR")
	if _, err := autoInstallPlugin(aiSlug, ""); err == nil || !strings.Contains(err.Error(), "extracting plugin") {
		t.Fatalf("want the extraction guard to refuse a path-traversing tarball, got %v", err)
	}
	if _, err := os.Stat(dir + "/escape.txt"); err == nil {
		t.Fatal("tar-slip wrote outside the plugin directory")
	}
	if _, err := os.Stat(aiPublished()); err == nil {
		t.Fatal("a binary was published from a refused tarball")
	}
}

func TestAutoInstallRestoresEnv(t *testing.T) {
	tb := aiTarball(t, nil)
	sum := aiSum(tb)
	aiServe(t, tb, sum, aiSigned(t, sum))
	t.Setenv("NSELF_LICENSE_SKIP_VERIFY", "1")
	t.Setenv("NSELF_PLUGIN_REQUIRE_CHECKSUM", "")
	if _, ok := os.LookupEnv(autoInstallTierEnv); ok {
		t.Fatal("tier env set before the call")
	}
	if _, err := autoInstallPlugin(aiSlug, ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := os.LookupEnv(autoInstallTierEnv); ok {
		t.Fatal("tier env leaked after the call")
	}
	if os.Getenv("NSELF_LICENSE_SKIP_VERIFY") != "1" || os.Getenv("NSELF_PLUGIN_REQUIRE_CHECKSUM") != "" {
		t.Fatal("caller env not restored")
	}
}

func TestAutoInstallRefusesDependenciesAndLicensed(t *testing.T) {
	for name, extra := range map[string]map[string]any{
		"dependencies":     {"dependencies": []string{"other"}},
		"requires_license": {"requires_license": true},
		"pro tier":         {"tier": "pro"},
	} {
		t.Run(name, func(t *testing.T) {
			compattest.Both(t, func(t *testing.T) {
				tb := aiTarball(t, nil)
				sum := aiSum(tb)
				signed := aiSigned(t, sum)
				for k, v := range extra {
					signed[k] = v
				}
				downloads := aiServe(t, tb, sum, signed)
				_, err := autoInstallPlugin(aiSlug, "")
				if err == nil || !strings.Contains(err.Error(), "nself add ci") {
					t.Fatalf("want a refusal naming nself add ci, got %v", err)
				}
				if n := atomic.LoadInt32(downloads); n != 0 {
					t.Fatalf("%d downloads before the refusal", n)
				}
			})
		})
	}
}

// A leftover plugin directory must not leak an unverified nself-ci into the
// published binary: the signed archive has no nself-ci, the directory does.
func TestAutoInstallIgnoresStaleDirectory(t *testing.T) {
	tb := aiTarball(t, nil)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := `{"name":"ci","version":"` + aiVersion + `","pluginType":"cli","binaryName":"nself-ci"}`
	_ = tw.WriteHeader(&tar.Header{Name: "plugin.json", Mode: 0o644, Size: int64(len(body))})
	_, _ = tw.Write([]byte(body))
	_ = tw.Close()
	_ = gz.Close()
	tb = buf.Bytes()
	sum := aiSum(tb)
	aiServe(t, tb, sum, aiSigned(t, sum))
	dir := os.Getenv("NSELF_PLUGIN_DIR") + "/ci"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/nself-ci", []byte("#!/bin/sh\necho stale\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := autoInstallPlugin(aiSlug, ""); err == nil {
		t.Fatal("a signed archive without nself-ci installed through a stale binary")
	}
	if _, err := os.Stat(aiPublished()); err == nil {
		t.Fatal("the stale binary was published")
	}
	if b, err := os.ReadFile(dir + "/nself-ci"); err != nil || !strings.Contains(string(b), "stale") {
		t.Fatal("the leftover directory was not put back after the refused install")
	}
}

// An archive whose plugin.json carries no readable version cannot be matched to
// the checked catalogue entry: refuse it and publish nothing.
func TestAutoInstallRefusesUnreadableInstalledVersion(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{
		"plugin.json": `{"name":"ci","pluginType":"cli","binaryName":"nself-ci"}`,
		"nself-ci":    "#!/bin/sh\necho x\n",
	} {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body))})
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	_ = gz.Close()
	tb := buf.Bytes()
	sum := aiSum(tb)
	aiServe(t, tb, sum, aiSigned(t, sum))
	if _, err := autoInstallPlugin(aiSlug, ""); err == nil || !strings.Contains(err.Error(), "catalogue changed") {
		t.Fatalf("want a refusal for an unreadable installed version, got %v", err)
	}
	if _, err := os.Stat(aiPublished()); err == nil {
		t.Fatal("a binary stayed published after the refusal")
	}
}
