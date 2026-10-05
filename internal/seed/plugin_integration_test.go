//go:build integration

package seed

// Linux integration proof (P7-PLUG-32): a fixture plugin seed argv runs inside
// a real postgres:16 container twice and the row count is identical. The
// runtime execs through the Docker engine API on the socket, because the
// golang test container has no docker CLI.

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type engineAPI struct{ c *http.Client }

func (e engineAPI) call(method, path string, body, out any) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, "http://docker"+path, rd)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.c.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode < 300 {
		_ = json.Unmarshal(raw, out)
	}
	if resp.StatusCode >= 400 && resp.StatusCode != 404 {
		return resp.StatusCode, raw, fmt.Errorf("docker %s %s: %d %s", method, path, resp.StatusCode, raw)
	}
	return resp.StatusCode, raw, nil
}

func engineSocket() string {
	if h := os.Getenv("DOCKER_HOST"); strings.HasPrefix(h, "unix://") {
		return strings.TrimPrefix(h, "unix://")
	}
	if out, err := exec.Command("docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output(); err == nil {
		if h := strings.TrimSpace(string(out)); strings.HasPrefix(h, "unix://") {
			return strings.TrimPrefix(h, "unix://")
		}
	}
	return "/var/run/docker.sock"
}

// demux strips the 8-byte docker stream frames from a non-TTY exec output.
func demux(raw []byte) string {
	var out bytes.Buffer
	for len(raw) >= 8 {
		n := int(binary.BigEndian.Uint32(raw[4:8]))
		raw = raw[8:]
		if n > len(raw) {
			n = len(raw)
		}
		out.Write(raw[:n])
		raw = raw[n:]
	}
	return out.String()
}

// apiRuntime is a PluginRuntime over the engine API for one known container.
type apiRuntime struct {
	e  engineAPI
	id string
}

func (a apiRuntime) FindContainer(_ context.Context, _ string) (string, error) { return a.id, nil }

func (a apiRuntime) Exec(_ context.Context, container string, argv []string) (string, string, error) {
	var created struct{ Id string }
	if _, _, err := a.e.call("POST", "/containers/"+container+"/exec",
		map[string]any{"Cmd": argv, "AttachStdout": true, "AttachStderr": true}, &created); err != nil {
		return "", "", err
	}
	_, raw, err := a.e.call("POST", "/exec/"+created.Id+"/start", map[string]any{"Detach": false}, nil)
	if err != nil {
		return "", "", err
	}
	text := demux(raw)
	var info struct{ ExitCode int }
	if _, _, err := a.e.call("GET", "/exec/"+created.Id+"/json", nil, &info); err != nil {
		return text, "", err
	}
	if info.ExitCode != 0 {
		return text, text, fmt.Errorf("exit %d", info.ExitCode)
	}
	return text, "", nil
}

func startPG(t *testing.T) (engineAPI, string) {
	t.Helper()
	sock := engineSocket()
	e := engineAPI{&http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sock)
	}}}}
	code, _, err := e.call("GET", "/images/"+url.PathEscape("postgres:16")+"/json", nil, nil)
	if err != nil {
		t.Fatalf("docker at %s: %v", sock, err)
	}
	if code == 404 {
		if _, _, err := e.call("POST", "/images/create?fromImage=postgres&tag=16", nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	var created struct{ Id string }
	if _, _, err := e.call("POST", "/containers/create", map[string]any{
		"Image": "postgres:16", "Env": []string{"POSTGRES_PASSWORD=test"},
		"HostConfig": map[string]any{"AutoRemove": true},
	}, &created); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _, _ = e.call("DELETE", "/containers/"+created.Id+"?force=true", nil, nil) })
	if _, _, err := e.call("POST", "/containers/"+created.Id+"/start", nil, nil); err != nil {
		t.Fatal(err)
	}
	return e, created.Id
}

func TestPluginSeedIdempotent(t *testing.T) {
	e, id := startPG(t)
	rt := apiRuntime{e: e, id: id}
	ctx := context.Background()

	// initdb restarts postgres once; wait for two consecutive good probes.
	deadline := time.Now().Add(90 * time.Second)
	good := 0
	for good < 2 {
		if _, _, err := rt.Exec(ctx, id, []string{"psql", "-U", "postgres", "-tAc", "select 1"}); err == nil {
			good++
		} else {
			good = 0
		}
		if time.Now().After(deadline) {
			t.Fatal("postgres not ready in 90s")
		}
		time.Sleep(500 * time.Millisecond)
	}

	target := PluginTarget{Name: "fixture", Service: "postgres", Argv: []string{
		"psql", "-U", "postgres", "-v", "ON_ERROR_STOP=1", "-c",
		"CREATE TABLE IF NOT EXISTS np_fixture_items (id int PRIMARY KEY, label text); " +
			"INSERT INTO np_fixture_items VALUES (1,'a'),(2,'b'),(3,'c') ON CONFLICT (id) DO NOTHING;",
	}}
	count := func() string {
		out, _, err := rt.Exec(ctx, id, []string{"psql", "-U", "postgres", "-tAc", "select count(*) from np_fixture_items"})
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(out)
	}

	if _, err := RunPluginSeed(ctx, rt, target); err != nil {
		t.Fatal(err)
	}
	first := count()
	if _, err := RunPluginSeed(ctx, rt, target); err != nil {
		t.Fatal(err)
	}
	second := count()
	if first != "3" || second != first {
		t.Fatalf("row counts: first=%q second=%q, want 3 and 3", first, second)
	}

	// Negative proof: a failing seed surfaces as E250, not a silent pass.
	bad := PluginTarget{Name: "fixture", Service: "postgres", Argv: []string{
		"psql", "-U", "postgres", "-v", "ON_ERROR_STOP=1", "-c", "select * from no_such_table"}}
	if _, err := RunPluginSeed(ctx, rt, bad); codeOf(err) != "E250" {
		t.Fatalf("failing seed: %v", err)
	}
}
