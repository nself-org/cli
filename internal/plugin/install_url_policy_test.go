package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestURLInstallRequiresUnsignedOptIn(t *testing.T) {
	for _, v15 := range []string{"0", "1"} {
		t.Run(v15, func(t *testing.T) {
			t.Setenv("NSELF_V15", v15)
			err := InstallFromURL(context.Background(), &config.Config{}, "http://127.0.0.1:1/plugin.tgz", t.TempDir(), "")
			if err == nil || (strings.Contains(err.Error(), "E107") != (v15 == "1")) {
				t.Fatalf("mode %s: %v", v15, err)
			}
		})
	}
}

func TestURLCannotReplaceRegistryPlugin(t *testing.T) {
	for _, registered := range []bool{false, true} {
		t.Run(map[bool]string{false: "unofficial", true: "official"}[registered], func(t *testing.T) {
			setTestHome(t, t.TempDir())
			body := `{"plugins":{}}`
			if registered {
				body = `{"plugins":{"example":{"name":"example","version":"1.0.0","tier":"free"}}}`
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer server.Close()
			t.Setenv("NSELF_PLUGIN_REGISTRY", server.URL)
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "example"), 0700); err != nil {
				t.Fatal(err)
			}
			err := checkURLReplacement(context.Background(), dir, "example")
			if (err != nil) != registered {
				t.Fatalf("registered=%v: %v", registered, err)
			}
		})
	}
}

func TestUnsignedURLContextDoesNotLeak(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	for _, allow := range []bool{true, false} {
		err := CheckUnsignedURLInstall(WithUnsignedURLAllowed(context.Background(), allow))
		if (err == nil) != allow {
			t.Fatalf("allow=%v: %v", allow, err)
		}
	}
}

func TestURLInstallPreservesOfficialDirectory(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	setTestHome(t, t.TempDir())
	tarball := buildThirdPartyTarball(t, validThirdPartyManifest)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/archive" {
			_, _ = w.Write(tarball)
			return
		}
		_, _ = w.Write([]byte(`{"plugins":{"third-party-demo":{"name":"third-party-demo","version":"1.0.0","tier":"free"}}}`))
	}))
	defer server.Close()
	t.Setenv("NSELF_PLUGIN_REGISTRY", server.URL)
	dir := t.TempDir()
	dest := filepath.Join(dir, "third-party-demo")
	if err := os.Mkdir(dest, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dest, "keep")
	if err := os.WriteFile(marker, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	err := InstallFromURL(WithUnsignedURLAllowed(context.Background(), true), &config.Config{}, server.URL+"/archive", dir, "")
	if err == nil || !strings.Contains(err.Error(), "refusing to replace registry plugin") {
		t.Fatalf("want registry refusal, got %v", err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "original" {
		t.Fatalf("official directory changed: %q %v", data, err)
	}
}
