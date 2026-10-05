package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
      pg_restore) cat > /dev/null; exit 0;;
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
