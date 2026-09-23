package compose

import (
	"net/url"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// A std-base64 REDIS_PASSWORD contains "/" and "="; left raw, the "/" ends
// the URL authority and Node's URL parser throws "Invalid URL" (nself-web
// prod, 2026-09-23). The injected REDIS_URL must parse back to the same
// host and password.
func TestRedisURLEncodesReservedPasswordChars(t *testing.T) {
	cfg := &config.Config{}
	cfg.Redis.Enabled = true
	cfg.Redis.Port = 6379
	cfg.Redis.Password = "Zm9v/YmFy+cXV4@:x?#="

	env := map[string]string{}
	addOptionalStoreEnvVars(env, cfg)

	u, err := url.Parse(env["REDIS_URL"])
	if err != nil {
		t.Fatalf("REDIS_URL does not parse: %v", err)
	}
	if u.Host != "redis:6379" {
		t.Errorf("host = %q, want redis:6379", u.Host)
	}
	if pw, _ := u.User.Password(); pw != cfg.Redis.Password {
		t.Errorf("password round trip = %q, want %q", pw, cfg.Redis.Password)
	}
}
