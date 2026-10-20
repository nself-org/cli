package plugin

// Purpose: D-1415 regression. A CLI plugin whose registry entry advertises a platform signature for this platform must never install from the source-package fallback.
// Inputs:  a fixture registry whose platform asset 404s and whose source tarball is validly source-signed.
// Outputs: assertions that the install refuses with E121 and publishes nothing.
// Constraints: the registry repository and platform_signatures fields are unsigned, so a 404 on the platform asset must not downgrade the install to source-only verification.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
)

// sourceTarballWith builds a plugin tarball holding plugin.json and, when binary is non-empty, an executable of that name.
func sourceTarballWith(t *testing.T, binary string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	add := func(name string, mode int64, body []byte) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	add("plugin.json", 0o644, []byte(`{"name":"`+sigPlugin+`","version":"1.0.0"}`))
	if binary != "" {
		add(binary, 0o755, []byte("#!/bin/sh\n"))
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestInstallRefusesSourceFallbackForSignedBinaryPlugin: the platform asset
// 404s, the source tarball carries a binary and a valid source signature, and
// the entry advertises a platform signature. With or without a binary in the
// source package the install refuses (E121), publishes nothing, in both modes.
func TestInstallRefusesSourceFallbackForSignedBinaryPlugin(t *testing.T) {
	plat, err := PlatformArch()
	if err != nil {
		t.Skip("unsupported platform")
	}
	for _, withBinary := range []bool{true, false} {
		name := "source-without-binary"
		if withBinary {
			name = "source-with-binary"
		}
		t.Run(name, func(t *testing.T) {
			compattest.Both(t, func(t *testing.T) {
				binary := ""
				if withBinary {
					binary = "nself-" + sigPlugin
				}
				source := sourceTarballWith(t, binary)
				sum := sha256Hex(source)
				key, signer := signaturetest.NewKey(t, signature.TierFree)
				signaturetest.UseKeys(t, []signature.Key{key}, nil)
				signaturetest.UseFloor(t, map[string]map[string]string{})
				var sourceHits int32
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case strings.Contains(r.URL.Path, "/releases/download/"):
						http.NotFound(w, r)
					case strings.HasSuffix(r.URL.Path, "/tarball"):
						atomic.AddInt32(&sourceHits, 1)
						_, _ = w.Write(source)
					default:
						extra := relMap(signaturetest.Sign(t, signer, sigPlugin, "1.0.0", sum))
						extra["pluginType"], extra["binaryName"] = "cli", "nself-"+sigPlugin
						extra["repository"] = "http://" + r.Host
						extra["platform_signatures"] = map[string]any{plat: relMap(signaturetest.SignPlatform(t, signer, sigPlugin, "1.0.0", plat, sha256Hex([]byte("binary package"))))["release_signature"]}
						entry := map[string]any{"name": sigPlugin, "version": "1.0.0", "description": "fixture", "category": "util", "tier": "free", "license": "MIT", "status": "stable", "checksum": sum}
						for k, v := range extra {
							entry[k] = v
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"plugins": []any{entry}})
					}
				}))
				t.Cleanup(srv.Close)
				home := t.TempDir()
				t.Setenv("HOME", home)
				t.Setenv("USERPROFILE", home)
				t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
				t.Setenv("NSELF_SKIP_SBOM_CHECK", "1")
				t.Setenv(pluginRequireChecksumEnv, "")
				t.Setenv(allowUnsignedEnv, "")
				dir := t.TempDir()
				var ierr error
				_ = captureStderr(t, func() {
					ierr = installLocked(context.Background(), &config.Config{ProjectName: "t"}, sigPlugin, dir)
				})
				if codeOf(ierr) != "E121" {
					t.Fatalf("want E121, got %v", ierr)
				}
				if !dirEmpty(t, dir) {
					t.Fatal("the plugin dir must stay empty")
				}
				if _, err := os.Stat(PublishedBinaryPath("nself-" + sigPlugin)); err == nil {
					t.Fatal("a binary was published from the source package")
				}
			})
		})
	}
}
