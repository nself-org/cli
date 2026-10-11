package admin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/httpapi"
	"github.com/nself-org/cli/internal/machineauth"
	"github.com/nself-org/cli/internal/ports"
)

func TestAdminServe(t *testing.T) {
	for _, configured := range []bool{false, true} {
		name := "generated"
		if configured {
			name = "configured"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv(machineauth.EnvVar, "")
			if configured {
				t.Setenv(machineauth.EnvVar, "fake-token-0000")
			}
			dir := t.TempDir()
			var log bytes.Buffer
			calls, listens := 0, 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cmd := ServeCommand(ServeDeps{
				ProjectDir: func() (string, error) { return dir, nil },
				Registry: func() (*cmdregistry.Registry, error) {
					calls++
					return &cmdregistry.Registry{SchemaVersion: "1"}, nil
				},
				Listen: func(network, addr string) (net.Listener, error) {
					listens++
					if addr != "127.0.0.1:3857" {
						t.Fatalf("listen address = %s", addr)
					}
					if strings.Contains(log.String(), "listening on") {
						t.Fatal("readiness announced before binding")
					}
					ln, err := net.Listen(network, "127.0.0.1:0")
					cancel()
					return ln, err
				},
			})
			cmd.SetContext(ctx)
			cmd.SetErr(&log)
			if cmd.Flags().Lookup("port").DefValue != "3857" || ports.AdminAPIPort != 3857 {
				t.Fatal("incorrect default API port")
			}
			if cmd.Flags().Lookup("timeout").DefValue != "5m0s" {
				t.Fatal("incorrect timeout default")
			}
			cmd.SetArgs(nil)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || listens != 1 {
				t.Fatalf("registry/listen calls = %d/%d", calls, listens)
			}
			if !strings.Contains(log.String(), "listening on") {
				t.Fatal("bound listener was not announced")
			}
			tok, _, err := machineauth.Lookup()
			if err != nil || tok == "" {
				t.Fatalf("missing machine token: %v", err)
			}
			if strings.Contains(log.String(), tok) {
				t.Fatal("stderr leaked token")
			}
			s, err := httpapi.New(httpapi.Options{Registry: &cmdregistry.Registry{SchemaVersion: "1"}, ProjectDir: dir, Port: 3857, Auth: httpapi.BearerAuth(tok), Log: &log})
			if err != nil {
				t.Fatal(err)
			}
			assertServeAuth(t, s)
			if !configured && !strings.Contains(log.String(), filepath.Join(".nself", "mcp-token")) {
				t.Fatal("generated token file path was not printed")
			}
		})
	}
	t.Run("project-required", func(t *testing.T) {
		cmd := ServeCommand(ServeDeps{
			ProjectDir: func() (string, error) { return "", errors.New("no nself project") },
			Registry:   func() (*cmdregistry.Registry, error) { t.Fatal("registry built outside project"); return nil, nil },
		})
		cmd.SetArgs(nil)
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "no nself project") {
			t.Fatalf("project refusal = %v", err)
		}
	})
	t.Run("occupied-port", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = ln.Close() }()
		t.Setenv(machineauth.EnvVar, "fake-token-0000")
		var log bytes.Buffer
		cmd := ServeCommand(ServeDeps{
			ProjectDir: func() (string, error) { return t.TempDir(), nil },
			Registry: func() (*cmdregistry.Registry, error) {
				return &cmdregistry.Registry{SchemaVersion: "1"}, nil
			},
		})
		cmd.SetErr(&log)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetArgs([]string{"--port", fmt.Sprint(ln.Addr().(*net.TCPAddr).Port)})
		if err := cmd.Execute(); err == nil {
			t.Fatal("occupied port did not return an error")
		}
		if strings.Contains(log.String(), "listening on") {
			t.Fatalf("failed bind announced readiness: %q", log.String())
		}
	})
}

func assertServeAuth(t *testing.T, s *httpapi.Server) {
	t.Helper()
	for _, authorized := range []bool{false, true} {
		r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:3857/openapi.json", nil)
		if authorized {
			tok, _, err := machineauth.Lookup()
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set("Authorization", "Bearer "+tok)
		}
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		want := http.StatusUnauthorized
		if authorized {
			want = http.StatusOK
		}
		if w.Code != want {
			t.Fatalf("authenticated=%t: status %d, body %s", authorized, w.Code, w.Body.String())
		}
		for name := range w.Header() {
			if strings.HasPrefix(strings.ToLower(name), "access-control-") {
				t.Fatalf("unexpected CORS header %s", name)
			}
		}
	}
}

