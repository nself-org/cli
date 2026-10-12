package compose

// cache_service_test.go — tests for the cache engine switch (P7-LIVE-19, EPIC D16).
//
// Purpose: prove the engine precedence (explicit REDIS_ENGINE > project lock pin
// > mode default) in both compat modes, that an existing project is never moved
// implicitly, and (with Docker) that a Valkey service starts, passes its
// health check and serves SET/GET on the REDIS_URL port.
// Constraints: the matrix is hermetic (no network); TestValkeyIntegration needs
// a reachable Docker daemon and pulls the locked valkey image, and skips without one.

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
)

// cacheCfg is a Redis-enabled config with the given engine ("" = not set).
func cacheCfg(engine, version, password string) *config.Config {
	cfg := minimalCfg()
	cfg.Redis = config.RedisConfig{Enabled: true, Engine: engine, Version: version, Port: 6379, Password: password}
	return cfg
}

// svcFacts reduces the redis service to what the matrix compares.
type svcFacts struct{ engine, image, command, volume string }

func factsOf(t *testing.T, cfg *config.Config) svcFacts {
	t.Helper()
	g := NewGenerator(cfg)
	svc := g.buildRedisService()
	f := svcFacts{engine: g.cacheEngine(), image: svc.Image, command: serverBinary(svc.Command), volume: strings.SplitN(svc.Volumes[0], ":", 2)[0]}
	if _, ok := g.buildVolumes()[f.volume]; !ok {
		t.Errorf("volume %s of the redis service is not declared in the compose volumes", f.volume)
	}
	other := map[string]string{"valkey_data": "redis_data", "redis_data": "valkey_data"}[f.volume]
	if _, ok := g.buildVolumes()[other]; ok {
		t.Errorf("volume %s declared next to %s: one engine, one data volume", other, f.volume)
	}
	return f
}

// serverBinary names the server binary a service command starts: the command is a
// string without a password and sh -c "exec <server> ..." with one (D-0282).
func serverBinary(command interface{}) string {
	var text string
	switch c := command.(type) {
	case string:
		text = c
	case []string:
		text = strings.Join(c, " ")
	}
	if _, after, ok := strings.Cut(text, "exec "); ok {
		text = after
	}
	return strings.Fields(text)[0]
}

// TestCacheEngineMatrix covers the D16 precedence in both compat modes.
func TestCacheEngineMatrix(t *testing.T) {
	redisPin := "docker.io/library/redis:7-alpine@sha256:" + strings.Repeat("a", 64)
	valkeyPin := "docker.io/valkey/valkey:8.1-alpine@sha256:" + strings.Repeat("b", 64)
	vk, _ := LockedRef("valkey")
	rd, _ := LockedRef("redis")
	for _, tc := range []struct {
		name      string
		v15       bool
		engine    string
		pin       string // "" = no pin
		want      string
		wantImage string // lock mode image of the want engine, "" = skip
	}{
		// v1.5 mode default, no lock, no existing redis: valkey.
		{"v15 new project", true, "", "", EngineValkey, vk.String()},
		// An existing project's pin wins over the mode default.
		{"v15 lock pins redis", true, "", redisPin, EngineRedis, ""},
		{"v15 lock pins valkey", true, "", valkeyPin, EngineValkey, ""},
		// Bootstrap of a project with no lock: the build pins the redis image of
		// its existing docker-compose.yml (a tag-only entry here).
		{"v15 compose runs redis, no lock", true, "", "redis:7-alpine", EngineRedis, ""},
		// The explicit key beats the mode and the pin, in both modes.
		{"v15 explicit redis", true, "redis", "", EngineRedis, rd.String()},
		{"v15 explicit redis over valkey pin", true, "redis", valkeyPin, EngineRedis, ""},
		{"v15 explicit valkey over redis pin", true, "valkey", redisPin, EngineValkey, ""},
		{"v14 explicit valkey", false, "valkey", "", EngineValkey, vk.String()},
		{"v14 explicit valkey over redis pin", false, "valkey", redisPin, EngineValkey, ""},
		// v1.4 default stays redis (a patch release moves nothing).
		{"v14 default", false, "", "", EngineRedis, rd.String()},
		{"v14 lock pins valkey", false, "", valkeyPin, EngineValkey, ""},
		{"v14 explicit redis", false, "redis", "", EngineRedis, rd.String()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			compattest.Set(t, tc.v15)
			setMode(t, PinningLock)
			if tc.pin != "" {
				t.Cleanup(SetProjectPins(map[string]string{"redis": tc.pin}))
			} else {
				t.Cleanup(SetProjectPins(nil))
			}
			f := factsOf(t, cacheCfg(tc.engine, "", "s3cretpassw0rdvalue"))
			if f.engine != tc.want {
				t.Fatalf("engine = %s, want %s (image %s)", f.engine, tc.want, f.image)
			}
			wantCmd, wantVol := "redis-server", "redis_data"
			if tc.want == EngineValkey {
				wantCmd, wantVol = "valkey-server", "valkey_data"
			}
			if f.command != wantCmd || f.volume != wantVol {
				t.Errorf("command %q volume %q, want %q %q", f.command, f.volume, wantCmd, wantVol)
			}
			if !strings.Contains(strings.ToLower(f.image), tc.want) {
				t.Errorf("image %q does not belong to engine %s", f.image, tc.want)
			}
			if tc.wantImage != "" && f.image != tc.wantImage {
				t.Errorf("image = %q, want the lock ref %q", f.image, tc.wantImage)
			}
		})
	}
}

