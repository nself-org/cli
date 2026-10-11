//go:build integration

// Live check of CheckFunctionShape against a real pgvector Postgres (D-0369):
// the generated function passes, a reformatted copy of it passes, and every
// hand edit that changes what the function returns or how Postgres runs it
// (STRICT, an inner LIMIT, a WHERE, SECURITY DEFINER) is E513. The shape query
// is plain SQL text, so only a real server proves it parses and compares.
package vectorsearch

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func dockerExec(ctx context.Context, container string, cmd []string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	c := exec.CommandContext(ctx, "docker", append([]string{"exec", container}, cmd...)...)
	c.Stdout, c.Stderr = &stdout, &stderr
	err := c.Run()
	return stdout.String(), stderr.String(), err
}

func startPgvector(t *testing.T) PG {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	out, err := exec.Command("docker", "run", "-d", "--rm", "-e", "POSTGRES_HOST_AUTH_METHOD=trust",
		"-e", "POSTGRES_DB=nself", "pgvector/pgvector:pg16").CombinedOutput()
	if err != nil {
		t.Fatalf("docker run: %v\n%s", err, out)
	}
	id := strings.TrimSpace(string(out))
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", id).Run() })
	pg := PG{Container: id, User: "postgres", DB: "nself"}
	deadline := time.Now().Add(60 * time.Second)
	for {
		if _, _, err := dockerExec(t.Context(), id, psqlArgv(pg, "SELECT 1")); err == nil {
			return pg
		}
		if time.Now().After(deadline) {
			t.Fatal("postgres did not become ready")
		}
		time.Sleep(time.Second)
	}
}

func TestCheckFunctionShapeLive(t *testing.T) {
	pg := startPgvector(t)
	run := func(sql string) {
		t.Helper()
		if _, stderr, err := dockerExec(t.Context(), pg.Container, psqlArgv(pg, sql)); err != nil {
			t.Fatalf("%v: %s\n%s", err, stderr, sql)
		}
	}
	run("CREATE EXTENSION IF NOT EXISTS vector; CREATE TABLE public.docs (id int PRIMARY KEY, owner text, embedding vector(3))")
	s := docsSpec(Cosine)
	if err := CheckFunctionShape(t.Context(), dockerExec, pg, s); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing function: %v", err)
	}
	generated, err := Render(s)
	if err != nil {
		t.Fatal(err)
	}
	const body = `SELECT * FROM "public"."docs" ORDER BY "embedding" <=> $1`
	for _, tc := range []struct {
		name string
		sql  string
		ok   bool
	}{
		{"generated", generated, true},
		{"reformatted", strings.Replace(generated, "  "+body+"\n", "\n\tSELECT *\n\tFROM \"public\".\"docs\"\n\tORDER BY \"embedding\" <=> $1;\n\n", 1), true},
		{"strict", strings.Replace(generated, "STABLE\n", "STABLE STRICT\n", 1), false},
		{"inner limit", strings.Replace(generated, body, body+" LIMIT 5", 1), false},
		{"where", strings.Replace(generated, "ORDER BY", "WHERE owner = 'a' ORDER BY", 1), false},
		{"security definer", strings.Replace(generated, "STABLE\n", "STABLE SECURITY DEFINER\n", 1), false},
		{"generated again", generated, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.sql == generated && tc.name != "generated" && tc.name != "generated again" {
				t.Fatal("variant did not change the SQL")
			}
			run(tc.sql)
			err := CheckFunctionShape(t.Context(), dockerExec, pg, s)
			if tc.ok && err != nil {
				t.Fatalf("want accepted, got %v", err)
			}
			if !tc.ok && (err == nil || !strings.Contains(err.Error(), "E513") || !strings.Contains(err.Error(), "not the generated inlinable shape")) {
				t.Fatalf("want E513 shape error, got %v", err)
			}
		})
	}
}
