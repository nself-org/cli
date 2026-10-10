package machineauth

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

// homeDir points HOME and USERPROFILE at a fresh temp dir and clears the
// token env var.
func homeDir(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	t.Setenv(EnvVar, "")
	return h
}

func writeTokenFile(t *testing.T, home, content string, mode os.FileMode) string {
	t.Helper()
	dir := filepath.Join(home, ".nself")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "mcp-token")
	if err := os.WriteFile(p, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func wantE426(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected E426, got nil")
	}
	var ce *errs.CLIError
	if !asCLI(err, &ce) || ce.Code != "E426" {
		t.Fatalf("expected an E426 CLIError, got %v", err)
	}
	for _, p := range parts {
		if !strings.Contains(err.Error(), p) {
			t.Errorf("error %q lacks %q", err.Error(), p)
		}
	}
}

func asCLI(err error, target **errs.CLIError) bool { return errors.As(err, target) }

func TestTokenResolve(t *testing.T) {
	t.Run("env wins", func(t *testing.T) {
		home := homeDir(t)
		writeTokenFile(t, home, "filetok\n", 0o600)
		t.Setenv(EnvVar, "envtok")
		tok, src, err := Lookup()
		if err != nil || tok != "envtok" || src != "env" {
			t.Fatalf("got %q %q %v", tok, src, err)
		}
		tok, src, gen, err := ResolveTo(&bytes.Buffer{})
		if err != nil || tok != "envtok" || src != "env" || gen {
			t.Fatalf("resolve got %q %q %v %v", tok, src, gen, err)
		}
	})

	t.Run("0600 file used and trimmed", func(t *testing.T) {
		home := homeDir(t)
		writeTokenFile(t, home, "  filetok \n", 0o600)
		tok, src, err := Lookup()
		if err != nil || tok != "filetok" || src != "file" {
			t.Fatalf("got %q %q %v", tok, src, err)
		}
	})

	t.Run("0644 file refused", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("POSIX modes only")
		}
		home := homeDir(t)
		p := writeTokenFile(t, home, "filetok\n", 0o644)
		_, _, err := Lookup()
		wantE426(t, err, p, "chmod 600")
		_, _, _, err = ResolveTo(&bytes.Buffer{})
		wantE426(t, err, p)
	})

	t.Run("empty file refused", func(t *testing.T) {
		home := homeDir(t)
		writeTokenFile(t, home, " \n", 0o600)
		_, _, err := Lookup()
		wantE426(t, err, "empty")
	})

	t.Run("symlink refused", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks need privileges on Windows")
		}
		home := homeDir(t)
		real := filepath.Join(home, "real-token")
		if err := os.WriteFile(real, []byte("x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(home, ".nself"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(real, filepath.Join(home, ".nself", "mcp-token")); err != nil {
			t.Fatal(err)
		}
		_, _, err := Lookup()
		wantE426(t, err, "not a regular file")
	})

	t.Run("neither then generated once", func(t *testing.T) {
		home := homeDir(t)
		tok, src, err := Lookup()
		if err != nil || tok != "" || src != "" {
			t.Fatalf("Lookup with nothing: %q %q %v", tok, src, err)
		}
		if _, statErr := os.Stat(filepath.Join(home, ".nself")); statErr == nil {
			t.Fatal("Lookup must not create anything")
		}
		var buf bytes.Buffer
		tok, src, gen, err := ResolveTo(&buf)
		if err != nil || tok == "" || src != "generated" || !gen {
			t.Fatalf("resolve got %q %q %v %v", tok, src, gen, err)
		}
		if len(tok) != 64 {
			t.Errorf("token length %d, want 64 hex chars", len(tok))
		}
		p := filepath.Join(home, ".nself", "mcp-token")
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		di, _ := os.Stat(filepath.Dir(p))
		if runtime.GOOS != "windows" {
			if fi.Mode().Perm() != 0o600 {
				t.Errorf("file mode %v, want 0600", fi.Mode().Perm())
			}
			if di.Mode().Perm() != 0o700 {
				t.Errorf("dir mode %v, want 0700", di.Mode().Perm())
			}
		}
		tok2, src2, gen2, err := ResolveTo(&buf)
		if err != nil || tok2 != tok || src2 != "file" || gen2 {
			t.Fatalf("second resolve got %q %q %v %v", tok2, src2, gen2, err)
		}
	})

	t.Run("group-writable dir refused and untouched", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("POSIX modes only")
		}
		home := homeDir(t)
		dir := filepath.Join(home, ".nself")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dir, 0o770); err != nil {
			t.Fatal(err)
		}
		_, _, _, err := ResolveTo(&bytes.Buffer{})
		wantE426(t, err, dir)
		di, _ := os.Stat(dir)
		if di.Mode().Perm() != 0o770 {
			t.Errorf("existing dir was chmod-ed to %v", di.Mode().Perm())
		}
		if _, err := os.Stat(filepath.Join(dir, "mcp-token")); err == nil {
			t.Error("a refused Resolve must not create the token file")
		}
	})
}