// TestCacheEngineExistingProjectNeverMoves is the negative control: an existing
// project that sets REDIS_* and has a redis lock pin or no key in v1.4 renders the
// same service it rendered before this ticket, in both modes, whatever the CLI default.
func TestCacheEngineExistingProjectNeverMoves(t *testing.T) {
	pin := "docker.io/library/redis:6.2-alpine@sha256:" + strings.Repeat("c", 64)
	compattest.Both(t, func(t *testing.T) {
		setMode(t, PinningLegacy)
		t.Cleanup(SetProjectPins(map[string]string{"redis": pin}))
		cfg := cacheCfg("", "6.2-alpine", "s3cretpassw0rdvalue")
		cfg.Redis.Port, cfg.Redis.PoolSize, cfg.Redis.Memory = 6390, 20, "256M"
		g := NewGenerator(cfg)
		svc := g.buildRedisService()
		if svc.Image != pin {
			t.Errorf("image = %q, want the lock pin %q", svc.Image, pin)
		}
		wantCmd := []string{"sh", "-c", `exec redis-server --appendonly yes --protected-mode yes --requirepass "$$REDISCLI_AUTH" --maxclients 20`}
		if got, _ := svc.Command.([]string); strings.Join(got, "\x00") != strings.Join(wantCmd, "\x00") {
			t.Errorf("command = %q, want %q", svc.Command, wantCmd)
		}
		if svc.Environment["REDISCLI_AUTH"] != "${REDIS_PASSWORD}" {
			t.Errorf("REDISCLI_AUTH = %q, want ${REDIS_PASSWORD}", svc.Environment["REDISCLI_AUTH"])
		}
		if svc.Volumes[0] != "redis_data:/data" || svc.Ports[0] != "127.0.0.1:6390:6379" || svc.User != "999:999" {
			t.Errorf("volume/port/user changed: %v %v %s", svc.Volumes, svc.Ports, svc.User)
		}
		if got := svc.Healthcheck.Test; strings.Join(got, " ") != "CMD redis-cli ping" {
			t.Errorf("healthcheck = %v", got)
		}
	})
}

// TestCacheEngineVersionKeys: REDIS_VERSION holding the redis default is the
// engine's own default for valkey, not a tag for the valkey repository; any other
// value is an explicit tag.
func TestCacheEngineVersionKeys(t *testing.T) {
	for _, tc := range []struct{ version, want string }{
		{"", "valkey/valkey:8.1-alpine"},
		{"7-alpine", "valkey/valkey:8.1-alpine"},
		{"8.1-alpine", "valkey/valkey:8.1-alpine"},
		{"8.0-alpine", "valkey/valkey:8.0-alpine"},
	} {
		setMode(t, PinningLegacy)
		t.Cleanup(SetProjectPins(nil))
		if got := NewGenerator(cacheCfg("valkey", tc.version, "")).buildRedisService().Image; got != tc.want {
			t.Errorf("REDIS_VERSION=%q -> %q, want %q", tc.version, got, tc.want)
		}
	}
}

