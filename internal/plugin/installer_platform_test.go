package plugin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
	"github.com/nself-org/cli/sdk/go/v2/signing"
)

// binTarball builds a platform package: plugin.json plus the command binary.
func binTarball(t *testing.T, extra string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	files := []struct{ name, body string }{
		{"plugin.json", `{"name":"` + sigPlugin + `","version":"1.0.0"}`},
		{"nself-" + sigPlugin, "#!/bin/sh\n" + extra},
	}
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o755, Size: int64(len(f.body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f.body)); err != nil {
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

// platRegistry is a registry that offers sigPlugin as a command-providing
// plugin with a platform package. platformSigs is the platform_signatures value.
type platRegistry struct {
	plat                   string
	source, binary         []byte
	sourceHits, binaryHits int32
	sourceSig              any
	platformSigs           any
}

func (p *platRegistry) serve(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/releases/download/"):
			atomic.AddInt32(&p.binaryHits, 1)
			_, _ = w.Write(p.binary)
		case strings.HasSuffix(r.URL.Path, "/tarball"):
			atomic.AddInt32(&p.sourceHits, 1)
			_, _ = w.Write(p.source)
		default:
			entry := map[string]any{
				"name": sigPlugin, "version": "1.0.0", "description": "fixture", "category": "util", "tier": "free",
				"license": "MIT", "status": "stable", "checksum": sha256Hex(p.source),
				"pluginType": "cli", "binaryName": "nself-" + sigPlugin, "repository": "http://" + r.Host,
				"checksums": map[string]any{"platforms": map[string]string{p.plat: sha256Hex(p.binary)}},
			}
			if p.sourceSig != nil {
				entry["release_signature"] = p.sourceSig
			}
			if p.platformSigs != nil {
				entry["platform_signatures"] = p.platformSigs
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
}

func sigObj(r *signature.Release) map[string]string {
	return map[string]string{"key_id": r.KeyID, "alg": r.Alg, "sig": r.Sig}
}

// otherPlatform returns a platform name that is not plat.
func otherPlatform(plat string) string {
	if plat == "darwin-arm64" {
		return "linux-amd64"
	}
	return "darwin-arm64"
}

// platCase pins a free key, builds the packages and signs the platform
// package; mutate may change what the registry serves before it starts.
func platCase(t *testing.T, mutate func(p *platRegistry, signer signing.Signer, plat string)) (*platRegistry, string, error) {
	t.Helper()
	plat, err := PlatformArch()
	if err != nil {
		t.Skip("unsupported platform")
	}
	key, signer := signaturetest.NewKey(t, signature.TierFree)
	signaturetest.UseKeys(t, []signature.Key{key}, nil)
	signaturetest.UseFloor(t, map[string]map[string]string{})
	p := &platRegistry{plat: plat, source: sigTarball(t), binary: binTarball(t, "")}
	p.sourceSig = sigObj(signaturetest.Sign(t, signer, sigPlugin, "1.0.0", sha256Hex(p.source)))
	p.platformSigs = map[string]any{plat: sigObj(signaturetest.SignPlatform(t, signer, sigPlugin, "1.0.0", plat, sha256Hex(p.binary)))}
	if mutate != nil {
		mutate(p, signer, plat)
	}
	p.serve(t)
	dir, err, _ := installSig(t)
	return p, dir, err
}

func TestInstallBinaryPackageSignedInstalls(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		p, dir, err := platCase(t, nil)
		if err != nil {
			t.Fatalf("a verified platform package must install: %v", err)
		}
		if atomic.LoadInt32(&p.binaryHits) == 0 || atomic.LoadInt32(&p.sourceHits) != 0 {
			t.Fatalf("expected the platform package only: binary=%d source=%d", p.binaryHits, p.sourceHits)
		}
		if _, err := os.Stat(filepath.Join(dir, sigPlugin, "plugin.json")); err != nil {
			t.Fatalf("the package was not extracted: %v", err)
		}
	})
}

// TestInstallBinaryPackageUnsignedE121: a source signature alone, a missing
// entry and an empty map all refuse before extraction.
func TestInstallBinaryPackageUnsignedE121(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		cases := map[string]func(*platRegistry, signing.Signer, string){
			"no platform_signatures": func(p *platRegistry, _ signing.Signer, _ string) { p.platformSigs = nil },
			"empty map":              func(p *platRegistry, _ signing.Signer, _ string) { p.platformSigs = map[string]any{} },
			"no release_signature either": func(p *platRegistry, _ signing.Signer, _ string) {
				p.platformSigs, p.sourceSig = nil, nil
			},
		}
		for name, mutate := range cases {
			t.Run(name, func(t *testing.T) {
				_, dir, err := platCase(t, mutate)
				if codeOf(err) != "E121" || !dirEmpty(t, dir) {
					t.Fatalf("want E121 and an empty plugin dir, got %v", err)
				}
			})
		}
	})
}

