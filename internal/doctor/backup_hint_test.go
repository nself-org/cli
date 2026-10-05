package doctor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func hint(t *testing.T, dir, id string) CheckResult {
	t.Helper()
	for _, r := range BackupHintChecks(dir) {
		if r.Name == id {
			return r
		}
	}
	t.Fatalf("no result %s", id)
	return CheckResult{}
}

func TestBackupHintFiresOnNtaskStyleScript(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"scripts/backup.sh": "#!/bin/bash\npg_dump -Fc \"$DB\" > /tmp/x.dump\naws s3 cp /tmp/x.dump s3://bucket/x.dump\n",
		"scripts/rclone.sh": "#!/bin/sh\npg_dump db | gzip | rclone copy - r2:b/x\n",
		"scripts/migrate":   "#!/bin/sh\nfor f in migrations/*.sql; do psql -f \"$f\"; done\n",
	})
	r := hint(t, dir, backupScriptID)
	if r.Status != "pass" || !strings.Contains(r.Message, HintPrefix) {
		t.Fatalf("want a fired advisory, got %+v", r)
	}
	for _, want := range []string{"scripts/backup.sh", "scripts/rclone.sh", "scripts/migrate", "nself backup stream", "nself db migrate"} {
		if !strings.Contains(r.Message+r.FixCmd, want) {
			t.Errorf("hint lacks %q: %+v", want, r)
		}
	}
}

func TestBackupHintQuietOnCleanProject(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"README.md":           "Use pg_dump and aws s3 cp, psql -f migrations/1.sql in prose.\n",
		"scripts/deploy.sh":   "#!/bin/sh\n# pg_dump | aws s3 cp is what we replaced\nnself backup stream --to r2:b\n",
		"scripts/dump.sh":     "#!/bin/sh\npg_dump db > local.dump\n",
		"scripts/up.sh":       "#!/bin/sh\naws s3 cp site s3://b/site\n",
		"node_modules/x/b.sh": "pg_dump a\naws s3 cp a b\n",
	})
	if r := hint(t, dir, backupScriptID); strings.Contains(r.Message, HintPrefix) {
		t.Fatalf("clean project got %+v", r)
	}
}

// The hint is advisory: its status is pass in every case, so doctor's exit code
// (warnings -> 2/12, failures -> 1/10) is the same with and without it.
func TestBackupHintNeverFails(t *testing.T) {
	noisy := t.TempDir()
	writeTree(t, noisy, map[string]string{"b.sh": "pg_dump x | aws s3 cp - s3://b\npsql -f migrations/1.sql\n"})
	clean := t.TempDir()
	for _, dir := range []string{noisy, clean, filepath.Join(clean, "missing")} {
		for _, r := range BackupHintChecks(dir) {
			if r.Status != "pass" {
				t.Fatalf("%s returned %s: %+v", dir, r.Status, r)
			}
		}
	}
}

func TestBackupHintIdentityUntilMarked(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{".env": "PROJECT_NAME=proj\n"})
	if r := hint(t, dir, backupKeyID); strings.Contains(r.Message, HintPrefix) {
		t.Fatalf("no identity yet: %+v", r)
	}
	key := filepath.Join(home, ".config", "nself", "proj-age.key")
	writeTree(t, filepath.Join(home, ".config", "nself"), map[string]string{"proj-age.key": "AGE-SECRET-KEY-1TESTONLYNOTREAL\n"})
	r := hint(t, dir, backupKeyID)
	if !strings.Contains(r.Message, HintPrefix) || !strings.Contains(r.Message, "unrecoverable") || !strings.Contains(r.Message, key) {
		t.Fatalf("want an honest warn, got %+v", r)
	}
	if strings.Contains(r.Message+r.FixCmd, "AGE-SECRET-KEY") {
		t.Fatal("the hint leaked key contents")
	}
	if err := os.WriteFile(key+".backed-up", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if r := hint(t, dir, backupKeyID); strings.Contains(r.Message, HintPrefix) {
		t.Fatalf("marked identity still hints: %+v", r)
	}
}