// TestCacheEngineKeepsRedisURL: the URL apps and plugins read does not depend on
// the engine (service name redis, same port).
func TestCacheEngineKeepsRedisURL(t *testing.T) {
	for _, engine := range []string{"redis", "valkey"} {
		cfg := cacheCfg(engine, "", "pw")
		env := map[string]string{}
		addOptionalStoreEnvVars(env, cfg)
		if env["REDIS_URL"] != "redis://:pw@redis:6379" {
			t.Errorf("%s: REDIS_URL = %q", engine, env["REDIS_URL"])
		}
	}
}

// TestCacheEngineConfigLoad: REDIS_ENGINE is read from the environment (any case),
// and a value that is neither valkey nor redis fails Load rather than falling back.
func TestCacheEngineConfigLoad(t *testing.T) {
	t.Setenv("REDIS_ENGINE", " Valkey ")
	cfg, err := config.Load(t.TempDir())
	if err != nil || cfg.Redis.Engine != "valkey" {
		t.Fatalf("Load = %q, %v; want engine valkey", cfg.Redis.Engine, err)
	}
	t.Setenv("REDIS_ENGINE", "keydb")
	if _, err := config.Load(t.TempDir()); err == nil || !strings.Contains(err.Error(), "REDIS_ENGINE") {
		t.Errorf("Load err = %v, want a REDIS_ENGINE error", err)
	}
}

// respDo sends one RESP command and returns the first reply line.
func respDo(c net.Conn, r *bufio.Reader, args ...string) (string, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte(b.String())); err != nil {
		return "", err
	}
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(line, "$") { // bulk reply: the value is the next line
		val, err := r.ReadString('\n')
		return strings.TrimSpace(val), err
	}
	return strings.TrimSpace(line), nil
}

