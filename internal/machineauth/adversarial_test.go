package machineauth

// Purpose: adversarial suite for the machine bearer token (P7-SURF-21 unit B).
// Inputs: hostile Authorization shapes, bind hosts and token file layouts.
// Outputs: TestMachineAuthAdversarial; every subtest asserts the SAFE outcome.
// Constraints: fake tokens only; no sleeps; token files live in t.TempDir().
//   Helpers homeDir, writeTokenFile and wantE426 come from machineauth_test.go.

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const advTok = "nself_test_FAKE_token_0001"

// panicBody panics if anything reads the request body.
type panicBody struct{}

func (panicBody) Read([]byte) (int, error) { panic("body read by the auth middleware") }

// TestMachineAuthAdversarial runs every adversarial theme as a named subtest.
func TestMachineAuthAdversarial(t *testing.T) {
	t.Run("middleware-credentials", advMiddlewareCredentials)
	t.Run("middleware-responses", advMiddlewareResponses)
	t.Run("constant-time", advConstantTime)
	t.Run("bind", advBind)
	t.Run("token-file", advTokenFile)
	t.Run("generation", advGeneration)
	for _, u := range unixSubtests {
		t.Run(u.name, u.fn)
	}
}

// advServe runs one request with the given Authorization header values (nil
// means none) through the middleware and returns the recorder and call count.
func advServe(h http.Handler, calls *atomic.Int64, auth []string, alter func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/mcp", nil)
	r.Body = io.NopCloser(panicBody{})
	for _, v := range auth {
		r.Header.Add("Authorization", v)
	}
	if alter != nil {
		alter(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func advHandler(calls *atomic.Int64) http.Handler {
	return Middleware(advTok, "nself-mcp")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
}

// advMiddlewareCredentials covers missing, malformed, mis-scheme'd, repeated
// and near-miss credentials. The body is a panic reader: the middleware must
// refuse without touching it.
func advMiddlewareCredentials(t *testing.T) {
	var calls atomic.Int64
	h := advHandler(&calls)
	deny := func(t *testing.T, auth []string, alter func(*http.Request)) {
		t.Helper()
		before := calls.Load()
		w := advServe(h, &calls, auth, alter)
		if w.Code != 401 || calls.Load() != before {
			t.Fatalf("status %d, handler ran %d times", w.Code, calls.Load()-before)
		}
	}
	for name, auth := range map[string][]string{
		"missing":          nil,
		"empty value":      {""},
		"scheme only":      {"Bearer"},
		"scheme and space": {"Bearer "},
		"basic scheme":     {"Basic " + advTok},
		"token scheme":     {"Token " + advTok},
		"lowercase scheme": {"bearer " + advTok},
		"uppercase scheme": {"BEARER " + advTok},
		"double space":     {"Bearer  " + advTok},
		"trailing space":   {"Bearer " + advTok + " "},
		"tab separator":    {"Bearer\t" + advTok},
		"token twice":      {"Bearer " + advTok + " " + advTok},
		"prefix only":      {advTok[:len(advTok)-1]},
		"wrong then right": {"Bearer wrong", "Bearer " + advTok},
		"right then wrong": {"Bearer " + advTok, "Bearer wrong"},
		"right twice":      {"Bearer " + advTok, "Bearer " + advTok},
	} {
		t.Run(name, func(t *testing.T) { deny(t, auth, nil) })
	}
	t.Run("token outside the Authorization header", func(t *testing.T) {
		deny(t, nil, func(r *http.Request) {
			r.URL.RawQuery = "token=" + advTok + "&access_token=" + advTok
			r.Header.Set("Cookie", "token="+advTok)
			for _, k := range []string{"X-Api-Key", "X-Auth-Token", "X-Nself-Token", "Proxy-Authorization"} {
				r.Header.Set(k, advTok)
			}
		})
	})
	t.Run("near-miss tokens of every length", func(t *testing.T) {
		for n := 0; n <= len(advTok)+3; n++ {
			if cand := (advTok + "xyz")[:n]; cand != advTok {
				deny(t, []string{"Bearer " + cand}, nil)
			}
		}
		for i := range advTok {
			b := []byte(advTok)
			b[i]++
			deny(t, []string{"Bearer " + string(b)}, nil)
		}
		deny(t, []string{"Bearer " + strings.Repeat(advTok, 4096)}, nil)
	})
	t.Run("empty configured token fails closed", func(t *testing.T) {
		var n atomic.Int64
		closed := Middleware("", "r")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { n.Add(1) }))
		for _, v := range [][]string{nil, {""}, {"Bearer"}, {"Bearer "}, {"Bearer " + advTok}, {" "}} {
			if w := advServe(closed, &n, v, nil); w.Code != 401 {
				t.Fatalf("%q: status %d", v, w.Code)
			}
		}
		if n.Load() != 0 {
			t.Fatal("empty token admitted a request")
		}
	})
	t.Run("the right token passes", func(t *testing.T) {
		var n atomic.Int64
		ok := Middleware(advTok, "r")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { n.Add(1) }))
		if w := advServe(ok, &n, []string{"Bearer " + advTok}, nil); w.Code != 200 || n.Load() != 1 {
			t.Fatalf("status %d calls %d", w.Code, n.Load())
		}
	})
	t.Run("a bare token without the scheme is accepted (v1.4 compatibility, finding SURF21-B2)", func(t *testing.T) {
		// middleware.go documents the v1.4 TrimPrefix parsing as deliberate. The
		// secret is still required, so this is not a bypass. This subtest pins the
		// documented behaviour so that tightening it is a conscious change.
		var n atomic.Int64
		ok := Middleware(advTok, "r")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { n.Add(1) }))
		if w := advServe(ok, &n, []string{advTok}, nil); w.Code != 200 {
			t.Fatalf("documented v1.4 parsing changed: status %d", w.Code)
		}
	})
}

