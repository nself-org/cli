package embedded

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDownloadAndVerifyFailures(t *testing.T) {
	for _, tc := range []struct {
		name        string
		url         string
		destination string
		truncated   bool
		want        string
	}{
		{name: "invalid request", url: "://invalid", want: "build request"},
		{name: "missing destination parent", destination: "missing/pglite.wasm", want: "create temp file"},
		{name: "truncated response", truncated: true, want: "write download"},
		{name: "destination is directory", destination: "directory", want: "atomic rename"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			dst := filepath.Join(dir, "pglite.wasm")
			if tc.destination != "" {
				dst = filepath.Join(dir, tc.destination)
			}
			if tc.destination == "directory" {
				if err := os.Mkdir(dst, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.truncated {
					w.Header().Set("Content-Length", "1000")
				}
				_, _ = w.Write(testWASMContent)
			}))
			defer srv.Close()
			url := srv.URL
			if tc.url != "" {
				url = tc.url
			}
			err := downloadAndVerify(context.Background(), url, dst, testPin())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("downloadAndVerify error = %v, want %q", err, tc.want)
			}
			matches, err := filepath.Glob(filepath.Join(dir, ".pglite-download-*"))
			if err != nil {
				t.Fatal(err)
			}
			if len(matches) != 0 {
				t.Fatalf("temporary downloads remain: %v", matches)
			}
			if tc.destination != "directory" {
				if _, err := os.Stat(dst); !os.IsNotExist(err) {
					t.Fatalf("destination stat = %v, want absent", err)
				}
			}
		})
	}
}

func TestFetchOrCachedCacheSetupFailures(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"missing home", "resolve home dir"},
		{"blocked cache directory", "create cache dir"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withTestPin(t)
			home := useTempHome(t)
			if tc.name == "missing home" {
				t.Setenv("HOME", "")
				t.Setenv("USERPROFILE", "")
			} else if err := os.WriteFile(filepath.Join(home, ".nself"), []byte("blocked"), 0o600); err != nil {
				t.Fatal(err)
			}
			path, err := FetchOrCached(context.Background(), testVersion)
			if path != "" || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("FetchOrCached = (%q, %v), want empty path and %q", path, err, tc.want)
			}
		})
	}
}

func TestSHA256HexFileFailures(t *testing.T) {
	for _, name := range []string{"missing file", "directory"} {
		t.Run(name, func(t *testing.T) {
			path := t.TempDir()
			if name == "missing file" {
				path = filepath.Join(path, "missing")
			}
			digest, err := sha256HexFile(path)
			if err == nil || digest != "" {
				t.Fatalf("sha256HexFile = (%q, %v), want empty digest and error", digest, err)
			}
		})
	}
}
