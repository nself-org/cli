package plugin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
)

const sigPlugin = "sigplug"

// sigTarball builds a minimal valid plugin tarball.
func sigTarball(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte(`{"name":"` + sigPlugin + `","version":"1.0.0"}`)
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
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// sigServer serves a one-plugin registry whose entry carries the given extra
// fields, plus the tarball, and counts tarball downloads.
func sigServer(t *testing.T, tarball []byte, checksum string, extra map[string]any) *int32 {
	t.Helper()
	entry := map[string]any{
		"name": sigPlugin, "version": "1.0.0", "description": "fixture", "category": "util",
		"tier": "free", "license": "MIT", "status": "stable", "checksum": checksum,
	}
	for k, v := range extra {
		entry[k] = v
	}
	reg, err := json.Marshal(map[string]any{"plugins": []any{entry}})
	if err != nil {
		t.Fatal(err)
	}
	var downloads int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tarball") {
			atomic.AddInt32(&downloads, 1)
			_, _ = w.Write(tarball)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(reg)
	}))
	t.Cleanup(srv.Close)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	t.Setenv("NSELF_SKIP_SBOM_CHECK", "1")
	t.Setenv(pluginRequireChecksumEnv, "")
	t.Setenv(allowUnsignedEnv, "")
	return &downloads
}

// captureStderr runs fn and returns what it wrote to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	done := make(chan string)
	go func() { b, _ := io.ReadAll(r); done <- string(b) }()
	func() {
		defer func() { os.Stderr = old; _ = w.Close() }()
		fn()
	}()
	return <-done
}

func installSig(t *testing.T) (pluginDir string, err error, stderr string) {
	t.Helper()
	pluginDir = t.TempDir()
	stderr = captureStderr(t, func() {
		err = installLocked(context.Background(), &config.Config{ProjectName: "t"}, sigPlugin, pluginDir)
	})
	return
}

func codeOf(err error) string {
	var ce *errs.CLIError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func dirEmpty(t *testing.T, dir string) bool {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(ents) == 0
}

func relMap(r *signature.Release) map[string]any {
	return map[string]any{"release_signature": map[string]string{"key_id": r.KeyID, "alg": r.Alg, "sig": r.Sig}}
}

// TestInstallChecksumMismatchE124NoWrite: a tampered tarball (checksum
// mismatch) refuses with E124 before anything is extracted, in both modes.
func TestInstallChecksumMismatchE124NoWrite(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		tb := sigTarball(t)
		key, signer := signaturetest.NewKey(t, signature.TierFree)
		signaturetest.UseKeys(t, []signature.Key{key}, nil)
		signaturetest.UseFloor(t, map[string]map[string]string{})
		// The registry (and its signature) describe other bytes than the server sends.
		want := sha256Hex([]byte("the published tarball"))
		downloads := sigServer(t, tb, want, relMap(signaturetest.Sign(t, signer, sigPlugin, "1.0.0", want)))
		dir, err, _ := installSig(t)
		if codeOf(err) != "E124" {
			t.Fatalf("want E124, got %v", err)
		}
		if atomic.LoadInt32(downloads) == 0 {
			t.Fatal("never reached the download: the refusal is not the checksum check")
		}
		if !dirEmpty(t, dir) {
			t.Fatal("a file was written before the checksum passed")
		}
	})
}

