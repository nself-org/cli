package docker

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

const indexFixture = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[` +
	`{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:aaa","platform":{"architecture":"amd64","os":"linux"}},` +
	`{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:bbb","platform":{"architecture":"arm64","os":"linux","variant":"v8"}},` +
	`{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:ccc","platform":{"architecture":"arm","os":"linux","variant":"v7"}},` +
	`{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:ddd","platform":{"architecture":"ppc64le","os":"linux"}},` +
	`{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":"sha256:eee","platform":{"architecture":"unknown","os":"unknown"}}]}`

func TestParseIndex(t *testing.T) {
	sum := sha256.Sum256([]byte(indexFixture))
	want := "sha256:" + hex.EncodeToString(sum[:])
	for _, raw := range []string{indexFixture, indexFixture + "\n"} {
		info, err := ParseIndex([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if info.Digest != want {
			t.Errorf("Digest = %s, want %s (a trailing newline must not change it)", info.Digest, want)
		}
		got := info.Platforms
		if len(got) != 3 || got["linux/amd64"] != "sha256:aaa" || got["linux/arm64"] != "sha256:bbb" || got["linux/arm/v7"] != "sha256:ccc" {
			t.Errorf("Platforms = %v, want amd64, arm64 and arm/v7 only", got)
		}
	}
}

func TestParseIndexRejectsSingleManifest(t *testing.T) {
	single := `{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json","config":{"digest":"sha256:x"}}`
	if _, err := ParseIndex([]byte(single)); !errors.Is(err, ErrNotIndex) {
		t.Errorf("err = %v, want ErrNotIndex", err)
	}
	if _, err := ParseIndex([]byte("not json")); err == nil {
		t.Error("garbage input must be an error")
	}
}