// advMiddlewareResponses checks what a refusal says, and concurrent use.
func advMiddlewareResponses(t *testing.T) {
	var calls atomic.Int64
	h := advHandler(&calls)
	t.Run("refusal never echoes a credential", func(t *testing.T) {
		for _, cred := range []string{"Bearer nself_test_FAKE_wrong_0002", advTok + "x", "Bearer " + advTok + "x"} {
			w := advServe(h, &calls, []string{cred}, nil)
			dump := w.Body.String()
			for k, v := range w.Header() {
				dump += k + ": " + strings.Join(v, ",") + "\n"
			}
			if strings.Contains(dump, "nself_test_FAKE") || strings.TrimSpace(w.Body.String()) != "unauthorized" {
				t.Fatalf("response %q", dump)
			}
			if w.Header().Get("WWW-Authenticate") != `Bearer realm="nself-mcp"` {
				t.Fatalf("challenge %q", w.Header().Get("WWW-Authenticate"))
			}
		}
	})
	t.Run("realm cannot inject headers", func(t *testing.T) {
		hostile := "x\"\r\nSet-Cookie: pwn=1\r\nX-Evil: 1"
		h := Middleware(advTok, hostile)(http.NotFoundHandler())
		w := advServe(h, &calls, nil, nil)
		if w.Header().Get("Set-Cookie") != "" || w.Header().Get("X-Evil") != "" || strings.ContainsAny(w.Header().Get("WWW-Authenticate"), "\r\n") {
			t.Fatalf("header injection: %v", w.Header())
		}
	})
	t.Run("responses do not depend on how much of the token matches", func(t *testing.T) {
		var first string
		for n := 0; n < len(advTok); n++ {
			w := advServe(h, &calls, []string{"Bearer " + advTok[:n] + strings.Repeat("~", len(advTok)-n)}, nil)
			var b bytes.Buffer
			b.WriteString(w.Body.String())
			for _, k := range []string{"Www-Authenticate", "Content-Type"} {
				b.WriteString(w.Header().Get(k))
			}
			got := strings.Join([]string{http.StatusText(w.Code), b.String()}, "|")
			if n == 0 {
				first = got
			} else if got != first {
				t.Fatalf("response differs at matching prefix %d: %q vs %q", n, got, first)
			}
		}
	})
	t.Run("concurrent requests", func(t *testing.T) {
		var n atomic.Int64
		h := Middleware(advTok, "r")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { n.Add(1) }))
		var wg sync.WaitGroup
		var bad atomic.Int64
		for i := 0; i < 64; i++ {
			wg.Add(1)
			go func(good bool) {
				defer wg.Done()
				v := "Bearer wrong"
				want := 401
				if good {
					v, want = "Bearer "+advTok, 200
				}
				if advServe(h, &n, []string{v}, nil).Code != want {
					bad.Add(1)
				}
			}(i%2 == 0)
		}
		wg.Wait()
		if bad.Load() != 0 || n.Load() != 32 {
			t.Fatalf("bad %d, handler calls %d", bad.Load(), n.Load())
		}
	})
}

