package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/spf13/cobra"
)

const doc = "{\"schema_version\":\"1\",\"command\":\"config get\",\"data\":{\"value\":\"local.nself.org\"}}\n"
const streamRecord = "{\"schema_version\":\"1\",\"command\":\"stream\",\"data\":{\"type\":\"log\",\"record\":1}}\n"
const streamResult = "{\"schema_version\":\"1\",\"command\":\"stream\",\"data\":{\"type\":\"result\",\"ok\":true}}\n"

func TestMain(m *testing.M) {
	if os.Getenv("HTTPAPI_STUB") == "1" {
		switch os.Getenv("HTTPAPI_MODE") {
		case "block":
			_ = os.WriteFile(filepath.Join(os.Getenv("HTTPAPI_PID"), strconv.Itoa(os.Getpid())), []byte("ready"), 0600)
			time.Sleep(30 * time.Second)
		case "stream":
			fmt.Print(streamRecord)
			fmt.Print(streamResult)
		case "disconnect":
			signal.Ignore(syscall.SIGTERM)
			_ = os.WriteFile(os.Getenv("HTTPAPI_PID"), []byte(strconv.Itoa(os.Getpid())), 0600)
			fmt.Print(streamRecord)
			for {
				fmt.Print(streamRecord)
				time.Sleep(10 * time.Millisecond)
			}
		default:
			fmt.Print(doc)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fixture(t *testing.T) *cmdregistry.Registry {
	t.Helper()
	root := &cobra.Command{Use: "nself"}
	config := &cobra.Command{Use: "config"}
	root.AddCommand(config)
	get := &cobra.Command{Use: "get [key]", Run: func(*cobra.Command, []string) {}}
	config.AddCommand(get)
	for _, name := range []string{"logs", "stream", "hidden", "cli-only", "destroy", "interactive"} {
		c := &cobra.Command{Use: name, Run: func(*cobra.Command, []string) {}}
		if name == "logs" {
			c.Flags().Bool("follow", false, "")
		}
		if name == "destroy" {
			c.Flags().Bool("yes", false, "")
		}
		if name == "hidden" {
			c.Hidden = true
		}
		root.AddCommand(c)
	}
	f, err := canon.Parse([]byte(`schema_version: 1
verbs: [config, logs, stream, hidden, cli-only, destroy, interactive]
commands:
  config: {canon: core}
  config get: {side_effect: read}
  logs: {canon: core, side_effect: read, flags: {follow: {output: stream}}}
  stream: {canon: core, side_effect: read, output: stream}
  hidden: {canon: core, side_effect: read}
  cli-only: {canon: core, side_effect: read, surface: cli-only}
  destroy: {canon: core, side_effect: destructive, confirm: {flags: [yes]}}
  interactive: {canon: core, side_effect: read, output: interactive}
`))
	if err != nil {
		t.Fatal(err)
	}
	r, err := cmdregistry.Build(root, f, map[string]any{"config get": map[string]any{}, "logs": map[string]any{}, "stream": map[string]any{}, "hidden": map[string]any{}, "cli-only": map[string]any{}, "destroy": map[string]any{}, "interactive": map[string]any{}}, cmdregistry.BuildOptions{V15: true})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func server(t *testing.T) *Server {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HTTPAPI_STUB", "1")
	t.Setenv("HTTPAPI_MODE", "")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(invoke.SelfExecOverrideEnv, exe)
	s, err := New(Options{Registry: fixture(t), Port: 3857, ProjectDir: t.TempDir(), Auth: BearerAuth("fake-token-0000"), Log: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func call(t *testing.T, s *Server, path, body string, alter func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("POST", "http://localhost:3857"+path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer fake-token-0000")
	r.Header.Set("Content-Type", "application/json")
	if alter != nil {
		alter(r)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	noCORS(t, w.Header())
	return w
}
func noCORS(t *testing.T, h http.Header) {
	t.Helper()
	for k := range h {
		if strings.HasPrefix(strings.ToLower(k), "access-control-") {
			t.Fatal("CORS header", k)
		}
	}
}
func expect(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if w.Code != status || (code != "" && !strings.Contains(w.Body.String(), `"code":"`+code+`"`) && !strings.Contains(w.Body.String(), `"code": "`+code+`"`)) {
		t.Fatalf("status %d body %s; wanted %d %s", w.Code, w.Body, status, code)
	}
}

func TestRoutes(t *testing.T) {
	s := server(t)
	for _, prefix := range []string{"/{rest...}", "/{x}/", "/v1/", "/openapi.json", "GET /mounted", "/%76%31/", "/%6fpenapi.json"} {
		if err := s.Mount(prefix, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})); err == nil {
			t.Fatal("unsafe mount accepted", prefix)
		}
	}
	w := call(t, s, "/v1/commands/config/get", `{"args":["BASE_DOMAIN"]}`, nil)
	expect(t, w, 200, "")
	if w.Body.String() != doc || headerValue(w.Header(), "X-Nself-Exit-Code") != "0" || len(headerValue(w.Header(), "X-Nself-Request-Id")) != 64 {
		t.Fatal(w.Body.String(), w.Header())
	}
	for _, tc := range []struct {
		modify func(*http.Request)
		status int
	}{{func(r *http.Request) { r.Method = "GET" }, 405}, {func(r *http.Request) { r.Header.Set("Content-Type", "text/plain") }, 415}, {func(r *http.Request) { r.Header.Set("Accept", "text/plain") }, 406}} {
		expect(t, call(t, s, "/v1/commands/config/get", "{}", tc.modify), tc.status, "E430")
	}
	expect(t, call(t, s, "/v1/commands/config/get", `{"wat":1}`, nil), 400, "E420")
	expect(t, call(t, s, "/v1/commands/config/get", "not json", nil), 400, "E430")
	expect(t, call(t, s, "/v1/commands/destroy", "{}", nil), 428, "")
}
func TestRouteNotExposedVsUnknown(t *testing.T) {
	s := server(t)
	for _, p := range []string{"hidden", "cli-only", "interactive"} {
		expect(t, call(t, s, "/v1/commands/"+p, "{}", nil), 404, "E421")
	}
	expect(t, call(t, s, "/v1/commands/nope", "{}", nil), 404, "E432")
}
func TestHostAllowlist(t *testing.T) {
	s := server(t)
	for _, host := range []string{"localhost:3857", "LOCALHOST:3857", "127.0.0.1:3857", "[::1]:3857"} {
		expect(t, call(t, s, "/v1/commands/config/get", "{}", func(r *http.Request) { r.Host = host }), 200, "")
	}
	expect(t, call(t, s, "/v1/commands/config/get", "{}", func(r *http.Request) { r.Host = "evil.example" }), 421, "E428")
}
func TestOriginCheck(t *testing.T) {
	s := server(t)
	for _, origin := range []string{"", "null", "http://evil.example", "https://localhost:3857", "http://localhost:3857/"} {
		expect(t, call(t, s, "/v1/commands/config/get", "{}", func(r *http.Request) { r.Header.Set("Origin", origin) }), 403, "E429")
	}
	expect(t, call(t, s, "/v1/commands/config/get", "{}", func(r *http.Request) { r.Header.Set("Origin", "http://localhost:3857") }), 200, "")
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("body read before authentication") }
func TestAuthRequired(t *testing.T) {
	s := server(t)
	w := call(t, s, "/v1/commands/config/get", "{}", func(r *http.Request) { r.Header.Del("Authorization"); r.Body = io.NopCloser(panicReader{}) })
	expect(t, w, 401, "E426")
	if headerValue(w.Header(), "WWW-Authenticate") != `Bearer realm="nself"` {
		t.Fatal(w.Header())
	}
	s.o.Auth = BearerAuth("")
	expect(t, call(t, s, "/v1/commands/config/get", "{}", nil), 401, "E426")
	if err := s.Mount("/mounted", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })); err != nil {
		t.Fatal(err)
	}
	expect(t, call(t, s, "/mounted", "{}", nil), 401, "E426")
}
func TestBodyLimit(t *testing.T) {
	s := server(t)
	expect(t, call(t, s, "/v1/commands/config/get", strings.Repeat(" ", 2<<20), nil), 413, "E430")
}
func TestOpenAPIServed(t *testing.T) {
	s := server(t)
	w := call(t, s, "/openapi.json", "", func(r *http.Request) { r.Method = "GET" })
	expect(t, w, 200, "")
	want, err := OpenAPI(s.o.Registry, nil)
	if err != nil || !bytes.Equal(w.Body.Bytes(), want) {
		t.Fatal("OpenAPI mismatch", err)
	}
	expect(t, call(t, s, "/openapi.json", "", func(r *http.Request) { r.Method = "GET"; r.Header.Del("Authorization") }), 401, "E426")
}
func TestStatusMapping(t *testing.T) {
	for _, tc := range []struct {
		code   string
		status int
	}{{"E420", 400}, {"E425", 400}, {"E430", 400}, {"E426", 401}, {"E429", 403}, {"E421", 404}, {"E432", 404}, {"E424", 409}, {"E460", 409}, {"E428", 421}, {"E431", 503}, {"E423", 504}} {
		t.Run(tc.code, func(t *testing.T) {
			r := invoke.Result{Stdout: []byte(fmt.Sprintf(`{"error":{"code":%q,"class":"user"}}`, tc.code))}
			if status(r) != tc.status {
				t.Fatal(status(r))
			}
			w := httptest.NewRecorder()
			document(w, r, tc.status)
			noCORS(t, w.Header())
			if tc.status == 503 && headerValue(w.Header(), "Retry-After") != "1" {
				t.Fatal(w.Header())
			}
		})
	}
	for _, tc := range []struct {
		class string
		want  int
	}{{"user", 422}, {"auth", 403}, {"destructive_blocked", 409}, {"infra", 502}, {"other", 500}} {
		if got := status(invoke.Result{Stdout: []byte(fmt.Sprintf(`{"error":{"class":%q}}`, tc.class))}); got != tc.want {
			t.Fatal(tc, got)
		}
	}
	if status(invoke.Result{Stdout: []byte(`{"data":{"confirmation_required":true}}`)}) != 428 || status(invoke.Result{Stdout: []byte(doc), ExitCode: 10}) != 200 {
		t.Fatal("data status")
	}
}

func TestStreamFlagHidden(t *testing.T) {
	s := server(t)
	b, err := OpenAPI(s.o.Registry, nil)
	if err != nil {
		t.Fatal(err)
	}
	var api map[string]any
	if err = json.Unmarshal(b, &api); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(b, []byte(`"follow"`)) {
		t.Fatal("stream override leaked", string(b))
	}
	expect(t, call(t, s, "/v1/commands/logs", `{"flags":{"follow":true}}`, nil), 404, "E421")
	t.Setenv("HTTPAPI_MODE", "stream")
	w := call(t, s, "/v1/commands/logs", `{"flags":{"follow":true}}`, func(r *http.Request) { r.Header.Set("Accept", "application/x-ndjson") })
	expect(t, w, 200, "")
	if headerValue(w.Header(), "Content-Type") != "application/x-ndjson" {
		t.Fatal(w.Header())
	}
}

func TestSaturation(t *testing.T) {
	s := server(t)
	t.Setenv("HTTPAPI_MODE", "block")
	pidDir := t.TempDir()
	t.Setenv("HTTPAPI_PID", pidDir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{}, 4)
	for i := 0; i < 4; i++ {
		go func() {
			call(t, s, "/v1/commands/config/get", "{}", func(r *http.Request) { *r = *r.WithContext(ctx) })
			done <- struct{}{}
		}()
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		entries, err := os.ReadDir(pidDir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) == 4 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("four children not started")
		}
		time.Sleep(5 * time.Millisecond)
	}
	w := call(t, s, "/v1/commands/config/get", "{}", nil)
	expect(t, w, 503, "E431")
	if headerValue(w.Header(), "Retry-After") != "1" {
		t.Fatal(w.Header())
	}
	cancel()
	for i := 0; i < 4; i++ {
		<-done
	}
}

func TestNDJSONStream(t *testing.T) {
	s := server(t)
	t.Setenv("HTTPAPI_MODE", "stream")
	w := call(t, s, "/v1/commands/stream", "{}", func(r *http.Request) { r.Header.Set("Accept", "application/x-ndjson") })
	expect(t, w, 200, "")
	if w.Body.String() != streamRecord+streamResult || headerValue(w.Result().Trailer, "X-Nself-Exit-Code") != "0" {
		t.Fatal(w.Body.String(), w.Result().Trailer)
	}
	if runtime.GOOS == "windows" {
		t.Skip("PID signal probe requires Unix")
	}
	t.Setenv("HTTPAPI_MODE", "disconnect")
	pidfile := filepath.Join(t.TempDir(), "pid")
	t.Setenv("HTTPAPI_PID", pidfile)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	req, _ := http.NewRequest("POST", ts.URL+"/v1/commands/stream", strings.NewReader("{}"))
	req.Host = "localhost:3857"
	req.Header.Set("Authorization", "Bearer fake-token-0000")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson")
	res, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	noCORS(t, res.Header)
	buf := make([]byte, 1)
	_, _ = res.Body.Read(buf)
	_ = res.Body.Close()
	b, err := os.ReadFile(pidfile)
	if err != nil {
		t.Fatal(err)
	}
	pid, _ := strconv.Atoi(string(b))
	process, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for process.Signal(syscall.Signal(0)) == nil {
		if time.Now().After(deadline) {
			t.Fatal("child still alive", pid)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestServerTimeouts(t *testing.T) {
	s := server(t)
	if s.o.ReadHeaderTimeout != 10*time.Second || s.o.ReadTimeout != 30*time.Second || s.o.IdleTimeout != 60*time.Second {
		t.Fatal(s.o)
	}
	s.o.ReadTimeout = 80 * time.Millisecond
	s.o.ReadHeaderTimeout = 80 * time.Millisecond
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- s.ListenAndServe(ctx, addr) }()
	deadline := time.Now().Add(3 * time.Second)
	var conn net.Conn
	for {
		conn, err = net.Dial("tcp", addr)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	defer conn.Close()
	_, _ = fmt.Fprint(conn, "POST /v1/commands/config/get HTTP/1.1\r\nHost: localhost:3857\r\nAuthorization: Bearer fake-token-0000\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n{")
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if n == 0 {
		t.Fatal("stalled body not refused", err)
	}
	if !bytes.Contains(buf[:n], []byte("400")) {
		t.Fatal(string(buf[:n]))
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("shutdown hung")
	}
}