// TestInstallSignatureFailuresNoWrite: E121/E122/E123 after a good checksum,
// each in both modes, each before any file is written.
func TestInstallSignatureFailuresNoWrite(t *testing.T) {
	tb := sigTarball(t)
	sum := sha256Hex(tb)
	type scenario struct {
		name, want string
		setup      func(t *testing.T) map[string]any
	}
	scenarios := []scenario{
		{"bad signature", "E121", func(t *testing.T) map[string]any {
			key, signer := signaturetest.NewKey(t, signature.TierFree)
			signaturetest.UseKeys(t, []signature.Key{key}, nil)
			return relMap(signaturetest.Sign(t, signer, sigPlugin, "1.0.0", sha256Hex([]byte("other bytes"))))
		}},
		{"unknown key", "E122", func(t *testing.T) map[string]any {
			key, _ := signaturetest.NewKey(t, signature.TierFree)
			_, stranger := signaturetest.NewKey(t, signature.TierFree)
			signaturetest.UseKeys(t, []signature.Key{key}, nil)
			return relMap(signaturetest.Sign(t, stranger, sigPlugin, "1.0.0", sum))
		}},
		{"licensed-tier key on a free entry", "E122", func(t *testing.T) map[string]any {
			free, _ := signaturetest.NewKey(t, signature.TierFree)
			lic, licSigner := signaturetest.NewKey(t, signature.TierLicensed)
			signaturetest.UseKeys(t, []signature.Key{free, lic}, nil)
			return relMap(signaturetest.Sign(t, licSigner, sigPlugin, "1.0.0", sum))
		}},
		{"revoked key", "E123", func(t *testing.T) map[string]any {
			key, signer := signaturetest.NewKey(t, signature.TierFree)
			signaturetest.UseKeys(t, []signature.Key{key}, []string{key.ID})
			return relMap(signaturetest.Sign(t, signer, sigPlugin, "1.0.0", sum))
		}},
		{"malformed release_signature", "E121", func(t *testing.T) map[string]any {
			key, _ := signaturetest.NewKey(t, signature.TierFree)
			signaturetest.UseKeys(t, []signature.Key{key}, nil)
			return map[string]any{"release_signature": "not-an-object"}
		}},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			compattest.Both(t, func(t *testing.T) {
				signaturetest.UseFloor(t, map[string]map[string]string{})
				extra := sc.setup(t)
				downloads := sigServer(t, tb, sum, extra)
				dir, err, _ := installSig(t)
				if codeOf(err) != sc.want {
					t.Fatalf("want %s, got %v", sc.want, err)
				}
				if atomic.LoadInt32(downloads) == 0 {
					t.Fatal("never reached the download")
				}
				if !dirEmpty(t, dir) {
					t.Fatal("a file was written before the signature passed")
				}
			})
		})
	}
}

// TestInstallValidSignatureInstalls: a good signature installs quietly in both modes.
func TestInstallValidSignatureInstalls(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		tb := sigTarball(t)
		sum := sha256Hex(tb)
		key, signer := signaturetest.NewKey(t, signature.TierFree)
		signaturetest.UseKeys(t, []signature.Key{key}, nil)
		signaturetest.UseFloor(t, map[string]map[string]string{signature.TierFree: {sigPlugin: "1.0.0"}})
		sigServer(t, tb, sum, relMap(signaturetest.Sign(t, signer, sigPlugin, "1.0.0", sum)))
		dir, err, stderr := installSig(t)
		if err != nil {
			t.Fatalf("install: %v", err)
		}
		if strings.Contains(stderr, "signature") {
			t.Errorf("a verified install must not warn about signatures: %q", stderr)
		}
		if _, statErr := os.Stat(dir + "/" + sigPlugin + "/plugin.json"); statErr != nil {
			t.Errorf("plugin not extracted: %v", statErr)
		}
	})
}

// TestInstallMissingSignatureV14Warns: v1.4 mode installs an unsigned plugin
// with exactly one warning.
func TestInstallMissingSignatureV14Warns(t *testing.T) {
	compattest.Set(t, false)
	tb := sigTarball(t)
	signaturetest.UseKeys(t, nil, nil)
	signaturetest.UseFloor(t, map[string]map[string]string{})
	sigServer(t, tb, sha256Hex(tb), nil)
	dir, err, stderr := installSig(t)
	if err != nil {
		t.Fatalf("v1.4 unsigned install must proceed: %v", err)
	}
	if n := strings.Count(stderr, "no release signature"); n != 1 {
		t.Fatalf("want exactly one signature warning, got %d in %q", n, stderr)
	}
	if _, statErr := os.Stat(dir + "/" + sigPlugin + "/plugin.json"); statErr != nil {
		t.Errorf("plugin not extracted: %v", statErr)
	}
}

// TestInstallMissingSignatureV15RefusesE107: v1.5 mode refuses before extraction.
func TestInstallMissingSignatureV15RefusesE107(t *testing.T) {
	compattest.Set(t, true)
	tb := sigTarball(t)
	signaturetest.UseKeys(t, nil, nil)
	signaturetest.UseFloor(t, map[string]map[string]string{})
	sigServer(t, tb, sha256Hex(tb), nil)
	dir, err, _ := installSig(t)
	if codeOf(err) != "E107" {
		t.Fatalf("want E107, got %v", err)
	}
	if !dirEmpty(t, dir) {
		t.Fatal("a file was written for a refused install")
	}
}

