package signature_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestAuthorPublicKeyNeverTrusted: the manifest's self-certifying
// author_public_key / signature pair is decoded for compatibility but no code
// that decides trust may read it. Greps the signature package and the
// installer files (comments excluded).
func TestAuthorPublicKeyNeverTrusted(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"security.go", "installer_signature.go", "installer_locked.go"} {
		files = append(files, filepath.Join("..", "..", f))
	}
	banned := regexp.MustCompile(`AuthorPublicKey|author_public_key|[mM]anifest\.Signature|ed25519\.Verify\(`)
	scanned := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		for i, line := range strings.Split(string(data), "\n") {
			code := line
			if j := strings.Index(code, "//"); j >= 0 {
				code = code[:j]
			}
			if loc := banned.FindString(code); loc != "" {
				t.Errorf("%s:%d reads %q", f, i+1, loc)
			}
		}
	}
	if scanned < 7 {
		t.Fatalf("scanned only %d files; the glob is wrong", scanned)
	}
	// Positive control: the pattern does match what it is meant to catch.
	if !banned.MatchString("m.AuthorPublicKey") || !banned.MatchString("manifest.Signature") || !banned.MatchString("ed25519.Verify(k, m, s)") {
		t.Fatal("control: pattern does not match AuthorPublicKey")
	}
}
