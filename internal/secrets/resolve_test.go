package secrets

// Purpose: verify D5 resolution, auditing and decryption refusal contracts.
// Inputs: fake values and ephemeral age identities.
// Outputs: assertions on mapping, values and fixed errors.
// Constraints: no real secrets or committed identities.
// SPORT: P7-TRUST-03.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
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

// TestParseRefGrammar proves valid D5 references pass and malformed segments fail.
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

// TestFileForMapping proves all five paths and sources and rejects reserved environments.
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

// TestResolveAuditNoValue proves exactly one value-free audit and fail-closed sink errors.
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
			if event != "secret-resolve" || fields["outcome"] != tc.outcome || fields["attempt_id"] != "a1" || fields["project"] != root || fields["user"] == "" {
				t.Errorf("bad audit: %s %+v", event, fields)
			}
			valid := []string{}
			for _, ref := range tc.refs {
				if _, err := ParseRef(ref); err == nil {
					valid = append(valid, ref)
				}
			}
			b, _ := json.Marshal(valid)
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

// TestResolveAuditInvalidInputs proves pasted values are hashed before reaching the audit log.
func TestResolveAuditInvalidInputs(t *testing.T) {
	invalid := []string{"project/fake-adv-value-xyz", "fake-raw-value-trust03"}
	for _, attempt := range []string{"a1", "fake-attempt-value\ntrust03", strings.Repeat("x", 129)} {
		var fields map[string]string
		err := AuditResolve(func(_ string, f map[string]string) error { fields = f; return nil }, ResolveRequest{
			ProjectRoot: t.TempDir(), Refs: append([]string{"project/FOO"}, invalid...), AttemptID: attempt,
		}, "E540")
		if err != nil || fields["refs"] != `["project/FOO"]` || fields["invalid_refs"] != "2" {
			t.Fatalf("bad sanitized audit: %+v %v", fields, err)
		}
		wantHashes := []string{}
		for _, ref := range invalid {
			wantHashes = append(wantHashes, fmt.Sprintf("%x", sha256.Sum256([]byte(ref)))[:12])
		}
		b, _ := json.Marshal(wantHashes)
		if fields["invalid_ref_hashes"] != string(b) {
			t.Fatalf("incorrect invalid ref digests: %+v", fields)
		}
		log, _ := json.Marshal(fields)
		for _, ref := range invalid {
			if strings.Contains(string(log), ref) {
				t.Fatal("audit log contains pasted secret")
			}
		}
		if attempt == "a1" {
			if fields["attempt_id"] != attempt || fields["attempt_id_sha256"] != "" {
				t.Fatal("valid attempt was changed")
			}
		} else if _, ok := fields["attempt_id"]; ok || fields["attempt_id_sha256"] != fmt.Sprintf("%x", sha256.Sum256([]byte(attempt)))[:12] {
			t.Fatal("invalid attempt was not hashed")
		}
	}
}

// TestResolveUnreadableIdentity proves a mode-000 identity returns E543 without values.
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

// TestResolveMalformedPlaintext proves plaintext-derived parse errors cannot leak values.
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

// TestResolveAllClassesInOrder proves real age resolution preserves all five classes in order.
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
	names := []string{"T", "D", "P", "R", "E"}
	sources := []string{"age:team.age", "age:prod.age", "age:ci.age", "age:release.age", "age:ci-staging.age"}
	v, err := Resolve(context.Background(), ResolveRequest{ProjectRoot: root, Refs: refs}, func(string, map[string]string) error { return nil })
	if err != nil || len(v) != len(refs) {
		t.Fatalf("resolve: %v", err)
	}
	for i, r := range refs {
		if v[i].Ref != r || v[i].Value != trustFakeValue || v[i].EnvName != names[i] || v[i].Source != sources[i] || v[i].ExpiresAt != nil {
			t.Fatal("order, value, environment name or source mismatch")
		}
	}
}
