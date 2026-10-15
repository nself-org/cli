package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// Real age ciphertext proves restore-test uses the host's rollback identity.
// Docker is a process double; the dump bytes and plaintext cleanup are checked.
func TestVerifyRestoreEncryptedBackup(t *testing.T) {
	for _, outcome := range []string{"restored", "wrong identity", "restore failed"} {
		t.Run(outcome, func(t *testing.T) {
			home, _, _ := autoKeyEnv(t)
			compattest.Set(t, true)
			key, err := EnsureIdentity("proj")
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			const id = "proj_full_20261014_120000.dump.age"
			const dump = "PGDMP-verified-fixture"
			ageEncrypt(t, key.Path, dump, filepath.Join(dir, id))
			if outcome == "wrong identity" {
				other := filepath.Join(home, "other.key")
				newAgeKey(t, other)
				wrong, err := os.ReadFile(other)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(key.Path, wrong, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			// Install only the docker double so age remains the real executable.
			bin := t.TempDir()
			script := strings.Replace(fakeDockerScript, "stdin=$(cat)", "stdin=$(cat)\n        case \" $* \" in *\" -c \"*) printf '1\\ns46-verify\\n'; exit 0;; esac", 1)
			if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("FAKE_DOCKER_DIR", bin)
			if outcome == "restore failed" {
				t.Setenv("FAKE_RESTORE_FAIL", "1")
			}
			result, err := Verify(context.Background(), localBackupCfg(dir), VerifyOptions{BackupID: id, RestoreTest: true, Cleanup: true})
			if outcome == "restored" {
				if err != nil || !result.Verified || result.Method != "restore-test" {
					t.Fatalf("result %+v, err %v", result, err)
				}
				if got := readFake(t, bin, "restored.bin"); got != dump {
					t.Fatalf("restored %q, want decrypted %q", got, dump)
				}
			} else if err == nil || result == nil || result.Verified {
				t.Fatalf("result %+v, err %v: must refuse %s", result, err, outcome)
			}
			calls := readFake(t, bin, "calls.log")
			if outcome == "wrong identity" {
				if !strings.Contains(err.Error(), "no identity matched") || calls != "" {
					t.Fatalf("decrypt must fail before starting docker: %v, calls %s", err, calls)
				}
			} else if !strings.Contains(calls, "rm -f -v proj_pg_restore_test") {
				t.Fatalf("restore container was not cleaned up: %s", calls)
			}
			left, err := filepath.Glob(filepath.Join(dir, ".nself-restore-*.dec"))
			if err != nil || len(left) != 0 {
				t.Fatalf("plaintext leftovers %v, err %v", left, err)
			}
		})
	}
}