func TestNonLoopbackBind(t *testing.T) {
	strong := "fake-token-00000000000000000000000"
	for _, tc := range []struct {
		name, token string
		allowed     []string
		accepted    bool
		file, weak  bool
	}{
		{"neither", "", nil, false, false, false},
		{"allowed-only", "", []string{"api.example.test"}, false, false, false},
		{"token-only", strong, nil, false, false, false},
		{"explicit", strong, []string{"api.example.test"}, true, false, false},
		{"short-env", "fake-token-0000", []string{"api.example.test"}, false, false, true},
		{"31-bytes", strings.Repeat("x", 31), []string{"api.example.test"}, false, false, true},
		{"32-bytes", strings.Repeat("x", 32), []string{"api.example.test"}, true, false, false},
		{"space-env", strong + " ", []string{"api.example.test"}, false, false, true},
		{"unicode-space-env", strong + "\u2003", []string{"api.example.test"}, false, false, true},
		{"short-file", "fake-token-0000", []string{"api.example.test"}, false, true, true},
		{"whitespace-file", strong + "\tbad", []string{"api.example.test"}, false, true, true},
		{"strong-file", strong, []string{"api.example.test"}, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv(machineauth.EnvVar, "")
			path, err := machineauth.TokenPath()
			if err != nil {
				t.Fatal(err)
			}
			if tc.file {
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(tc.token+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				t.Setenv(machineauth.EnvVar, tc.token)
			}
			for i := 0; i < 2; i++ {
				var log bytes.Buffer
				built, listened := false, false
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				cmd := ServeCommand(ServeDeps{
					ProjectDir: func() (string, error) { return t.TempDir(), nil },
					Registry: func() (*cmdregistry.Registry, error) {
						built = true
						return &cmdregistry.Registry{SchemaVersion: "1"}, nil
					},
					Listen: func(network, _ string) (net.Listener, error) {
						listened = true
						ln, err := net.Listen(network, "127.0.0.1:0")
						cancel()
						return ln, err
					},
				})
				cmd.SetContext(ctx)
				cmd.SetErr(&log)
				args := []string{"--host", "0.0.0.0"}
				for _, allowed := range tc.allowed {
					args = append(args, "--allowed-host", allowed)
				}
				cmd.SetArgs(args)
				err := cmd.Execute()
				if tc.accepted {
					if err != nil || !built || !listened || strings.Count(log.String(), "cleartext HTTP") != 1 {
						t.Fatalf("accepted bind: %v %q", err, log.String())
					}
				} else {
					var e *errs.CLIError
					if !errors.As(err, &e) || e.Code != "E426" {
						t.Fatalf("refusal = %v", err)
					}
					if tc.weak && (!strings.Contains(e.Fix, path) || !strings.Contains(e.Fix, "nself admin serve on loopback")) {
						t.Fatalf("weak token guidance = %q", e.Fix)
					}
					if built || listened {
						t.Fatal("refused bind reached registry or listener")
					}
				}
				if tc.token != "" && strings.Contains(log.String(), tc.token) {
					t.Fatal("warning leaked token")
				}
				if _, err := os.Stat(path); !tc.file && !os.IsNotExist(err) {
					t.Fatalf("non-loopback run created token file: %v", err)
				}
			}
		})
	}
}

func TestServeAllowedHostValidation(t *testing.T) {
	for _, host := range []string{"myhost:3857", "http://myhost", "a/b", "my host", " myhost", "myhost ", "myhost\nignored", ""} {
		t.Run(host, func(t *testing.T) {
			cmd := ServeCommand(ServeDeps{
				ProjectDir: func() (string, error) { t.Fatal("invalid host reached project lookup"); return "", nil },
				Registry:   func() (*cmdregistry.Registry, error) { t.Fatal("invalid host reached registry"); return nil, nil },
			})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs([]string{"--allowed-host", "api.example.test", "--allowed-host", host})
			err := cmd.Execute()
			var cliErr *errs.CLIError
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%q", host)) {
				t.Fatalf("invalid allowed host %q: %v", host, err)
			}
			if errors.As(err, &cliErr) && cliErr.Code != "E430" {
				t.Fatalf("invalid allowed host code: %v", err)
			}
			if errors.As(err, &cliErr) && (cliErr.Why != "an allowed host is a DNS name, an IPv4 literal or a bracketed IPv6 literal, without scheme, port or path" || cliErr.Fix != "pass --allowed-host <name>; the server adds the port itself") {
				t.Fatalf("invalid allowed host guidance: %+v", cliErr)
			}
		})
	}
}
