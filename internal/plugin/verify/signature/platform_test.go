package signature_test

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
	"github.com/nself-org/cli/sdk/go/v2/signing"
)

const platform = "linux-amd64"

func TestPlatformSigningInput(t *testing.T) {
	got := string(signature.SigningInputPlatform("ci", "1.2.7", "linux-amd64", "ab"))
	if want := "nself-plugin-bin-v1\nci\n1.2.7\nlinux-amd64\nab\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if string(signature.SigningInput("ci", "1.2.7", "ab")) == got {
		t.Fatal("the source and platform inputs must differ")
	}
}

func TestPlatformsAreTheFiveNames(t *testing.T) {
	want := []string{"darwin-amd64", "darwin-arm64", "linux-amd64", "linux-arm64", "windows-amd64"}
	if strings.Join(signature.Platforms, ",") != strings.Join(want, ",") {
		t.Fatalf("Platforms = %v", signature.Platforms)
	}
	for _, p := range want {
		if !signature.ValidPlatform(p) {
			t.Errorf("%s must be valid", p)
		}
	}
	for _, p := range []string{"", "linux", "Linux-amd64", "linux-amd64 ", "freebsd-amd64", "source", "linux-386"} {
		if signature.ValidPlatform(p) {
			t.Errorf("%q must not be valid", p)
		}
	}
}

func TestVerifyPlatformOK(t *testing.T) {
	f := newFixture(t)
	sha := digest("binary tarball")
	for _, p := range signature.Platforms {
		rel := signaturetest.SignPlatform(t, f.freeSign, slug, version, p, sha)
		if err := signature.VerifyPlatform(signature.TierFree, slug, version, p, sha, rel); err != nil {
			t.Fatalf("free %s: %v", p, err)
		}
	}
	rel := signaturetest.SignPlatform(t, f.licensedSign, slug, version, platform, sha)
	if err := signature.VerifyPlatform(signature.TierLicensed, slug, version, platform, sha, rel); err != nil {
		t.Fatalf("licensed: %v", err)
	}
}

// TestVerifyPlatformTamperedE121 covers every way the identity or the
// signature can be wrong and still be the key's own tier.
func TestVerifyPlatformTamperedE121(t *testing.T) {
	f := newFixture(t)
	sha := digest("binary tarball")
	good := signaturetest.SignPlatform(t, f.freeSign, slug, version, platform, sha)
	flipped := []byte(good.Sig)
	if flipped[0] == 'A' {
		flipped[0] = 'B'
	} else {
		flipped[0] = 'A'
	}
	cases := map[string]struct {
		slug, version, platform, sha string
		rel                          *signature.Release
	}{
		"nil release":          {slug, version, platform, sha, nil},
		"empty release":        {slug, version, platform, sha, &signature.Release{}},
		"flipped sig byte":     {slug, version, platform, sha, &signature.Release{KeyID: good.KeyID, Alg: good.Alg, Sig: string(flipped)}},
		"wrong alg":            {slug, version, platform, sha, &signature.Release{KeyID: good.KeyID, Alg: "rsa", Sig: good.Sig}},
		"not base64":           {slug, version, platform, sha, &signature.Release{KeyID: good.KeyID, Alg: good.Alg, Sig: "!!!"}},
		"other sha":            {slug, version, platform, digest("other"), good},
		"other slug":           {"minio", version, platform, sha, good},
		"uppercase sha":        {slug, version, platform, strings.ToUpper(sha), good},
		"short sha":            {slug, version, platform, sha[:63], good},
		"empty platform":       {slug, version, "", sha, good},
		"unknown platform":     {slug, version, "freebsd-amd64", sha, good},
		"source as a platform": {slug, version, "source", sha, good},
		"newline in platform":  {slug, version, "linux-amd64\n", sha, good},
		"newline in slug":      {"ci\nx", version, platform, sha, good},
		"empty version":        {slug, "", platform, sha, good},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := code(signature.VerifyPlatform(signature.TierFree, c.slug, c.version, c.platform, c.sha, c.rel)); got != "E121" {
				t.Fatalf("want E121, got %q", got)
			}
		})
	}
	t.Run("unknown tier fails closed", func(t *testing.T) {
		if code(signature.VerifyPlatform("enterprise", slug, version, platform, sha, good)) != "E121" {
			t.Fatal("unknown tier must be E121")
		}
	})
	t.Run("broken key table fails closed", func(t *testing.T) {
		signaturetest.UseKeys(t, []signature.Key{f.free, f.free}, nil)
		if code(signature.VerifyPlatform(signature.TierFree, slug, version, platform, sha, good)) != "E121" {
			t.Fatal("duplicate key must fail closed with E121")
		}
	})
}

