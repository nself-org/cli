package secrets

// Purpose: verify D5 resolution, auditing and decryption refusal contracts.
// Inputs: fake values and ephemeral age identities.
// Outputs: assertions on mapping, values and fixed errors.
// Constraints: no real secrets or committed identities.
// SPORT: P7-TRUST-03.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

const trustFakeValue = "s3cr3t-value-0042"

// requireTrustAge supports isolated secret contract checks.
func requireTrustAge(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"age", "age-keygen"} {
		if _, err := exec.LookPath(bin); err != nil {
			if os.Getenv("NSELF_REQUIRE_AGE") == "1" {
				t.Fatal("NSELF_REQUIRE_AGE=1: age and age-keygen required")
			}
			t.Skip("age and age-keygen required for real secret-store test")
		}
	}
}

// trustFixture supports isolated secret contract checks.
func trustFixture(t *testing.T) (root, home, key string) {
	t.Helper()
	requireTrustAge(t)
	root, home = t.TempDir(), t.TempDir()
	key = filepath.Join(home, "operator.key")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SECRETS_AGE_KEY_PATH", key)
	trustKey(t, key)
	return
}

// trustKey supports isolated secret contract checks.
func trustKey(t *testing.T, path string) string {
	t.Helper()
	if err := exec.Command("age-keygen", "-o", path).Run(); err != nil {
		t.Fatal("generate age identity:", err)
	}
	pub, err := GetPublicKey(path)
	if err != nil {
		t.Fatal(err)
	}
	return pub
}

// trustStore supports isolated secret contract checks.
func trustStore(t *testing.T, root, env, name string) {
	t.Helper()
	if err := saveStore(root, env, &SecretStore{Secrets: map[string]SecretEntry{name: {Value: trustFakeValue}}}); err != nil {
		t.Fatal(err)
	}
}

// trustCode supports isolated secret contract checks.
func trustCode(t *testing.T, err error, code string) {
	t.Helper()
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != code {
		t.Fatalf("error=%v, want %s", err, code)
	}
	if strings.Contains(err.Error(), trustFakeValue) {
		t.Fatal("error leaked planted value")
	}
}

// TestParseRefGrammar verifies the secret contract.
func TestParseRefGrammar(t *testing.T) {
	for _, s := range []string{"project/X", "team/FOO_42", "release/Z", "environment/staging/FOO", "deploy/prod/X", "project/" + strings.Repeat("A", 128)} {
		if _, err := ParseRef(s); err != nil {
			t.Errorf("valid %q: %v", s, err)
		}
	}
	for _, s := range []string{"", "project", "project/", "project/x", "project/1X", "project/../X", "project/X/Y", "environment/X", "environment//X", "deploy/Prod/X", "deploy/prod/X/Y", "unknown/X", "team/..", "project/" + strings.Repeat("A", 129), "deploy/" + strings.Repeat("a", 33) + "/X"} {
		_, err := ParseRef(s)
		trustCode(t, err, "E540")
	}
}

// TestFileForMapping verifies the secret contract.
func TestFileForMapping(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	for ref, rel := range map[string]string{"project/FOO": ".secrets/ci.age", "environment/staging/X": ".secrets/ci-staging.age", "deploy/prod/X": ".secrets/prod.age", "release/X": ".secrets/release.age", "team/X": ".config/nself/secrets/team.age"} {
		r, err := ParseRef(ref)
		if err != nil {
			t.Fatal(err)
		}
		p, source, err := FileFor(root, home, r)
		base := root
		if r.Class == "team" {
			base = home
		}
		if err != nil || p != filepath.Join(base, rel) || source != "age:"+filepath.Base(rel) {
			t.Errorf("mapping %s: %s %s %v", ref, p, source, err)
		}
	}
	for _, s := range []string{"deploy/ci/X", "deploy/release/X", "deploy/ci-prod/X", "environment/release/X", "environment/team/X", "deploy/team/X", "environment/ci/X", "environment/ci-prod/X"} {
		_, err := ParseRef(s)
		trustCode(t, err, "E540")
	}
	_, _, err := FileFor(root, home, Ref{Raw: "deploy/prod/X", Class: "deploy", Env: "../../escape", Name: "X"})
	trustCode(t, err, "E540")
}