func TestTokenNeverPrinted(t *testing.T) {
	home := homeDir(t)
	var buf bytes.Buffer
	tok, _, gen, err := ResolveTo(&buf)
	if err != nil || !gen {
		t.Fatalf("resolve: %v %v", gen, err)
	}
	out := buf.String()
	p := filepath.Join(home, ".nself", "mcp-token")
	if !strings.Contains(out, p) {
		t.Errorf("notice %q lacks the path %s", out, p)
	}
	if !strings.Contains(out, "cat "+p) {
		t.Errorf("notice %q lacks the cat hint", out)
	}
	if strings.Contains(out, tok) {
		t.Error("the token value appeared in the notice")
	}
	if got, _ := TokenPath(); got != p {
		t.Errorf("TokenPath %q, want %q", got, p)
	}
}

func TestIsLoopback(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"localhost", true}, {"LOCALHOST", true}, {"127.0.0.1", true}, {"127.1.2.3", true},
		{"::1", true}, {"[::1]", true},
		{"", false}, {"0.0.0.0", false}, {"::", false}, {"[::]", false},
		{"192.168.1.5", false}, {"10.0.0.1", false}, {"example.com", false}, {"128.0.0.1", false},
	}
	for _, c := range cases {
		if got := IsLoopback(c.host); got != c.want {
			t.Errorf("IsLoopback(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestCheckBind(t *testing.T) {
	cases := []struct {
		name    string
		host    string
		allowed []string
		source  string
		refuse  bool
	}{
		{"loopback no token", "127.0.0.1", nil, "", false},
		{"localhost generated", "localhost", nil, "generated", false},
		{"wildcard no allowed", "0.0.0.0", nil, "env", true},
		{"wildcard allowed env", "0.0.0.0", []string{"nself.example.com"}, "env", false},
		{"wildcard allowed file", "0.0.0.0", []string{"nself.example.com"}, "file", false},
		{"wildcard allowed generated", "0.0.0.0", []string{"nself.example.com"}, "generated", true},
		{"wildcard allowed no token", "0.0.0.0", []string{"nself.example.com"}, "", true},
		{"blank allowed entry", "0.0.0.0", []string{" ", ""}, "env", true},
		{"empty host", "", []string{"x"}, "", true},
	}
	for _, c := range cases {
		err := CheckBind(c.host, c.allowed, c.source)
		if (err != nil) != c.refuse {
			t.Errorf("%s: err=%v, refuse=%v", c.name, err, c.refuse)
			continue
		}
		if err != nil {
			wantE426(t, err, "--allowed-host", EnvVar)
		}
	}
}

func TestBearerMiddleware(t *testing.T) {
	called := 0
	h := Middleware("secret-token", "nself-mcp")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	}))
	do := func(auth string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	rec := do("")
	if rec.Code != 401 || rec.Header().Get("WWW-Authenticate") != `Bearer realm="nself-mcp"` {
		t.Errorf("no header: %d %v", rec.Code, rec.Header())
	}
	if strings.TrimSpace(rec.Body.String()) != "unauthorized" {
		t.Errorf("body %q", rec.Body.String())
	}
	for _, bad := range []string{"Bearer wrong", "Bearer secret-token-longer", "Bearer secret", "Bearer ", "secret-tokeX"} {
		if r := do(bad); r.Code != 401 {
			t.Errorf("%q: got %d, want 401", bad, r.Code)
		}
	}
	if called != 0 {
		t.Fatalf("handler ran %d times for rejected requests", called)
	}
	if r := do("Bearer secret-token"); r.Code != 200 || called != 1 {
		t.Errorf("right token: %d called=%d", r.Code, called)
	}
	// An empty configured token rejects everything.
	deny := Middleware("", "r")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called++ }))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec = httptest.NewRecorder()
	deny.ServeHTTP(rec, req)
	if rec.Code != 401 || called != 1 {
		t.Errorf("empty token: %d called=%d", rec.Code, called)
	}
}