// TestVerifyPlatformSignedAmbiguousIdentityE121: the signer signs a message whose
// identity fields would change its line structure or break the shape; the
// verifier must still refuse, so the guards (not just the signature check) are
// what hold.
func TestVerifyPlatformSignedAmbiguousIdentityE121(t *testing.T) {
	f := newFixture(t)
	sha := digest("binary tarball")
	cases := map[string]struct{ slug, version, platform, sha string }{
		"newline in slug":     {"ci\nx", version, platform, sha},
		"newline in version":  {slug, "1.2.7\n", platform, sha},
		"carriage return":     {slug, "1.2.7\r", platform, sha},
		"nul in slug":         {"ci\x00", version, platform, sha},
		"empty slug":          {"", version, platform, sha},
		"empty version":       {slug, "", platform, sha},
		"empty platform":      {slug, version, "", sha},
		"unknown platform":    {slug, version, "freebsd-amd64", sha},
		"upper-case platform": {slug, version, "Linux-amd64", sha},
		"uppercase sha":       {slug, version, platform, strings.ToUpper(sha)},
		"short sha":           {slug, version, platform, sha[:63]},
		"non-hex sha":         {slug, version, platform, strings.Repeat("g", 64)},
		"sha with newline":    {slug, version, platform, sha[:63] + "\n"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			sig, err := f.freeSign.Sign(signature.SigningInputPlatform(c.slug, c.version, c.platform, c.sha))
			if err != nil {
				t.Fatal(err)
			}
			rel := &signature.Release{KeyID: f.freeSign.KeyID(), Alg: signature.Alg, Sig: signing.EncodeSig(sig)}
			if got := code(signature.VerifyPlatform(signature.TierFree, c.slug, c.version, c.platform, c.sha, rel)); got != "E121" {
				t.Fatalf("a signed but ambiguous identity must be E121, got %q", got)
			}
		})
	}
}

func TestVerifyPlatformWrongPlatformE121(t *testing.T) {
	f := newFixture(t)
	sha := digest("binary tarball")
	rel := signaturetest.SignPlatform(t, f.freeSign, slug, version, "linux-amd64", sha)
	for _, p := range signature.Platforms {
		if p == "linux-amd64" {
			continue
		}
		if code(signature.VerifyPlatform(signature.TierFree, slug, version, p, sha, rel)) != "E121" {
			t.Errorf("a linux-amd64 signature verified as %s", p)
		}
	}
}

func TestVerifyPlatformWrongVersionE121(t *testing.T) {
	f := newFixture(t)
	sha := digest("binary tarball")
	rel := signaturetest.SignPlatform(t, f.freeSign, slug, version, platform, sha)
	if code(signature.VerifyPlatform(signature.TierFree, slug, "1.2.8", platform, sha, rel)) != "E121" {
		t.Fatal("a signature for 1.2.7 verified as 1.2.8")
	}
}

func TestVerifyPlatformOtherTierE122(t *testing.T) {
	f := newFixture(t)
	sha := digest("binary tarball")
	free := signaturetest.SignPlatform(t, f.freeSign, slug, version, platform, sha)
	licensed := signaturetest.SignPlatform(t, f.licensedSign, slug, version, platform, sha)
	if code(signature.VerifyPlatform(signature.TierLicensed, slug, version, platform, sha, free)) != "E122" {
		t.Error("a free-tier key must not verify for the licensed tier")
	}
	if code(signature.VerifyPlatform(signature.TierFree, slug, version, platform, sha, licensed)) != "E122" {
		t.Error("a licensed-tier key must not verify for the free tier")
	}
	_, stranger := signaturetest.NewKey(t, signature.TierFree)
	unknown := signaturetest.SignPlatform(t, stranger, slug, version, platform, sha)
	if code(signature.VerifyPlatform(signature.TierFree, slug, version, platform, sha, unknown)) != "E122" {
		t.Error("an unknown key must be E122")
	}
}