// TestResolveAuditNoValue verifies the secret contract.
func TestResolveAuditNoValue(t *testing.T) {
	root, _, _ := trustFixture(t)
	trustStore(t, root, "ci", "FOO")
	for _, tc := range []struct {
		refs    []string
		outcome string
	}{
		{[]string{"project/FOO"}, "ok"}, {[]string{"project/MISSING"}, "E541"},
		{[]string{"deploy/prod/X"}, "E541"}, {[]string{"project/x"}, "E540"},
		{[]string{"project/FOO", "project/FOO"}, "E540"}, {[]string{"project/FOO", "team/FOO"}, "E540"},
	} {
		calls := 0
		sink := func(event string, fields map[string]string) error {
			calls++
			if event != "secret-resolve" || fields["outcome"] != tc.outcome || fields["attempt_id"] != "a1" || fields["project"] != root || fields["user"] == "" || len(fields) != 5 {
				t.Errorf("bad audit: %s %+v", event, fields)
			}
			b, _ := json.Marshal(tc.refs)
			if fields["refs"] != string(b) {
				t.Error("wrong refs")
			}
			for _, v := range fields {
				if strings.Contains(v, trustFakeValue) {
					t.Fatal("audit leaked value")
				}
			}
			return nil
		}
		v, err := Resolve(context.Background(), ResolveRequest{ProjectRoot: root, Refs: tc.refs, AttemptID: "a1"}, sink)
		if calls != 1 {
			t.Fatalf("audit calls=%d", calls)
		}
		if tc.outcome == "ok" {
			if err != nil || len(v) != 1 || v[0].Value != trustFakeValue || v[0].ExpiresAt != nil {
				t.Fatalf("bad resolution: %v", err)
			}
		} else {
			trustCode(t, err, tc.outcome)
			if v != nil {
				t.Fatal("values on refusal")
			}
		}
	}
	calls := 0
	v, err := Resolve(context.Background(), ResolveRequest{ProjectRoot: root, Refs: []string{"project/FOO"}}, func(string, map[string]string) error {
		calls++
		return errors.New(trustFakeValue)
	})
	trustCode(t, err, "E401")
	if calls != 1 || v != nil {
		t.Fatal("audit failure must fail closed")
	}
}

// TestResolveUnreadableIdentity verifies the secret contract.
func TestResolveUnreadableIdentity(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Fatal("mode-000 proof must run as non-root")
	}
	root, _, key := trustFixture(t)
	trustStore(t, root, "ci", "FOO")
	if err := os.Chmod(key, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(key, 0600) })
	v, err := Resolve(context.Background(), ResolveRequest{ProjectRoot: root, Refs: []string{"project/FOO"}}, func(string, map[string]string) error { return nil })
	trustCode(t, err, "E543")
	if v != nil {
		t.Fatal("values on unreadable identity")
	}
}

// TestResolveMalformedPlaintext verifies the secret contract.
func TestResolveMalformedPlaintext(t *testing.T) {
	root, _, key := trustFixture(t)
	pub, err := GetPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".secrets"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{trustFakeValue, `{"secrets":{"FOO":{"value":42}},"recipients":["` + trustFakeValue + `"]}`} {
		cmd := exec.Command("age", "-r", pub, "-o", filepath.Join(root, ".secrets", "ci.age"))
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
		v, err := Resolve(context.Background(), ResolveRequest{ProjectRoot: root, Refs: []string{"project/FOO"}}, func(string, map[string]string) error { return nil })
		trustCode(t, err, "E543")
		if v != nil {
			t.Fatal("values from malformed store")
		}
	}
}

// TestResolveAllClassesInOrder verifies the secret contract.
func TestResolveAllClassesInOrder(t *testing.T) {
	root, home, _ := trustFixture(t)
	for env, name := range map[string]string{"ci": "P", "ci-staging": "E", "prod": "D", "release": "R"} {
		trustStore(t, root, env, name)
	}
	teamRoot := t.TempDir()
	trustStore(t, teamRoot, "team", "T")
	dir := filepath.Join(home, ".config", "nself", "secrets")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(teamRoot, ".secrets", "team.age"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "team.age"), b, 0600); err != nil {
		t.Fatal(err)
	}
	refs := []string{"team/T", "deploy/prod/D", "project/P", "release/R", "environment/staging/E"}
	v, err := Resolve(context.Background(), ResolveRequest{ProjectRoot: root, Refs: refs}, func(string, map[string]string) error { return nil })
	if err != nil || len(v) != len(refs) {
		t.Fatalf("resolve: %v", err)
	}
	for i, r := range refs {
		if v[i].Ref != r || v[i].Value != trustFakeValue {
			t.Fatal("order or value mismatch")
		}
	}
}
