//go:build integration

package build

// Purpose: prove, with real postgres and redis containers, that the encoded
// password twins written by WriteComposeEnv make a plugin compose fragment
// connect when the password holds URL-reserved characters (P7-PROD-31), that
// the raw variable in the same URL does not (so the test can fail), and that
// an older compose.env without the twins keeps working through the nested
// default.
// Inputs:  docker + the compose plugin, local images postgres:16-alpine and
//          redis:7-alpine (pulled by docker when absent).
// Outputs: pass/fail. No password value is ever logged: command output is
//          redacted before it reaches the test log.
// Constraints: the connect checks are `docker compose run` clients inside the
//          compose network (no host ports, no bind mounts), so the test also
//          runs from a docker:28-cli container that only has the daemon socket.
//          Build tag `integration`; skipped when docker is unavailable.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
)

// Fixtures, not secrets: every URL-reserved character plus "+" and a space.
const (
	urlEncReservedPW = "a/b@c:d#e?f%g+h i"
	urlEncLegacyPW   = "plainLegacyPw1"
)

// urlEncStack is the stack under test. {{PG}} and {{REDIS}} are the password
// expressions a plugin fragment writes into its URLs.
const urlEncStack = `services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: app
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres -d app"]
      interval: 2s
      timeout: 3s
      retries: 40
  redis:
    image: redis:7-alpine
    command: ["redis-server", "--requirepass", "${REDIS_PASSWORD}"]
    environment:
      REDIS_PW: ${REDIS_PASSWORD}
    healthcheck:
      test: ["CMD-SHELL", "redis-cli -a \"$$REDIS_PW\" ping | grep -q PONG"]
      interval: 2s
      timeout: 3s
      retries: 40
  pgclient:
    image: postgres:16-alpine
    profiles: [client]
    environment:
      - DATABASE_URL=postgresql://postgres:{{PG}}@postgres:5432/app?sslmode=disable
    entrypoint: ["sh", "-c", "psql \"$$DATABASE_URL\" -tAc 'select 1'"]
  redisclient:
    image: redis:7-alpine
    profiles: [client]
    environment:
      - REDIS_URL=redis://:{{REDIS}}@redis:6379
    # redis-cli does not decode percent-escapes in -u, so split and decode the
    # URL the way a client library does: userinfo up to the last "@", then
    # percent-decode the password and authenticate with it.
    entrypoint:
      - sh
      - -c
      - |
        u="$${REDIS_URL#redis://}"; ui="$${u%@*}"; hp="$${u##*@}"
        pw="$$(printf '%s' "$${ui#:}" | awk 'BEGIN{for(i=0;i<256;i++)h[sprintf("%02X",i)]=i}{s=$$0;o="";while((i=index(s,"%"))>0){o=o substr(s,1,i-1) sprintf("%c",h[toupper(substr(s,i+1,2))]);s=substr(s,i+3)}printf "%s",o s}')"
        out="$$(redis-cli -h "$${hp%%:*}" -p "$${hp##*:}" -a "$$pw" --no-auth-warning ping 2>&1)"
        test "$$out" = PONG
`

// urlEncCompose runs docker compose in dir, redacting every secret from the
// returned output.
type urlEncCompose struct {
	t       *testing.T
	dir     string
	project string
	secrets []string
}

func (c *urlEncCompose) run(args ...string) (string, error) {
	c.t.Helper()
	return c.exec(true, args...)
}

// stdout is run with only standard output returned, NOT redacted: warnings go
// to stderr and would corrupt `config --format json`, and the caller parses the
// document. The caller must never log what it returns.
func (c *urlEncCompose) stdout(args ...string) (string, error) {
	c.t.Helper()
	return c.exec(false, args...)
}

func (c *urlEncCompose) exec(combined bool, args ...string) (string, error) {
	full := append([]string{"compose", "-p", c.project, "--env-file", filepath.Join(c.dir, ".nself", "compose.env"), "-f", filepath.Join(c.dir, "stack.yml")}, args...)
	cmd := exec.Command("docker", full...)
	cmd.Dir = c.dir
	var out []byte
	var err error
	if combined {
		out, err = cmd.CombinedOutput()
	} else {
		out, err = cmd.Output()
	}
	s := string(out)
	if !combined {
		return s, err
	}
	for _, sec := range c.secrets {
		if sec != "" {
			s = strings.ReplaceAll(s, sec, "***")
			s = strings.ReplaceAll(s, config.URLPassword(sec), "***")
		}
	}
	return s, err
}