func TestVerifyPlatformRevokedE123(t *testing.T) {
	f := newFixture(t)
	sha := digest("binary tarball")
	rel := signaturetest.SignPlatform(t, f.freeSign, slug, version, platform, sha)
	signaturetest.UseKeys(t, []signature.Key{f.free, f.licensed}, []string{f.free.ID})
	if code(signature.VerifyPlatform(signature.TierFree, slug, version, platform, sha, rel)) != "E123" {
		t.Fatal("a revoked key must be E123")
	}
}

// TestPlatformSourceDomainSeparation: the two domains never verify each
// other's signatures, in either direction.
func TestPlatformSourceDomainSeparation(t *testing.T) {
	f := newFixture(t)
	sha := digest("tarball")
	src := signaturetest.Sign(t, f.freeSign, slug, version, sha)
	bin := signaturetest.SignPlatform(t, f.freeSign, slug, version, platform, sha)
	if code(signature.VerifyPlatform(signature.TierFree, slug, version, platform, sha, src)) != "E121" {
		t.Error("a source signature verified as a platform signature")
	}
	if code(signature.Verify(signature.TierFree, slug, version, sha, bin)) != "E121" {
		t.Error("a platform signature verified as a source signature")
	}
}

// fixtureVectors is the subset of the P7-PLUG-83 vectors.json this test reads.
type fixtureVectors struct {
	KeyID                   string `json:"key_id"`
	PlatformTarball         string `json:"platform_tarball"`
	Platform, Slug, Version string
	PlatformSHA256          string            `json:"platform_sha256"`
	PlatformSignature       signature.Release `json:"platform_signature"`
	SourceSignature         signature.Release `json:"source_signature"`
	Cases                   []struct {
		Name, Expect, Platform, Version, Slug, Signature string
		AppendByte                                       bool `json:"tarball_append_byte"`
	}
}

// TestPlatformInteropSignSh verifies the signatures the real plugins sign.sh
// made (P7-PLUG-83 fixture, testdata/platform/README.md): the bash signer and
// the Go verifier agree byte for byte, and each one-field change fails.
func TestPlatformInteropSignSh(t *testing.T) {
	dir := filepath.Join("testdata", "platform")
	var v fixtureVectors
	raw, err := os.ReadFile(filepath.Join(dir, "vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	blk, _ := pem.Decode(mustRead(t, filepath.Join(dir, "fixture.pub")))
	if blk == nil {
		t.Fatal("fixture.pub is not PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(blk.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	edPub := pub.(ed25519.PublicKey)
	idSum := sha256.Sum256(edPub)
	if id := "plugins-" + hex.EncodeToString(idSum[:])[:16]; id != v.KeyID || v.PlatformSignature.KeyID != v.KeyID {
		t.Fatalf("key id %q does not match the fixture key id %q", id, v.KeyID)
	}
	signaturetest.UseKeys(t, []signature.Key{{ID: v.KeyID, Tier: signature.TierFree, PublicKey: edPub}}, nil)
	tarball := mustRead(t, filepath.Join(dir, v.PlatformTarball))
	if len(v.Cases) < 6 {
		t.Fatalf("only %d cases: the vectors file was not read", len(v.Cases))
	}
	for _, c := range v.Cases {
		t.Run(c.Name, func(t *testing.T) {
			body, plat, ver, slg, rel := tarball, v.Platform, v.Version, v.Slug, v.PlatformSignature
			if c.AppendByte {
				body = append(append([]byte{}, tarball...), 0)
			}
			if c.Platform != "" {
				plat = c.Platform
			}
			if c.Version != "" {
				ver = c.Version
			}
			if c.Slug != "" {
				slg = c.Slug
			}
			if c.Signature == "source" {
				rel = v.SourceSignature
			}
			sum := sha256.Sum256(body)
			err := signature.VerifyPlatform(signature.TierFree, slg, ver, plat, hex.EncodeToString(sum[:]), &rel)
			if c.Expect == "ok" && err != nil {
				t.Fatalf("must verify: %v", err)
			}
			if c.Expect == "fail" && code(err) != "E121" {
				t.Fatalf("must fail with E121, got %v", err)
			}
		})
	}
	if sum := sha256.Sum256(tarball); hex.EncodeToString(sum[:]) != v.PlatformSHA256 {
		t.Fatal("the fixture tarball does not match platform_sha256")
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
