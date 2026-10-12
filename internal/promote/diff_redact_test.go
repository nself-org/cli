package promote

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRedactIfSecret pins the promote diff to the shared rule (D-0415): every
// key config.IsSecretKey names is masked in full (including AUTH_SMTP_PASS and
// *_WEBHOOK_URL, which the old promote list missed), URL credentials are
// masked in any other value, and no character of a secret survives. All
// values are synthetic.
func TestRedactIfSecret(t *testing.T) {
	cases := []struct{ key, value, want string }{
		{"POSTGRES_PASSWORD", "FAKEPGPASS01", "****"},
		{"HASURA_GRAPHQL_ADMIN_SECRET", "FAKEADMIN02", "****"},
		{"AUTH_SMTP_PASS", "FAKESMTP03", "****"},
		{"SLACK_WEBHOOK_URL", "https://hooks.example.test/FAKEHOOK04", "****"},
		{"SENTRY_DSN", "https://FAKEDSN05@sentry.example.test/1", "****"},
		{"PASSWORD_SALT", "FAKESALT06", "****"},
		{"S3_ACCESS_KEY", "abc", "****"},
		{"DATABASE_URL", "postgres://app:FAKEDB07@db:5432/app", "postgres://****@db:5432/app"},
		{"CALLBACK_URL", "https://example.test/cb#access_token=FAKETOK08", "https://example.test/cb#access_token=****"},
		{"PROJECT_NAME", "demo", "demo"},
		{"BASE_DOMAIN", "example.test", "example.test"},
		{"POSTGRES_PASSWORD", "", ""},
	}
	for _, c := range cases {
		if got := redactIfSecret(c.key, c.value); got != c.want {
			t.Errorf("redactIfSecret(%q, %q) = %q, want %q", c.key, c.value, got, c.want)
		}
	}
}

// TestDryRunDiffHidesSecrets runs DryRun over two synthetic env files and
// checks the marshalled diff: no fake secret (or a 2-character edge of one)
// appears, and a changed secret still reports status "changed".
func TestDryRunDiffHidesSecrets(t *testing.T) {
	dir := t.TempDir()
	staging := "POSTGRES_PASSWORD=FAKESTAGEPW11\nAUTH_SMTP_PASS=FAKESMTPST12\nDATABASE_URL=postgres://app:FAKEDBST13@db/app\nSAME_TOKEN=FAKESAME14\nPROJECT_NAME=demo\n"
	prod := "POSTGRES_PASSWORD=FAKEPRODPW15\nAUTH_SMTP_PASS=FAKESMTPPR16\nSAME_TOKEN=FAKESAME14\nOLD_WEBHOOK_URL=https://hooks.example.test/FAKEHOOK17\nPROJECT_NAME=demo\n"
	for name, body := range map[string]string{".env.staging": staging, ".env.prod": prod} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	res, err := DryRun(context.Background(), dir, "staging", "prod")
	if err != nil {
		t.Fatalf("DryRun: %v", err)
	}
	out, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"FAKESTAGEPW11", "FAKESMTPST12", "FAKEDBST13", "FAKESAME14", "FAKEPRODPW15", "FAKESMTPPR16", "FAKEHOOK17", `"FA***`, `***11"`, `***16"`} {
		if strings.Contains(string(out), secret) {
			t.Errorf("promote diff leaks %q:\n%s", secret, out)
		}
	}
	want := map[string]string{"POSTGRES_PASSWORD": "changed", "AUTH_SMTP_PASS": "changed", "SAME_TOKEN": "same", "DATABASE_URL": "added", "OLD_WEBHOOK_URL": "removed", "PROJECT_NAME": "same"}
	for _, e := range res.EnvVars {
		if w, ok := want[e.Key]; ok && e.Status != w {
			t.Errorf("%s status = %q, want %q", e.Key, e.Status, w)
		}
	}
	if len(res.EnvVars) != len(want) {
		t.Errorf("diff has %d entries, want %d", len(res.EnvVars), len(want))
	}
}