// TestInstallBinaryPackageOtherPlatformE121 also covers every other way a
// platform signature can be the wrong one: version, slug, key, tampered bytes.
func TestInstallBinaryPackageOtherPlatformE121(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		cases := map[string]struct {
			mutate func(*platRegistry, signing.Signer, string)
			code   string
		}{
			"signature made for another platform": {func(p *platRegistry, s signing.Signer, plat string) {
				p.platformSigs = map[string]any{plat: sigObj(signaturetest.SignPlatform(t, s, sigPlugin, "1.0.0", otherPlatform(plat), sha256Hex(p.binary)))}
			}, "E121"},
			"only another platform listed": {func(p *platRegistry, s signing.Signer, plat string) {
				other := otherPlatform(plat)
				p.platformSigs = map[string]any{other: sigObj(signaturetest.SignPlatform(t, s, sigPlugin, "1.0.0", other, sha256Hex(p.binary)))}
			}, "E121"},
			"another version": {func(p *platRegistry, s signing.Signer, plat string) {
				p.platformSigs = map[string]any{plat: sigObj(signaturetest.SignPlatform(t, s, sigPlugin, "1.0.1", plat, sha256Hex(p.binary)))}
			}, "E121"},
			"another slug": {func(p *platRegistry, s signing.Signer, plat string) {
				p.platformSigs = map[string]any{plat: sigObj(signaturetest.SignPlatform(t, s, "other", "1.0.0", plat, sha256Hex(p.binary)))}
			}, "E121"},
			"source signature used as platform signature": {func(p *platRegistry, _ signing.Signer, plat string) {
				p.platformSigs = map[string]any{plat: p.sourceSig}
			}, "E121"},
			"tampered bytes (signature over other bytes)": {func(p *platRegistry, s signing.Signer, plat string) {
				p.platformSigs = map[string]any{plat: sigObj(signaturetest.SignPlatform(t, s, sigPlugin, "1.0.0", plat, sha256Hex(binTarball(t, "# original"))))}
			}, "E121"},
			"signed by a stranger": {func(p *platRegistry, _ signing.Signer, plat string) {
				_, stranger := signaturetest.NewKey(t, signature.TierFree)
				p.platformSigs = map[string]any{plat: sigObj(signaturetest.SignPlatform(t, stranger, sigPlugin, "1.0.0", plat, sha256Hex(p.binary)))}
			}, "E122"},
		}
		for name, c := range cases {
			t.Run(name, func(t *testing.T) {
				_, dir, err := platCase(t, c.mutate)
				if codeOf(err) != c.code || !dirEmpty(t, dir) {
					t.Fatalf("want %s and an empty plugin dir, got %v", c.code, err)
				}
			})
		}
	})
}

// TestInstallBinaryPackageNoSourceFallback: a platform package that fails
// verification is never replaced by the (validly signed) source package.
func TestInstallBinaryPackageNoSourceFallback(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		p, dir, err := platCase(t, func(p *platRegistry, _ signing.Signer, _ string) { p.platformSigs = nil })
		if codeOf(err) != "E121" || !dirEmpty(t, dir) {
			t.Fatalf("want E121 and an empty plugin dir, got %v", err)
		}
		if atomic.LoadInt32(&p.binaryHits) == 0 {
			t.Fatal("the platform package was never requested: the test does not exercise the binary branch")
		}
		if n := atomic.LoadInt32(&p.sourceHits); n != 0 {
			t.Fatalf("the source package was downloaded %d time(s) after the platform package failed", n)
		}
	})
}

