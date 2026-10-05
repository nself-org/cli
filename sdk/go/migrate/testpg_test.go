package migrate

import (
	"bytes"
	"context"
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

	"github.com/jackc/pgx/v5/pgxpool"
)

// This file starts one throwaway postgres:16 container for the whole test
// package, talking to the Docker Engine API over its unix socket (no docker
// CLI needed, so it also works inside a golang container that has the socket
// mounted). Unreachable Docker is a failure, never a skip: a skipped
// migration test proves nothing.

const pgImage = "postgres:16"

var testDSN string

func TestMain(m *testing.M) {
	code := 1
	stop, err := startPostgres()
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate tests need Docker and postgres:16:", err)
	} else {
		code = m.Run()
		stop()
	}
	os.Exit(code)
}

func dockerSocket() (string, error) {
	if h := os.Getenv("DOCKER_HOST"); strings.HasPrefix(h, "unix://") {
		return strings.TrimPrefix(h, "unix://"), nil
	}
	if h := os.Getenv("DOCKER_HOST"); h != "" {
		return "", fmt.Errorf("DOCKER_HOST %q is not a unix socket", h)
	}
	if out, err := exec.Command("docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output(); err == nil {
		if h := strings.TrimSpace(string(out)); strings.HasPrefix(h, "unix://") {
			return strings.TrimPrefix(h, "unix://"), nil
		}
	}
	return "/var/run/docker.sock", nil
}

type engine struct{ c *http.Client }

func (e engine) call(method, path string, body any, out any) (int, error) {
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

func startPostgres() (stop func(), err error) {
	sock, err := dockerSocket()
	if err != nil {
		return nil, err
	}
	e := engine{&http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", sock)
	}}}}
	if code, err := e.call("GET", "/images/"+url.PathEscape(pgImage)+"/json", nil, nil); err != nil {
		return nil, fmt.Errorf("docker at %s: %w", sock, err)
	} else if code == 404 {
		if _, err := e.call("POST", "/images/create?fromImage=postgres&tag=16", nil, nil); err != nil {
			return nil, err
		}
	}
	var created struct{ Id string }
	_, err = e.call("POST", "/containers/create", map[string]any{
		"Image": pgImage,
		"Env":   []string{"POSTGRES_PASSWORD=test"},
		"Cmd":   []string{"postgres", "-c", "fsync=off", "-c", "max_connections=200"},
		"HostConfig": map[string]any{"AutoRemove": true, "PortBindings": map[string]any{
			"5432/tcp": []map[string]string{{"HostIp": "127.0.0.1", "HostPort": ""}}}},
	}, &created)
	if err != nil {
		return nil, err
	}
	stop = func() { _, _ = e.call("DELETE", "/containers/"+created.Id+"?force=true", nil, nil) }
	if _, err = e.call("POST", "/containers/"+created.Id+"/start", nil, nil); err != nil {
		stop()
		return nil, err
	}
	var info struct {
		NetworkSettings struct {
			IPAddress string
			Ports     map[string][]struct{ HostPort string }
		}
	}
	if _, err = e.call("GET", "/containers/"+created.Id+"/json", nil, &info); err != nil {
		stop()
		return nil, err
	}
	// Two ways to reach the container: the published port on loopback (host
	// runs, Colima, Linux CI) or its bridge IP (this test running inside a
	// sibling container that has the Docker socket mounted).
	var candidates []string
	if ports := info.NetworkSettings.Ports["5432/tcp"]; len(ports) > 0 {
		candidates = append(candidates, "127.0.0.1:"+ports[0].HostPort)
	}
	if ip := info.NetworkSettings.IPAddress; ip != "" {
		candidates = append(candidates, ip+":5432")
	}
	deadline := time.Now().Add(90 * time.Second)
	var perr error
	for {
		for _, addr := range candidates {
			dsn := "postgres://postgres:test@" + addr + "/postgres?sslmode=disable"
			if perr = pingTwice(dsn); perr == nil {
				testDSN = dsn
				return stop, nil
			}
		}
		if time.Now().After(deadline) {
			stop()
			return nil, fmt.Errorf("postgres not ready: %w", perr)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// pingTwice pings, waits, and pings again: the image's initdb restarts
// postgres once before it accepts real connections.
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

// newPool returns a pool on the shared container.
func newPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// schemaFor returns a schema unique to the test and drops it afterwards.
func schemaFor(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	s := "np_t_" + strings.ToLower(strings.NewReplacer("/", "_", " ", "_", "-", "_").Replace(t.Name()))
	if len(s) > 60 {
		s = s[:60]
	}
	drop := func() { _, _ = pool.Exec(context.Background(), `DROP SCHEMA IF EXISTS "`+s+`" CASCADE`) }
	drop()
	t.Cleanup(drop)
	return s
}

func count(t *testing.T, pool *pgxpool.Pool, q string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return n
}
