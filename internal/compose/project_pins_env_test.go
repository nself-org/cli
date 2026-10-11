package compose

// project_pins_env_test.go — an explicit *_IMAGE digest against a lock pin
// (P7-LIVE-09, adversarial r2 N2, r3 M1/S1): the digest check covers every
// *_IMAGE variable the generator reads, not only the first variable set.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

const otherDigest = "sha256:7777777777777777777777777777777777777777777777777777777777777777"

// pinNotices captures the drop notices of one test.
func pinNotices(t *testing.T) *bytes.Buffer {
	t.Helper()
	resetPinNotices(t)
	var notice bytes.Buffer
	prev := pinNoticeW
	pinNoticeW = &notice
	t.Cleanup(func() { pinNoticeW = prev })
	return &notice
}

// TestProjectPinEnvImageDigest: POSTGRES_IMAGE is the one *_IMAGE the generator
// reads (ResolvePostgresImage). Its digest beats the pin when it differs (same
// repository and tag), with exactly one notice, also when POSTGRES_VERSION is
// set (nself init writes it into every .env). The pin's own digest and a
// digest-free value keep the pin.
func TestProjectPinEnvImageDigest(t *testing.T) {
	const base = "pgvector/pgvector:pg16"
	pin := base + "@" + pinDigest
	for _, version := range []string{"", "16-alpine"} {
		cases := []struct {
			name, env string
			kept      bool
		}{
			{"different digest", base + "@" + otherDigest, false},
			{"different digest, docker.io spelling", "docker.io/" + base + "@" + otherDigest, false},
			{"same digest", base + "@" + pinDigest, true},
			{"no digest", base, true},
		}
		for _, c := range cases {
			t.Run("version="+version+"/"+c.name, func(t *testing.T) {
				notice := pinNotices(t)
				t.Setenv("POSTGRES_VERSION", version)
				t.Setenv("POSTGRES_IMAGE", c.env)
				defer SetProjectPins(map[string]string{"postgres": pin})()
				for i := 0; i < 3; i++ {
					got := ResolveImage("postgres", ResolvePostgresImage(config.PostgresConfig{Image: c.env, Version: version}))
					if c.kept && got != pin {
						t.Fatalf("pin lost: got %q", got)
					}
					if !c.kept && got == pin {
						t.Fatalf("the user's digest was replaced by the pin: %q", got)
					}
				}
				n := strings.Count(notice.String(), "notice:")
				if c.kept && n != 0 {
					t.Errorf("a kept pin must not print a notice: %q", notice.String())
				}
				if !c.kept && (n != 1 || !strings.Contains(notice.String(), "POSTGRES_IMAGE")) {
					t.Errorf("want one notice naming POSTGRES_IMAGE, got %q", notice.String())
				}
			})
		}
	}
}

// TestProjectPinIgnoredImageVars: an *_IMAGE variable the generator never reads
// must not drop the pin and must not print a notice (adversarial r3 S1).
func TestProjectPinIgnoredImageVars(t *testing.T) {
	const base = "hasura/graphql-engine:v2.44.0"
	pin := base + "@" + pinDigest
	for _, c := range []struct{ service, env, image string }{
		{"hasura", "HASURA_IMAGE", base},
		{"admin", "ADMIN_IMAGE", DefaultImage("admin")},
		{"storage", "STORAGE_IMAGE", DefaultImage("storage")},
	} {
		t.Run(c.env, func(t *testing.T) {
			notice := pinNotices(t)
			p := pin
			if c.service != "hasura" {
				p = CanonicalRef(c.image) + "@" + pinDigest
			}
			t.Setenv(c.env, c.image+"@"+otherDigest)
			defer SetProjectPins(map[string]string{c.service: p})()
			if got, ok := projectPin(c.service, c.image); !ok || got != p {
				t.Errorf("an ignored %s dropped the pin: %q %v", c.env, got, ok)
			}
			if notice.Len() != 0 {
				t.Errorf("an ignored %s printed a notice: %q", c.env, notice.String())
			}
		})
	}
}

// TestEnvDigestDiffersNames: only the generator's *_IMAGE variables are read,
// and the variable that differs is the one returned.
func TestEnvDigestDiffersNames(t *testing.T) {
	pin := "x:1@" + pinDigest
	t.Setenv("POSTGRES_VERSION", "x:1@"+otherDigest) // a version never carries a digest
	t.Setenv("HASURA_IMAGE", "x:1@"+otherDigest)
	if got := envDigestDiffers(inputFor("postgres", "postgres:16-alpine"), pin); got != "" {
		t.Errorf("POSTGRES_VERSION must not count: %q", got)
	}
	if got := envDigestDiffers(inputFor("hasura", DefaultImage("hasura")), pin); got != "" {
		t.Errorf("HASURA_IMAGE has no consumer: %q", got)
	}
	t.Setenv("POSTGRES_IMAGE", "x:1@"+otherDigest)
	if got := envDigestDiffers(inputFor("postgres", "postgres:16-alpine"), pin); got != "POSTGRES_IMAGE" {
		t.Errorf("got %q, want POSTGRES_IMAGE", got)
	}
}
