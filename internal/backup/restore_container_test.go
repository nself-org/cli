package backup

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Purpose: tests for the throwaway restore container (P7-PROD-07). docker and
// age are shell doubles placed first on PATH that log every call; no real
// container is started here (TestDrillRemotePostgres in drill_remote_test.go
// runs against a real postgres on Linux).

const fakeDockerScript = `#!/bin/sh
d="$FAKE_DOCKER_DIR"
printf '%s\n' "$*" >> "$d/calls.log"
[ -n "$POSTGRES_PASSWORD" ] && printf '%s' "$POSTGRES_PASSWORD" > "$d/password"
case "$1" in
  version) [ -n "$FAKE_NO_DOCKER" ] && exit 1; echo 24; exit 0;;
  run)
    for a in "$@"; do case "$a" in org.nself.drill=*) printf '%s' "${a#org.nself.drill=}" > "$d/label";; esac; done
    echo cid; exit 0;;
  inspect) if [ -n "$FAKE_BAD_LABEL" ]; then echo other; else cat "$d/label"; fi; exit 0;;
  exec)
    shift 3
    case "$1" in
      pg_isready) exit 0;;
      pg_restore)
        cat > "$d/restored.bin"; echo pg_restore > "$d/tool"
        if [ -n "$FAKE_RESTORE_FAIL" ]; then echo "pg_restore: error: connection to server failed: FATAL" >&2; exit 1; fi
        exit 0;;
      psql)
        stdin=$(cat)
        case " $* " in
          *" -f "*)
            case "$stdin" in
              *pg_class*) printf 'public\tusers\npublic\torders\n';;
              *"count(*)"*) printf '0\t%s\n1\t%s\n' "${FAKE_USERS:-1000}" "${FAKE_ORDERS:-25}";;
            esac;;
          *) printf '%s' "$stdin" > "$d/restored.sql"; echo psql > "$d/tool";;
        esac
        exit 0;;
    esac;;
esac
exit 0
`

const fakeAgeScript = `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_DOCKER_DIR/age.log"
if [ -n "$FAKE_AGE_FAIL" ]; then echo "age: error: no identity matched any of the recipients" >&2; exit 1; fi
cat
`

// fakeDocker installs the docker and age doubles and returns the log directory.
func fakeDocker(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PATH doubles are POSIX shell scripts")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"docker": fakeDockerScript, "age": fakeAgeScript} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DOCKER_DIR", dir)
	return dir
}

func readFake(t *testing.T, dir, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(b)
}

func TestRestoreFormatFromContentNotExtension(t *testing.T) {
	dir := fakeDocker(t)
	ctx := context.Background()
	c := &Container{Name: "nself-drill-0123456789abcdef", User: "postgres", DB: "drill"}
	custom := filepath.Join(t.TempDir(), "stream.sql") // custom-format archive named .sql
	if err := os.WriteFile(custom, []byte("PGDMP\x01\x0e-archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RestoreIntoContainer(ctx, c, custom); err != nil {
		t.Fatal(err)
	}
	if got := readFake(t, dir, "tool"); !strings.HasPrefix(got, "pg_restore") {
		t.Fatalf("custom archive named .sql restored with %q", got)
	}
	if !strings.Contains(readFake(t, dir, "calls.log"), "pg_restore --no-owner --no-acl -U postgres -d drill") {
		t.Errorf("pg_restore must run with --no-owner --no-acl:\n%s", readFake(t, dir, "calls.log"))
	}
	plain := filepath.Join(t.TempDir(), "plain.dump") // plain SQL named .dump
	if err := os.WriteFile(plain, []byte("CREATE TABLE t(a int);\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RestoreIntoContainer(ctx, c, plain); err != nil {
		t.Fatal(err)
	}
	if got := readFake(t, dir, "tool"); !strings.HasPrefix(got, "psql") || !strings.Contains(readFake(t, dir, "restored.sql"), "CREATE TABLE") {
		t.Fatalf("plain SQL named .dump restored with %q", got)
	}
}

func TestRestoreFatalErrorFails(t *testing.T) {
	fakeDocker(t)
	t.Setenv("FAKE_RESTORE_FAIL", "1")
	f := filepath.Join(t.TempDir(), "x")
	_ = os.WriteFile(f, []byte("PGDMPx"), 0o600)
	if err := RestoreIntoContainer(context.Background(), &Container{Name: "nself-drill-0123456789abcdef", User: "u", DB: "d"}, f); err == nil {
		t.Fatal("a failed pg_restore was reported as success")
	}
}

func TestThrowawayContainerIsolation(t *testing.T) {
	dir := fakeDocker(t)
	c, err := StartContainer(context.Background(), ContainerSpec{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Remove()
	if !throwawayNameRe.MatchString(c.Name) {
		t.Fatalf("name %q is not a throwaway name", c.Name)
	}
	calls := readFake(t, dir, "calls.log")
	var run string
	for _, l := range strings.Split(calls, "\n") {
		if strings.HasPrefix(l, "run ") {
			run = l
		}
	}
	for _, want := range []string{"--network none", "--label org.nself.drill=", "-e POSTGRES_PASSWORD postgres:16-alpine", "--name " + c.Name} {
		if !strings.Contains(run, want) {
			t.Errorf("docker run lacks %q: %s", want, run)
		}
	}
	for _, bad := range []string{" -p ", "--publish", "--network host", " -v "} {
		if strings.Contains(run, bad) {
			t.Errorf("docker run must not contain %q: %s", bad, run)
		}
	}
	pw := readFake(t, dir, "password")
	if len(pw) < 32 {
		t.Errorf("password not random and long: %d chars", len(pw))
	}
	if strings.Contains(calls, pw) {
		t.Error("the password reached docker's argv")
	}
}

func TestContainerRemovedAndGuarded(t *testing.T) {
	dir := fakeDocker(t)
	c, err := StartContainer(context.Background(), ContainerSpec{})
	if err != nil {
		t.Fatal(err)
	}
	c.Remove()
	if !strings.Contains(readFake(t, dir, "calls.log"), "rm -f -v "+c.Name) {
		t.Error("throwaway container was not removed")
	}
	for name, want := range map[string]bool{
		"nself-drill-0123456789abcdef": true, "myproj_pg_restore_test": true,
		"myproj_postgres": false, "nself-drill-xyz": false, "_pg_restore_test": false, "nself-drill-0123456789abcdef2": false,
	} {
		if removable(name) != want {
			t.Errorf("removable(%q) = %v", name, !want)
		}
	}
	before := readFake(t, dir, "calls.log")
	(&Container{Name: "myproj_postgres"}).Remove()
	if readFake(t, dir, "calls.log") != before {
		t.Error("Remove touched a live project container")
	}
	if _, err := StartContainer(context.Background(), ContainerSpec{Name: "myproj_postgres"}); err == nil {
		t.Error("StartContainer accepted a live container name")
	}
	if got := readFake(t, dir, "calls.log"); got != before {
		t.Errorf("docker was called for a refused name:\n%s", got)
	}
}

func TestContainerLabelMismatchRefused(t *testing.T) {
	dir := fakeDocker(t)
	t.Setenv("FAKE_BAD_LABEL", "1")
	if _, err := StartContainer(context.Background(), ContainerSpec{}); err == nil {
		t.Fatal("a container without this run's label was used")
	}
	if strings.Contains(readFake(t, dir, "calls.log"), "pg_isready") {
		t.Error("something ran in a container whose label did not match")
	}
}