// advConstantTime is the structural stand-in for a timing measurement: both
// sides are hashed, compared with crypto/subtle, and no secret-named value is
// compared with == or !=.
func advConstantTime(t *testing.T) {
	check := func(t *testing.T, file string, wantCompare bool) {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		imports := map[string]bool{}
		for _, imp := range f.Imports {
			imports[strings.Trim(imp.Path.Value, `"`)] = true
		}
		if imports["math/rand"] {
			t.Errorf("%s imports math/rand", file)
		}
		if wantCompare && (!imports["crypto/subtle"] || !imports["crypto/sha256"]) {
			t.Errorf("%s must import crypto/subtle and crypto/sha256", file)
		}
		called := false
		secretish := map[string]bool{"tok": true, "got": true, "want": true, "value": true, "token": true, "sum": true}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "ConstantTimeCompare" {
					called = true
				}
			case *ast.BinaryExpr:
				if x.Op != token.EQL && x.Op != token.NEQ {
					return true
				}
				for i, side := range []ast.Expr{x.X, x.Y} {
					other := []ast.Expr{x.Y, x.X}[i]
					lit, isLit := other.(*ast.BasicLit)
					if id, ok := side.(*ast.Ident); ok && secretish[id.Name] && !(isLit && lit.Value == `""`) {
						t.Errorf("%s: %s compares %q with %s", file, fset.Position(x.Pos()), id.Name, x.Op)
					}
				}
			}
			return true
		})
		if wantCompare && !called {
			t.Errorf("%s never calls subtle.ConstantTimeCompare", file)
		}
	}
	t.Run("middleware.go", func(t *testing.T) { check(t, "middleware.go", true) })
	t.Run("machineauth.go uses crypto/rand", func(t *testing.T) {
		check(t, "machineauth.go", false)
		f, err := parser.ParseFile(token.NewFileSet(), "machineauth.go", nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, imp := range f.Imports {
			found = found || imp.Path.Value == `"crypto/rand"`
		}
		if !found {
			t.Fatal("machineauth.go does not import crypto/rand")
		}
	})
}