// TestInstallAuthorKeyDoesNotCountAsSigned: a manifest whose own author key
// signs its own tarball (the retired self-certifying path) is still unsigned.
func TestInstallAuthorKeyDoesNotCountAsSigned(t *testing.T) {
	tb := sigTarball(t)
	sum := sha256Hex(tb)
	signaturetest.UseKeys(t, nil, nil)
	signaturetest.UseFloor(t, map[string]map[string]string{})
	pub, priv := genAuthorKey(t)
	digest := sha256.Sum256(tb)
	extra := map[string]any{"author_public_key": hex.EncodeToString(pub), "signature": signHex(priv, digest[:])}
	sigServer(t, tb, sum, extra)
	t.Run("v1.4 warns", func(t *testing.T) {
		compattest.Set(t, false)
		_, err, stderr := installSig(t)
		if err != nil || !strings.Contains(stderr, "no release signature") {
			t.Fatalf("err=%v stderr=%q", err, stderr)
		}
	})
	t.Run("v1.5 refuses", func(t *testing.T) {
		compattest.Set(t, true)
		dir, err, _ := installSig(t)
		if codeOf(err) != "E107" || !dirEmpty(t, dir) {
			t.Fatalf("err=%v", err)
		}
	})
}

// TestInstallAllowUnsignedLocalOnly: --allow-unsigned (threaded as
// NSELF_PLUGIN_ALLOW_UNSIGNED) lets a local path or link install through,
// never a registry install, free or licensed.
func TestInstallAllowUnsignedLocalOnly(t *testing.T) {
	compattest.Set(t, true)
	tb := sigTarball(t)
	signaturetest.UseKeys(t, nil, nil)
	signaturetest.UseFloor(t, map[string]map[string]string{})
	sigServer(t, tb, sha256Hex(tb), nil)
	t.Setenv(allowUnsignedEnv, "1")

	dir, err, _ := installSig(t)
	if codeOf(err) != "E107" || !dirEmpty(t, dir) {
		t.Fatalf("registry install with the flag must still refuse: %v", err)
	}
	m := &PluginManifest{Name: sigPlugin, Version: "1.0.0", Checksum: sha256Hex(tb)}
	for _, paid := range []bool{false, true} {
		if err := verifyReleaseSignature(m, paid, false); codeOf(err) != "E107" {
			t.Errorf("registry (paid=%v) with the flag: want E107, got %v", paid, err)
		}
		var werr error
		out := captureStderr(t, func() { werr = verifyReleaseSignature(m, paid, true) })
		if werr != nil || !strings.Contains(out, "--allow-unsigned") {
			t.Errorf("local (paid=%v) with the flag: err=%v stderr=%q", paid, werr, out)
		}
	}
	t.Setenv(allowUnsignedEnv, "")
	if err := verifyReleaseSignature(m, false, true); codeOf(err) != "E107" {
		t.Errorf("local without the flag: want E107, got %v", err)
	}
	if !compat.V15() {
		t.Fatal("test must run in v1.5 mode")
	}
}

// TestReleaseSignatureTierRouting: the manifest's paid flag selects the tier:
// a licensed entry verifies with the licensed key only.
func TestReleaseSignatureTierRouting(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		free, freeSigner := signaturetest.NewKey(t, signature.TierFree)
		lic, licSigner := signaturetest.NewKey(t, signature.TierLicensed)
		signaturetest.UseKeys(t, []signature.Key{free, lic}, nil)
		signaturetest.UseFloor(t, map[string]map[string]string{})
		sum := sha256Hex([]byte("x"))
		m := &PluginManifest{Name: sigPlugin, Version: "1.0.0", Checksum: strings.ToUpper(sum)}
		m.ReleaseSignature = signaturetest.Sign(t, licSigner, sigPlugin, "1.0.0", sum)
		if err := verifyReleaseSignature(m, true, false); err != nil {
			t.Fatalf("licensed entry, licensed key (checksum upper-cased in the registry): %v", err)
		}
		if codeOf(verifyReleaseSignature(m, false, false)) != "E122" {
			t.Error("licensed-key signature on a free entry must be E122")
		}
		m.ReleaseSignature = signaturetest.Sign(t, freeSigner, sigPlugin, "1.0.0", sum)
		if codeOf(verifyReleaseSignature(m, true, false)) != "E122" {
			t.Error("free-key signature on a licensed entry must be E122")
		}
		m.Checksum = ""
		if codeOf(verifyReleaseSignature(m, false, false)) != "E121" {
			t.Error("a signature with no checksum to bind it to must be E121")
		}
	})
}

// genAuthorKey and signHex build the retired self-certifying author-key pair.
func genAuthorKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func signHex(priv ed25519.PrivateKey, msg []byte) string {
	return hex.EncodeToString(ed25519.Sign(priv, msg))
}