// TestInstallBinaryPackageBypassNeverSkips: the dev bypass skips an ABSENT
// platform signature, never a present one, and a registry that strips
// release_signature does not change that.
func TestInstallBinaryPackageBypassNeverSkips(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		key, signer := signaturetest.NewKey(t, signature.TierFree)
		signaturetest.UseKeys(t, []signature.Key{key}, nil)
		signaturetest.UseFloor(t, map[string]map[string]string{})
		body := binTarball(t, "")
		archive := filepath.Join(t.TempDir(), "pkg.tar.gz")
		if err := os.WriteFile(archive, body, 0o600); err != nil {
			t.Fatal(err)
		}
		const plat = "linux-amd64"
		m := &PluginManifest{Name: sigPlugin, Version: "1.0.0", Checksum: sha256Hex([]byte("source"))}
		if err := verifyReleaseSignature(m, false, false, plat, sha256Hex(body), true); err != nil {
			t.Fatalf("bypass with no platform signature must skip: %v", err)
		}
		_, stranger := signaturetest.NewKey(t, signature.TierFree)
		m.PlatformSignatures = map[string]*signature.Release{plat: signaturetest.SignPlatform(t, stranger, sigPlugin, "1.0.0", plat, sha256Hex(body))}
		if codeOf(verifyReleaseSignature(m, false, false, plat, sha256Hex(body), true)) != "E122" {
			t.Fatal("bypass must not skip an unknown-key platform signature (release_signature stripped)")
		}
		m.PlatformSignatures[plat] = signaturetest.SignPlatform(t, signer, sigPlugin, "1.0.0", plat, sha256Hex([]byte("other")))
		if codeOf(verifyReleaseSignature(m, false, false, plat, sha256Hex(body), true)) != "E121" {
			t.Fatal("bypass must not skip a bad platform signature")
		}
		good := signaturetest.SignPlatform(t, signer, sigPlugin, "1.0.0", plat, sha256Hex(body))
		m.PlatformSignatures[plat] = good
		if err := verifyReleaseSignature(m, false, false, plat, sha256Hex(body), true); err != nil {
			t.Fatalf("good platform signature under bypass: %v", err)
		}
		// An entry for another platform is not "present" for this one.
		m.PlatformSignatures = map[string]*signature.Release{"darwin-arm64": good}
		if err := verifyReleaseSignature(m, false, false, plat, sha256Hex(body), true); err != nil {
			t.Fatalf("an entry for another platform must not count under bypass: %v", err)
		}
		if codeOf(verifyReleaseSignature(m, false, false, plat, sha256Hex(body), false)) != "E121" {
			t.Fatal("without the bypass an absent platform entry is E121")
		}
		// An unreadable archive is E121, never a skip, once an entry is present.
		m.PlatformSignatures = map[string]*signature.Release{plat: good}
		if codeOf(verifyReleaseSignature(m, false, false, plat, "", true)) != "E121" {
			t.Fatal("an archive that cannot be read must refuse")
		}
		// The licensed tier has no platform producer: pinned means refused.
		lkey, _ := signaturetest.NewKey(t, signature.TierLicensed)
		signaturetest.UseKeys(t, []signature.Key{key, lkey}, nil)
		m.PlatformSignatures = nil
		if codeOf(verifyReleaseSignature(m, true, false, plat, sha256Hex(body), false)) != "E121" {
			t.Fatal("a pinned licensed tier must refuse an unsigned platform package")
		}
		// An unpinned tier is untouched by v1.1.
		signaturetest.UseKeys(t, nil, nil)
		if err := verifyReleaseSignature(m, false, false, plat, sha256Hex(body), true); err != nil {
			t.Fatalf("unpinned tier under bypass: %v", err)
		}
	})
}