// advBind covers hostile bind hosts: only real loopback addresses pass, and a
// non-loopback bind needs a named host and a configured token.
func advBind(t *testing.T) {
	loop := []string{"localhost", "LOCALHOST", "127.0.0.1", "127.255.255.254", "::1", "[::1]", "0:0:0:0:0:0:0:1", "::ffff:127.0.0.1"}
	notLoop := []string{
		"", " ", "0.0.0.0", "::", "[::]", "::0", "0", "*", "0.0.0.0/0", "[::ffff:0.0.0.0]", "example.com", "my-host", "evil.localhost",
		"localhost.evil.com", "127.0.0.1.evil.com", "localhost.", "localhost:8080", "127.0.0.1:8080", "[::1]:8080", "::1:8080",
		"2130706433", "0x7f.0.0.1", "0177.0.0.1", "127.1", "127.0.0.1/8", "::1%lo0", "fe80::1", "192.168.1.5", "10.0.0.1", "128.0.0.1",
		"a@127.0.0.1", "http://127.0.0.1", "127.0.0.1\x00.evil.com", "localhost\x00",
	}
	for _, h := range loop {
		t.Run("loopback/"+h, func(t *testing.T) {
			if !IsLoopback(h) {
				t.Fatalf("%q must be loopback", h)
			}
			if err := CheckBind(h, nil, ""); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, h := range notLoop {
		t.Run("refused/"+h, func(t *testing.T) {
			if IsLoopback(h) {
				t.Fatalf("%q must not be loopback", h)
			}
			for _, src := range []string{"", SourceGenerated, SourceEnv, SourceFile} {
				if err := CheckBind(h, nil, src); err == nil {
					t.Fatalf("bind %q with source %q and no allowed host was accepted", h, src)
				} else {
					wantE426(t, err, "--allowed-host")
					if strings.Contains(err.Error(), advTok) {
						t.Fatal("token in error")
					}
				}
			}
			for _, allowed := range [][]string{{""}, {" "}, {"\t", ""}} {
				if CheckBind(h, allowed, SourceEnv) == nil {
					t.Fatalf("blank allowed hosts %q accepted for %q", allowed, h)
				}
			}
		})
	}
	t.Run("a generated token never counts as configured", func(t *testing.T) {
		for _, h := range []string{"0.0.0.0", "::", "example.com"} {
			if CheckBind(h, []string{"nself.example.com"}, SourceGenerated) == nil {
				t.Fatalf("%q accepted with a generated token", h)
			}
		}
	})
	t.Run("a named host and a configured token are both required", func(t *testing.T) {
		if CheckBind("0.0.0.0", []string{"nself.example.com"}, "") == nil || CheckBind("0.0.0.0", nil, SourceEnv) == nil {
			t.Fatal("one condition was enough")
		}
	})
}

// advTokenFile covers token file layouts an attacker or accident could make.
func advTokenFile(t *testing.T) {
	const secret = "nself_test_FAKE_filetoken_0002"
	noLeak := func(t *testing.T, err error) {
		t.Helper()
		if err != nil && strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaks the token: %v", err)
		}
	}
	if runtime.GOOS != "windows" {
		for _, mode := range []os.FileMode{0o644, 0o640, 0o604, 0o660, 0o606, 0o666, 0o777, 0o470, 0o407} {
			t.Run("refused mode "+mode.String(), func(t *testing.T) {
				home := homeDir(t)
				p := writeTokenFile(t, home, secret+"\n", mode)
				tok, _, err := Lookup()
				wantE426(t, err, p)
				noLeak(t, err)
				if tok != "" {
					t.Fatal("token returned from a refused file")
				}
				_, _, gen, err := ResolveTo(io.Discard)
				wantE426(t, err, p)
				noLeak(t, err)
				if b, _ := os.ReadFile(p); gen || string(b) != secret+"\n" {
					t.Fatal("a refused file was replaced")
				}
			})
		}
		for _, mode := range []os.FileMode{0o600, 0o400, 0o700} {
			t.Run("accepted mode "+mode.String(), func(t *testing.T) {
				home := homeDir(t)
				writeTokenFile(t, home, secret+"\r\n", mode)
				if tok, src, err := Lookup(); err != nil || tok != secret || src != SourceFile {
					t.Fatalf("%q %q %v", tok, src, err)
				}
			})
		}
		t.Run("symlinks and special files are refused", func(t *testing.T) {
			home := homeDir(t)
			dir := filepath.Join(home, ".nself")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(dir, "mcp-token")
			real := filepath.Join(home, "real-token")
			if err := os.WriteFile(real, []byte(secret+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			steps := map[string]func() error{
				"symlink to a file": func() error { return os.Symlink(real, link) },
				"symlink to a dir":  func() error { return os.Symlink(home, link) },
				"dangling symlink":  func() error { return os.Symlink(filepath.Join(home, "nowhere"), link) },
				"directory":         func() error { return os.Mkdir(link, 0o700) },
				"symlink loop":      func() error { return os.Symlink(link, link) },
			}
			for name, make := range steps {
				_ = os.Remove(link)
				if err := make(); err != nil {
					t.Fatal(name, err)
				}
				_, _, err := Lookup()
				wantE426(t, err)
				noLeak(t, err)
				if _, _, _, err := ResolveTo(io.Discard); err == nil {
					t.Fatalf("%s: Resolve wrote through it", name)
				}
				if _, err := os.Stat(filepath.Join(home, "nowhere")); err == nil {
					t.Fatalf("%s: dangling target was created", name)
				}
				_ = os.Remove(link)
			}
		})
		t.Run("a file where the directory should be", func(t *testing.T) {
			home := homeDir(t)
			if err := os.WriteFile(filepath.Join(home, ".nself"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := Lookup(); err == nil {
				t.Fatal("Lookup accepted a path below a regular file")
			}
			if _, _, _, err := ResolveTo(io.Discard); err == nil {
				t.Fatal("Resolve created a token below a regular file")
			}
		})
		t.Run("owner other than the current user", func(t *testing.T) {
			home := homeDir(t)
			writeTokenFile(t, home, secret+"\n", 0o600)
			oldOwner, oldUID := fileOwner, currentUID
			t.Cleanup(func() { fileOwner, currentUID = oldOwner, oldUID })
			for _, owner := range []int{0, 1, 65534, 4242} {
				fileOwner, currentUID = func(os.FileInfo) (int, bool) { return owner, true }, func() int { return 501 }
				_, _, err := Lookup()
				wantE426(t, err, "owned by uid")
				noLeak(t, err)
			}
		})
		t.Run("read-only home cannot produce a token file", func(t *testing.T) {
			if os.Geteuid() == 0 {
				t.Skip("root ignores directory modes")
			}
			home := homeDir(t)
			if err := os.Chmod(home, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(home, 0o700) })
			var notice bytes.Buffer
			tok, _, _, err := ResolveTo(&notice)
			wantE426(t, err)
			if tok != "" || notice.Len() != 0 {
				t.Fatal("a token or notice appeared without a file")
			}
		})
		t.Run("a wide existing directory is narrowed, never widened", func(t *testing.T) {
			home := homeDir(t)
			dir := filepath.Join(home, ".nself")
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, 0o777); err != nil {
				t.Fatal(err)
			}
			if _, src, gen, err := ResolveTo(io.Discard); err != nil || !gen || src != SourceGenerated {
				t.Fatalf("%q %v %v", src, gen, err)
			}
			st, _ := os.Stat(dir)
			fi, _ := os.Stat(filepath.Join(dir, "mcp-token"))
			if st.Mode().Perm() != 0o700 || fi.Mode().Perm() != 0o600 {
				t.Fatalf("dir %04o file %04o", st.Mode().Perm(), fi.Mode().Perm())
			}
		})
	}
	t.Run("empty and blank files are refused", func(t *testing.T) {
		for _, content := range []string{"", " ", "\n", " \t\r\n\n"} {
			home := homeDir(t)
			writeTokenFile(t, home, content, 0o600)
			_, _, err := Lookup()
			wantE426(t, err, "empty")
		}
	})
	t.Run("a huge file is read in bounded size", func(t *testing.T) {
		home := homeDir(t)
		writeTokenFile(t, home, strings.Repeat("a", 4<<20), 0o600)
		if tok, _, err := Lookup(); err == nil && len(tok) > 1<<16 {
			t.Fatalf("read %d bytes of token", len(tok))
		}
	})
	t.Run("the environment variable wins and the file is not consulted", func(t *testing.T) {
		home := homeDir(t)
		if runtime.GOOS != "windows" {
			writeTokenFile(t, home, secret+"\n", 0o666)
		}
		t.Setenv(EnvVar, "nself_test_FAKE_env_0003")
		tok, src, err := Lookup()
		if err != nil || tok != "nself_test_FAKE_env_0003" || src != SourceEnv {
			t.Fatalf("%q %q %v", tok, src, err)
		}
	})
}

// advGeneration checks that generated tokens are long, random and private.
func advGeneration(t *testing.T) {
	const n = 64
	seen := map[string]bool{}
	prefixes := map[string]bool{}
	counts := map[byte]int{}
	for i := 0; i < n; i++ {
		home := homeDir(t)
		var notice bytes.Buffer
		tok, src, gen, err := ResolveTo(&notice)
		if err != nil || !gen || src != SourceGenerated {
			t.Fatalf("%q %v %v", src, gen, err)
		}
		if len(tok) != 64 || strings.Trim(tok, "0123456789abcdef") != "" {
			t.Fatalf("token %q is not 64 lowercase hex chars (256 bits)", tok)
		}
		if seen[tok] {
			t.Fatal("duplicate token")
		}
		seen[tok], prefixes[tok[:8]] = true, true
		for j := 0; j < len(tok); j++ {
			counts[tok[j]]++
		}
		if strings.Contains(notice.String(), tok) {
			t.Fatal("token in the notice")
		}
		p := filepath.Join(home, ".nself", "mcp-token")
		b, err := os.ReadFile(p)
		if err != nil || string(b) != tok+"\n" {
			t.Fatalf("file content %q err %v", b, err)
		}
		if runtime.GOOS != "windows" {
			if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
				t.Fatalf("token file mode %04o", st.Mode().Perm())
			}
			if st, _ := os.Stat(filepath.Dir(p)); st.Mode().Perm() != 0o700 {
				t.Fatalf("token dir mode %04o", st.Mode().Perm())
			}
		}
		again, src2, gen2, err := ResolveTo(io.Discard)
		if err != nil || again != tok || gen2 || src2 != SourceFile {
			t.Fatalf("second resolve %q %q %v %v", again, src2, gen2, err)
		}
	}
	if len(prefixes) != n {
		t.Fatalf("%d distinct 8-char prefixes in %d tokens", len(prefixes), n)
	}
	for _, c := range []byte("0123456789abcdef") {
		if counts[c] < 150 || counts[c] > 360 {
			t.Fatalf("hex digit %q appears %d times in %d chars (expected about 256)", c, counts[c], n*64)
		}
	}
}
