package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
)

func TestInstallArchiveUsesOpenFile(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged", true: "replaced-path"}[replace], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "archive.tar.gz")
			original := binTarball(t, "original")
			if err := os.WriteFile(path, original, 0600); err != nil {
				t.Fatal(err)
			}
			archive, err := openInstallArchive(path)
			if err != nil {
				t.Fatal(err)
			}
			defer archive.Close()
			if replace {
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, binTarball(t, "replacement"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := archive.checksum(sha256Hex(original), "stable"); err != nil {
				t.Fatal(err)
			}
			key, signer := signaturetest.NewKey(t, signature.TierFree)
			signaturetest.UseKeys(t, []signature.Key{key}, nil)
			m := &PluginManifest{Name: sigPlugin, Version: "1.0.0", PlatformSignatures: map[string]*signature.Release{"linux-amd64": signaturetest.SignPlatform(t, signer, sigPlugin, "1.0.0", "linux-amd64", sha256Hex(original))}}
			if err := archive.signature(m, false, "linux-amd64", false); err != nil {
				t.Fatal(err)
			}
			dest := t.TempDir()
			if err := archive.extract(dest); err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(filepath.Join(dest, "nself-"+sigPlugin))
			if err != nil || string(body) != "#!/bin/sh\noriginal" {
				t.Fatalf("extracted %q, %v", body, err)
			}
		})
	}
}