// TestValkeyIntegration starts a stack with REDIS_ENGINE=valkey, waits for the
// health check and does SET/GET on the port REDIS_URL names.
func TestValkeyIntegration(t *testing.T) {
	// The Windows CI leg runs `go test -short` and has no Docker (ci.yml); the
	// other legs run this test, with no opt-in variable (ticket verify line).
	if testing.Short() {
		t.Skip("short mode: Docker integration test skipped")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker daemon is not reachable")
	}
	setMode(t, PinningLock)
	compattest.Set(t, false) // explicit key, v1.4 mode: the owner-named prod switch
	t.Cleanup(SetProjectPins(nil))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	const password = "integrationPassw0rdNotASecret1234"
	cfg := cacheCfg("valkey", "", password)
	cfg.ProjectName, cfg.DockerNetwork, cfg.Redis.Port, cfg.Env = fmt.Sprintf("l19%d", os.Getpid()), fmt.Sprintf("l19net%d", os.Getpid()), port, "dev"
	cfg, err = config.ApplyDefaults(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Redis.Engine, cfg.Redis.Password = "valkey", password
	raw, err := NewGenerator(cfg).Generate()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	file := dir + "/docker-compose.yml"
	if err := os.WriteFile(file, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	// Compose v2 plugin (`docker compose`) or the standalone docker-compose.
	base := []string{"docker", "compose"}
	if exec.Command("docker", "compose", "version").Run() != nil {
		if _, err := exec.LookPath("docker-compose"); err != nil {
			t.Skip("neither the docker compose plugin nor docker-compose is installed")
		}
		base = []string{"docker-compose"}
	}
	dc := func(args ...string) ([]byte, error) {
		cmd := exec.Command(base[0], append(append(base[1:len(base):len(base)], "-f", file, "-p", cfg.ProjectName), args...)...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "REDIS_PASSWORD="+password)
		return cmd.CombinedOutput()
	}
	t.Cleanup(func() { _, _ = dc("down", "-v", "--remove-orphans") })
	if out, err := dc("up", "-d", "--wait", "--wait-timeout", "120", "redis"); err != nil {
		t.Fatalf("compose up redis: %v\n%s", err, out)
	}
	if out, _ := dc("ps", "--format", "{{.Service}} {{.Image}} {{.Health}}", "redis"); !strings.Contains(string(out), "valkey") || !strings.Contains(string(out), "healthy") {
		t.Fatalf("redis service is not a healthy valkey container: %s", out)
	}

	// The health check command (valkey-cli ping, no -a) must authenticate through
	// REDISCLI_AUTH alone, and the same command without it must be refused: that
	// proves the PONG comes from the env var, not from an open server.
	if out, err := dc("exec", "-T", "redis", "valkey-cli", "ping"); err != nil || strings.TrimSpace(string(out)) != "PONG" {
		t.Fatalf("valkey-cli ping with REDISCLI_AUTH = %q, %v; want PONG", out, err)
	}
	if out, _ := dc("exec", "-T", "redis", "env", "-u", "REDISCLI_AUTH", "valkey-cli", "ping"); !strings.Contains(string(out), "NOAUTH") {
		t.Fatalf("valkey-cli ping without a password = %q; want a NOAUTH refusal", out)
	}
	if out, err := dc("exec", "-T", "redis", "redis-cli", "ping"); err != nil || strings.TrimSpace(string(out)) != "PONG" {
		t.Fatalf("redis-cli symlink ping = %q, %v; want PONG", out, err)
	}

	u, _ := url.Parse(fmt.Sprintf("redis://:%s@127.0.0.1:%d", password, port))
	var c net.Conn
	for i := 0; i < 20; i++ {
		if c, err = net.DialTimeout("tcp", u.Host, 2*time.Second); err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("connect %s: %v", u.Host, err)
	}
	defer c.Close()
	r := bufio.NewReader(c)
	for _, step := range [][]string{{"AUTH", password}, {"SET", "nself:l19", "valkey-ok"}} {
		if rep, err := respDo(c, r, step...); err != nil || rep != "+OK" {
			t.Fatalf("%v -> %q, %v", step[0], rep, err)
		}
	}
	if rep, err := respDo(c, r, "GET", "nself:l19"); err != nil || rep != "valkey-ok" {
		t.Fatalf("GET -> %q, %v", rep, err)
	}
}

// TestExistingCacheEngine: the engine an existing docker-compose.yml shows, in
// any image form, and its place in the precedence (below the explicit key and a
// pin, above the mode default).
func TestExistingCacheEngine(t *testing.T) {
	for _, tc := range []struct{ name, yaml, want string }{
		{"literal redis", "services:\n  redis:\n    image: redis:7-alpine\n", EngineRedis},
		{"variable tag", "services:\n  redis:\n    image: redis:${REDIS_VERSION:-7-alpine}\n", EngineRedis},
		{"variable whole ref", "services:\n  redis:\n    image: ${R:-redis:7-alpine}\n", EngineRedis},
		{"built service", "services:\n  redis:\n    build: .\n", EngineRedis},
		{"valkey image", "services:\n  redis:\n    image: valkey/valkey:8.1-alpine\n", EngineValkey},
		{"redis_data volume", "services:\n  a:\n    image: x\nvolumes:\n  redis_data: {}\n", EngineRedis},
		{"valkey_data volume", "volumes:\n  valkey_data: {}\n", EngineValkey},
		{"no cache", "services:\n  a:\n    image: x\n", ""},
		{"not yaml", "services: [\n", ""},
	} {
		if got := ExistingCacheEngine([]byte(tc.yaml)); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
	compattest.Both(t, func(t *testing.T) {
		t.Cleanup(SetProjectPins(nil))
		t.Cleanup(SetExistingCacheEngine(EngineRedis))
		if got := NewGenerator(cacheCfg("", "", "")).cacheEngine(); got != EngineRedis {
			t.Errorf("existing redis evidence: engine %s", got)
		}
		if got := NewGenerator(cacheCfg("valkey", "", "")).cacheEngine(); got != EngineValkey {
			t.Errorf("explicit key must win over the evidence: engine %s", got)
		}
		SetProjectPins(map[string]string{"redis": "valkey/valkey:8.1-alpine"})
		if got := NewGenerator(cacheCfg("", "", "")).cacheEngine(); got != EngineValkey {
			t.Errorf("a pin must win over the evidence: engine %s", got)
		}
	})
}
