//go:build integration

package readiness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/cli/sdk/go/v2/migrate"
)

// Linux integration proof (Constitution 10.3, P7-PLUG-59): a fixture plugin
// built on sdk/go/migrate serves /health from a real postgres:16 container and
// applies its migrations after a delay (a slow boot). readiness.Wait must
// return nil only once applied == expected. Docker unreachable is a failure.

const pgImage = "postgres:16"

type engine struct{ c *http.Client }

func (e engine) call(method, path string, body, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, "http://docker"+path, rd)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode < 300 {
		_ = json.Unmarshal(raw, out)
	}
	if resp.StatusCode >= 400 && resp.StatusCode != 404 {
		return resp.StatusCode, fmt.Errorf("docker %s %s: %d %s", method, path, resp.StatusCode, raw)
	}
	return resp.StatusCode, nil
}

func dockerSocket() string {
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

// startPostgres runs a throwaway postgres:16 and returns a ready DSN.
func startPostgres(t *testing.T) string {
	t.Helper()
	sock := dockerSocket()
	e := engine{&http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sock)
	}}}}
	code, err := e.call("GET", "/images/"+url.PathEscape(pgImage)+"/json", nil, nil)
	if err != nil {
		t.Fatalf("docker at %s: %v", sock, err)
	}
	if code == 404 {
		if _, err := e.call("POST", "/images/create?fromImage=postgres&tag=16", nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	var created struct{ Id string }
	if _, err := e.call("POST", "/containers/create", map[string]any{
		"Image": pgImage, "Env": []string{"POSTGRES_PASSWORD=test"},
		"HostConfig": map[string]any{"AutoRemove": true, "PortBindings": map[string]any{
			"5432/tcp": []map[string]string{{"HostIp": "127.0.0.1", "HostPort": ""}}}},
	}, &created); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = e.call("DELETE", "/containers/"+created.Id+"?force=true", nil, nil) })
	if _, err := e.call("POST", "/containers/"+created.Id+"/start", nil, nil); err != nil {
		t.Fatal(err)
	}
	var info struct {
		NetworkSettings struct {
			Ports    map[string][]struct{ HostPort string }
			Networks map[string]struct{ IPAddress string }
		}
	}
	if _, err := e.call("GET", "/containers/"+created.Id+"/json", nil, &info); err != nil {
		t.Fatal(err)
	}
	var addrs []string
	if p := info.NetworkSettings.Ports["5432/tcp"]; len(p) > 0 {
		addrs = append(addrs, "127.0.0.1:"+p[0].HostPort)
	}
	for _, n := range info.NetworkSettings.Networks {
		if n.IPAddress != "" {
			addrs = append(addrs, n.IPAddress+":5432")
		}
	}
	deadline := time.Now().Add(90 * time.Second)
	for {
		for _, a := range addrs {
			dsn := "postgres://postgres:test@" + a + "/postgres?sslmode=disable"
			if pingTwice(dsn) == nil {
				return dsn
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("postgres not ready in 90s")
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// pingTwice: the image's initdb restarts postgres once before real connections work.
func pingTwice(dsn string) error {
	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		p, err := pgxpool.New(ctx, dsn)
		if err == nil {
			err = p.Ping(ctx)
			p.Close()
		}
		cancel()
		if err != nil {
			return err
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

func TestReadinessIntegrationPostgres(t *testing.T) {
	dsn := startPostgres(t)
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "CREATE SCHEMA np_fixture"); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	for i, sql := range []string{
		"CREATE TABLE np_fixture.a (id int primary key);",
		"CREATE TABLE np_fixture.b (id int primary key);",
		"CREATE TABLE np_fixture.c (id int primary key);",
	} {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("00%d_t.sql", i+1)), []byte(sql), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	opts := migrate.Options{Schema: "np_fixture", Dir: dir}

	// The fixture plugin: /health serves migrate.Status as the contract requires.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st, serr := migrate.Status(r.Context(), pool, opts)
		if serr != nil {
			http.Error(w, serr.Error(), http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintf(w, `{"status":"ok",%s}`, migrate.HealthField(st))
	}))
	defer srv.Close()

	// Before boot-apply the plugin is behind: the proof must see that state.
	before, err := Probe(ctx, nil, "fixture", srv.URL)
	if err != nil || before.Ready() || before.Expected != 3 {
		t.Fatalf("before apply: %+v, %v (want applied < expected = 3)", before, err)
	}

	applyErr := make(chan error, 1)
	go func() { // slow boot: the plugin applies its SQL a few seconds after start
		time.Sleep(3 * time.Second)
		_, aerr := migrate.Apply(ctx, pool, opts)
		applyErr <- aerr
	}()

	if err := Wait(ctx, "fixture", srv.URL, 60*time.Second); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if aerr := <-applyErr; aerr != nil {
		t.Fatalf("Apply: %v", aerr)
	}
	after, err := Probe(ctx, nil, "fixture", srv.URL)
	if err != nil || !after.Ready() || after.Applied != 3 {
		t.Fatalf("after Wait: %+v, %v", after, err)
	}
}