// TestPlatformSignatureMalformedE121: a malformed platform_signatures value is
// present-but-invalid for the platform that is looked up, never "absent".
func TestPlatformSignatureMalformedE121(t *testing.T) {
	const plat = "linux-amd64"
	key, _ := signaturetest.NewKey(t, signature.TierFree)
	signaturetest.UseKeys(t, []signature.Key{key}, nil)
	signaturetest.UseFloor(t, map[string]map[string]string{})
	archiveSum := sha256Hex(binTarball(t, ""))
	cases := map[string]string{
		"string":          `"sig"`,
		"array":           `[]`,
		"number":          `7`,
		"entry is string": `{"linux-amd64":"c2ln"}`,
		"entry is null":   `{"linux-amd64":null}`,
		"entry is empty":  `{"linux-amd64":{}}`,
		"entry bad field": `{"linux-amd64":{"key_id":1}}`,
		"entry is array":  `{"linux-amd64":[]}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			reg, err := parseRegistryJSON([]byte(`{"plugins":[{"name":"` + sigPlugin + `","version":"1.0.0","platform_signatures":` + raw + `}]}`))
			if err != nil {
				t.Fatal(err)
			}
			m := &reg.Plugins[0]
			if _, present := m.PlatformSignatures[plat]; !present {
				t.Fatalf("a malformed value must stay present for %s: %+v", plat, m.PlatformSignatures)
			}
			for _, bypass := range []bool{false, true} {
				if codeOf(verifyReleaseSignature(m, false, false, plat, archiveSum, bypass)) != "E121" {
					t.Fatalf("bypass=%v: want E121", bypass)
				}
			}
		})
	}
}

// TestPlatformSignaturesRegistryParse: absent and null are nil, a good value
// decodes per platform, names outside the five are kept but never selected, and
// an unreadable value is invalid for all five platforms.
func TestPlatformSignaturesRegistryParse(t *testing.T) {
	const rel = `{"key_id":"plugins-0123456789abcdef","alg":"ed25519","sig":"c2ln"}`
	body := `{"plugins":[
	  {"name":"a","version":"1.0.0","platform_signatures":{"linux-amd64":` + rel + `,"darwin-arm64":` + rel + `,"freebsd-amd64":"junk","Linux-amd64":` + rel + `}},
	  {"name":"b","version":"1.0.0"},
	  {"name":"c","version":"1.0.0","platform_signatures":null},
	  {"name":"d","version":"1.0.0","platform_signatures":{}},
	  {"name":"e","version":"1.0.0","platform_signatures":{"freebsd-amd64":` + rel + `}},
	  {"name":"f","version":"1.0.0","platform_signatures":"mangled"}]}`
	reg, err := parseRegistryJSON([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]map[string]*signature.Release{}
	for i := range reg.Plugins {
		got[reg.Plugins[i].Name] = reg.Plugins[i].PlatformSignatures
	}
	want := signature.Release{KeyID: "plugins-0123456789abcdef", Alg: "ed25519", Sig: "c2ln"}
	if len(got["a"]) != 4 || got["a"]["linux-amd64"] == nil || *got["a"]["linux-amd64"] != want || got["a"]["darwin-arm64"] == nil || *got["a"]["darwin-arm64"] != want {
		t.Errorf("a: %+v", got["a"])
	}
	for _, n := range []string{"b", "c", "d"} {
		if got[n] != nil {
			t.Errorf("%s must have no platform signatures: %+v", n, got[n])
		}
	}
	if len(got["e"]) != 1 || got["e"]["freebsd-amd64"] == nil {
		t.Errorf("e: %+v", got["e"])
	}
	if _, ok := got["a"]["Linux-amd64"]; !ok {
		t.Error("a name outside the five is kept (and never selected)")
	}
	if len(got["f"]) != len(signature.Platforms) {
		t.Errorf("an unreadable value must be invalid for all five platforms: %+v", got["f"])
	}
	for p, r := range got["f"] {
		if r == nil || *r != (signature.Release{}) {
			t.Errorf("f/%s must be a non-nil empty Release: %+v", p, r)
		}
	}
}