// startStack writes compose.env through the real WriteComposeEnv, writes the
// stack with the given URL expressions and starts postgres and redis.
func startStack(t *testing.T, pw string, withTwins bool, pgExpr, redisExpr string) *urlEncCompose {
	t.Helper()
	dir := t.TempDir()
	cfg := secretTestConfig()
	cfg.Postgres.Password = pw
	cfg.Redis.Enabled = true
	cfg.Redis.Password = pw
	if err := WriteComposeEnv(dir, cfg, SecretEnvMap(cfg), nil); err != nil {
		t.Fatal(err)
	}
	if !withTwins {
		stripTwins(t, filepath.Join(dir, ".nself", "compose.env"))
	}
	stack := strings.NewReplacer("{{PG}}", pgExpr, "{{REDIS}}", redisExpr).Replace(urlEncStack)
	if err := os.WriteFile(filepath.Join(dir, "stack.yml"), []byte(stack), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &urlEncCompose{t: t, dir: dir, project: fmt.Sprintf("urlenc%d", time.Now().UnixNano()%1e9), secrets: []string{pw}}
	t.Cleanup(func() { _, _ = c.run("down", "-v", "--remove-orphans") })
	if out, err := c.run("up", "-d", "--wait", "postgres", "redis"); err != nil {
		t.Fatalf("stack did not start: %v\n%s", err, out)
	}
	return c
}

// stripTwins removes the *_URLENC lines, modelling a compose.env written by an
// older CLI.
func stripTwins(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var keep []string
	for _, l := range strings.Split(string(data), "\n") {
		if !strings.Contains(l, "_URLENC=") {
			keep = append(keep, l)
		}
	}
	if err := os.WriteFile(path, []byte(strings.Join(keep, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestURLEncConnect(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	const (
		fragPG    = "${POSTGRES_PASSWORD_URLENC:-${POSTGRES_PASSWORD}}"
		fragRedis = "${REDIS_PASSWORD_URLENC:-${REDIS_PASSWORD}}"
	)

	t.Run("encoded fragment connects with reserved characters", func(t *testing.T) {
		c := startStack(t, urlEncReservedPW, true, fragPG, fragRedis)
		for _, svc := range []string{"pgclient", "redisclient"} {
			if out, err := c.run("run", "--rm", "-T", svc); err != nil {
				t.Errorf("%s failed to connect with the URL-encoded variable: %v\n%s", svc, err, out)
			}
		}
		// Rendered monitoring DSN parses and decodes to the original password.
		mon := filepath.Join("..", "compose", "docker-compose.monitoring.yml")
		raw, err := os.ReadFile(mon)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(c.dir, "stack.yml"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := c.stdout("--profile", "*", "config", "--format", "json")
		if err != nil {
			t.Fatalf("docker compose config on the monitoring compose: %v\n%s", err, out)
		}
		var doc struct {
			Services map[string]struct {
				Environment map[string]any `json:"environment"`
			} `json:"services"`
		}
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("config output is not JSON: %v", err)
		}
		dsn, _ := doc.Services["postgres-exporter"].Environment["DATA_SOURCE_NAME"].(string)
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatalf("rendered DSN does not parse: %v", err)
		}
		if got, _ := u.User.Password(); got != urlEncReservedPW || u.Host != "postgres:5432" || u.RawQuery != "sslmode=disable" {
			t.Errorf("rendered DSN has the wrong shape (host %q query %q)", u.Host, u.RawQuery)
		}
	})

	t.Run("raw variable in the same URL is rejected", func(t *testing.T) {
		c := startStack(t, urlEncReservedPW, true, "${POSTGRES_PASSWORD}", "${REDIS_PASSWORD}")
		for _, svc := range []string{"pgclient", "redisclient"} {
			if _, err := c.run("run", "--rm", "-T", svc); err == nil {
				t.Errorf("%s connected with the raw password in the URL: the test cannot detect the bug", svc)
			}
		}
	})

	t.Run("compose.env from an older CLI keeps working", func(t *testing.T) {
		c := startStack(t, urlEncLegacyPW, false, fragPG, fragRedis)
		for _, svc := range []string{"pgclient", "redisclient"} {
			if out, err := c.run("run", "--rm", "-T", svc); err != nil {
				t.Errorf("%s failed through the nested-default fallback: %v\n%s", svc, err, out)
			}
		}
	})
}
