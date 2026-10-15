package plugin

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/plugin/verify/signature"
	"github.com/nself-org/cli/internal/plugin/verify/signature/signaturetest"
)

// archiveSwapTransport replaces the verified archive during the SBOM request.
// Other requests still reach the local registry fixture through the transport.
type archiveSwapTransport struct {
	http.RoundTripper
	swap func() error
}

func (s archiveSwapTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !strings.HasSuffix(req.URL.Path, ".cdx.json") {
		return s.RoundTripper.RoundTrip(req)
	}
	if err := s.swap(); err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: http.StatusNotFound,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
	}, nil
}

// TestInstallArchiveChangedDuringSBOM refuses bytes that no longer match the
// checksum authenticated by the release signature, before creating install files.
func TestInstallArchiveChangedDuringSBOM(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		tarball := sigTarball(t)
		checksum := sha256Hex(tarball)
		key, signer := signaturetest.NewKey(t, signature.TierFree)
		signaturetest.UseKeys(t, []signature.Key{key}, nil)
		signaturetest.UseFloor(t, map[string]map[string]string{})
		sigServer(t, tarball, checksum, relMap(signaturetest.Sign(t, signer, sigPlugin, "1.0.0", checksum)))
		t.Setenv("NSELF_SKIP_SBOM_CHECK", "")
		tmp := t.TempDir()
		for _, env := range []string{"TMPDIR", "TMP", "TEMP"} {
			t.Setenv(env, tmp)
		}
		replacement := writeTempArchive(t, buildTarGz(t, []struct{ name, content string }{
			{"plugin.json", `{"name":"` + sigPlugin + `","version":"1.0.0"}`},
			{"unverified.txt", "must never be installed"},
		}))
		original := http.DefaultTransport
		t.Cleanup(func() { http.DefaultTransport = original })
		var swapped bool
		var swapErr error
		http.DefaultTransport = archiveSwapTransport{RoundTripper: original, swap: func() error {
			archives, err := filepath.Glob(filepath.Join(tmp, "nself-plugin-*.tar.gz"))
			if err != nil || len(archives) != 1 {
				swapErr = fmt.Errorf("want one downloaded archive, got %v: %v", archives, err)
				return swapErr
			}
			swapErr = os.Rename(replacement, archives[0])
			swapped = swapErr == nil
			return swapErr
		}}

		dir, err, _ := installSig(t)
		if !swapped {
			t.Fatalf("SBOM request did not replace the verified archive: %v", swapErr)
		}
		if codeOf(err) != "E124" {
			t.Errorf("want E124 after archive replacement, got %v", err)
		}
		if !dirEmpty(t, dir) {
			t.Error("unverified archive created install files")
		}
		archives, globErr := filepath.Glob(filepath.Join(tmp, "nself-plugin-*.tar.gz"))
		if globErr != nil || len(archives) != 0 {
			t.Errorf("downloaded archive was not cleaned up: %v, %v", archives, globErr)
		}
	})
}
