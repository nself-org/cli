package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/backup"
	"github.com/spf13/pflag"
)

// Purpose: flag and run tests for `nself backup drill --from` (P7-PROD-07).
// docker and age are shell doubles on PATH; the backup and heartbeat live in
// path:// directories; the command runs from a temp directory that is not a
// project, as the owner-machine launchd job does.

const drillFakeDocker = `#!/bin/sh
d="$FAKE_DOCKER_DIR"
printf '%s\n' "$*" >> "$d/calls.log"
case "$1" in
  version) exit 0;;
  run) for a in "$@"; do case "$a" in org.nself.drill=*) printf '%s' "${a#org.nself.drill=}" > "$d/label";; esac; done; exit 0;;
  inspect) cat "$d/label"; exit 0;;
  exec)
    shift 3
    case "$1" in
      pg_restore) [ -n "$FAKE_RESTORE_HANG" ] && exec sleep 30; cat > /dev/null; exit 0;;
      psql)
        stdin=$(cat)
        case "$stdin" in
          *pg_class*) printf 'public\tusers\n';;
          *"count(*)"*) printf '0\t7\n';;
        esac;;
    esac;;
esac
exit 0
`

func TestBackupDrillRemoteFlags(t *testing.T) {
	fs := backupDrillCmd.Flags()
	for _, name := range []string{"from", "identity", "key", "heartbeat-to", "project", "file"} {
		if f := fs.Lookup(name); f == nil || f.Value.Type() != "string" {
			t.Errorf("flag --%s is missing or not a string on `backup drill`", name)
		}
	}
	run := func(flags map[string]string) (string, error) {
		resetBackupFlagSet(t, fs)
		fs.VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
		for k, v := range flags {
			if err := fs.Set(k, v); err != nil {
				t.Fatal(err)
			}
		}
		return captureStdout(t, func() error { return runBackupDrill(backupDrillCmd, nil) })
	}
	// The remote-only flags are refused without --from, and --from refuses the local-only ones.
	if _, err := run(map[string]string{"identity": "k"}); err == nil || !strings.Contains(err.Error(), "--identity needs --from") {
		t.Errorf("--identity without --from: %v", err)
	}
	if _, err := run(map[string]string{"from": "path:///x", "project": "proj", "file": "x.dump"}); err == nil || !strings.Contains(err.Error(), "--file cannot be used with --from") {
		t.Errorf("--file with --from: %v", err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("the docker and age doubles are POSIX shell scripts")
	}

	root := t.TempDir()
	bin, src, hbDir, tmp := filepath.Join(root, "bin"), filepath.Join(root, "src"), filepath.Join(root, "hb"), filepath.Join(root, "tmp")
	for _, d := range []string{bin, src, hbDir, tmp} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{"docker": drillFakeDocker, "age": "#!/bin/sh\ncat\n"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DOCKER_DIR", root)
	t.Setenv("TMPDIR", tmp)
	t.Setenv("NSELF_BACKUP_HEARTBEAT_REMOTE", "")
	identity := filepath.Join(root, "id.key")
	if err := os.WriteFile(identity, []byte("AGE-SECRET-KEY-FAKE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	key := "nself-web_stream_20261005_023000.sql.age"
	if err := os.WriteFile(filepath.Join(src, key), []byte("PGDMP-fake"), 0o600); err != nil {
		t.Fatal(err)
	}
	// The backup heartbeat carries the source row counts the drill compares with.
	if err := os.MkdirAll(filepath.Join(hbDir, "nself-web"), 0o700); err != nil {
		t.Fatal(err)
	}
	hbSrc := `{"schema_version":"1","kind":"backup","project":"nself-web","at":"` + time.Now().UTC().Format(time.RFC3339) +
		`","result":"ok","backup_key":"` + key + `","bytes":10,"encrypted":true,"cli_version":"1.4.12","approx_rows":{"public.users":7},"restored_rows":null,"mismatches":[]}`
	if err := os.WriteFile(filepath.Join(hbDir, "nself-web", "backup.json"), []byte(hbSrc), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir()) // not a project

	out, err := run(map[string]string{"project": "nself-web", "from": "path://" + src, "identity": identity, "heartbeat-to": "path://" + hbDir, "json": "true"})
	if err != nil {
		t.Fatalf("drill: %v", err)
	}
	data, rerr := os.ReadFile(filepath.Join(hbDir, "nself-web", "drill.json"))
	if rerr != nil {
		t.Fatalf("nself-web/drill.json not written: %v", rerr)
	}
	hb, perr := backup.ParseHeartbeat(data, "nself-web", "drill", time.Now())
	if perr != nil || hb.Result != "ok" || hb.BackupKey != key || hb.RestoredRows["public.users"] != 7 {
		t.Fatalf("%v %+v", perr, hb)
	}
	var printed backup.Heartbeat
	if err := json.Unmarshal([]byte(out), &printed); err != nil || printed.Result != "ok" {
		t.Errorf("--json output: %v\n%s", err, out)
	}
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Errorf("temp files left: %v", left)
	}
	// The heartbeat remote can also come from the environment.
	_ = os.Remove(filepath.Join(hbDir, "nself-web", "drill.json"))
	t.Setenv("NSELF_BACKUP_HEARTBEAT_REMOTE", "path://"+hbDir)
	if _, err := run(map[string]string{"project": "nself-web", "from": "path://" + src, "identity": identity}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(hbDir, "nself-web", "drill.json")); err != nil {
		t.Errorf("drill.json not written from NSELF_BACKUP_HEARTBEAT_REMOTE: %v", err)
	}
}

// SIGINT and SIGTERM during a drill cancel its context (signal.NotifyContext),
// so the defers remove the throwaway container and the decrypted files.
func TestBackupDrillSignalCleanup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the docker and age doubles are POSIX shell scripts")
	}
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			root := t.TempDir()
			bin, src, tmp := filepath.Join(root, "bin"), filepath.Join(root, "src"), filepath.Join(root, "tmp")
			for _, d := range []string{bin, src, tmp} {
				if err := os.MkdirAll(d, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			for name, body := range map[string]string{"docker": drillFakeDocker, "age": "#!/bin/sh\ncat\n"} {
				if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("FAKE_DOCKER_DIR", root)
			t.Setenv("FAKE_RESTORE_HANG", "1")
			t.Setenv("TMPDIR", tmp)
			t.Setenv("NSELF_BACKUP_HEARTBEAT_REMOTE", "")
			identity := filepath.Join(root, "id.key")
			_ = os.WriteFile(identity, []byte("AGE-SECRET-KEY-FAKE\n"), 0o600)
			_ = os.WriteFile(filepath.Join(src, "nself-web_stream_20261005_023000.sql.age"), []byte("PGDMP-fake"), 0o600)
			t.Chdir(t.TempDir())

			fs := backupDrillCmd.Flags()
			resetBackupFlagSet(t, fs)
			fs.VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue); f.Changed = false })
			for k, v := range map[string]string{"project": "nself-web", "from": "path://" + src, "identity": identity} {
				if err := fs.Set(k, v); err != nil {
					t.Fatal(err)
				}
			}
			done := make(chan error, 1)
			go func() {
				_, err := captureStdout(t, func() error { return runBackupDrill(backupDrillCmd, nil) })
				done <- err
			}()
			self, _ := os.FindProcess(os.Getpid())
			for i := 0; ; i++ {
				b, _ := os.ReadFile(filepath.Join(root, "calls.log"))
				if strings.Contains(string(b), "pg_restore") {
					break
				}
				if i > 200 {
					t.Fatal("the restore never started")
				}
				time.Sleep(50 * time.Millisecond)
			}
			if err := self.Signal(sig); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("an interrupted drill reported success")
				}
			case <-time.After(20 * time.Second):
				t.Fatal("the drill did not stop on the signal")
			}
			calls, _ := os.ReadFile(filepath.Join(root, "calls.log"))
			if !strings.Contains(string(calls), "rm -f -v nself-drill-") {
				t.Errorf("the throwaway container was not removed:\n%s", calls)
			}
			if left, _ := os.ReadDir(tmp); len(left) != 0 {
				t.Errorf("temp files left: %v", left)
			}
		})
	}
}
