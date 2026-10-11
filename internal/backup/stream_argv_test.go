package backup

// Purpose: prove that no subprocess argv built by this package carries the database password (D-0352).
//
// Inputs: shell shims named pg_dump, psql, rclone, age and docker placed first on PATH; they log every argv and the PGPASSWORD they receive.
// Outputs: test failures naming the argv line that leaked the password.
//
// Constraints: unix only (the shims are /bin/sh scripts); the password is an obviously fake value.

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const argvTestPassword = "fake-pw-not-real-0352"

// writeArgvShims writes the shim binaries into a new dir, puts it first on
// PATH, and returns the argv log path and the env log path. Each shim appends
// "<name> <args>" to the argv log and "<name> PGPASSWORD=<value>" to the env
// log, then does the minimum the real tool would do on the pipeline.
func writeArgvShims(t *testing.T) (argvLog, envLog string) {
	t.Helper()
	dir := t.TempDir()
	logDir := t.TempDir()
	argvLog = filepath.Join(logDir, "argv.log")
	envLog = filepath.Join(logDir, "env.log")
	t.Setenv("ARGV_TEST_LOG", argvLog)
	t.Setenv("ENV_TEST_LOG", envLog)

	head := "#!/bin/sh\n" +
		"printf '%s\\n' \"$(basename \"$0\") $*\" >> \"$ARGV_TEST_LOG\"\n" +
		"printf '%s\\n' \"$(basename \"$0\") PGPASSWORD=$PGPASSWORD\" >> \"$ENV_TEST_LOG\"\n"
	shims := map[string]string{
		"pg_dump": "printf 'PGDMP-fake-bytes'\n",
		"psql":    "printf 'public.t\\t1\\n'\n",
		"rclone":  "if [ \"$1\" = rcat ]; then /bin/cat > /dev/null; fi\n",
		"age":     "/bin/cat\n",
		"docker":  "printf 'PGDMP-fake-bytes'\n",
	}
	for name, body := range shims {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(head+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	return argvLog, envLog
}

// argvLinesWithSecret returns the lines of log that contain secret. It is the
// scan the test applies to the shim argv log.
func argvLinesWithSecret(log, secret string) []string {
	var hits []string
	for _, line := range strings.Split(log, "\n") {
		if strings.Contains(line, secret) {
			hits = append(hits, line)
		}
	}
	return hits
}

func readLogFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// TestNoArgvCarriesDatabasePassword drives the real stream and local-dump
// paths against shims and checks that the password reaches the child only
// through PGPASSWORD.
func TestNoArgvCarriesDatabasePassword(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell shims need /bin/sh")
	}
	argvLog, envLog := writeArgvShims(t)
	cfg := minimalConfig()
	cfg.Postgres.Password = argvTestPassword

	res, err := Stream(context.Background(), cfg, StreamOptions{
		To:          "fakeremote:bucket",
		Recipients:  []string{"age1fakerecipientnotreal0352"},
		HeartbeatTo: "fakehb:hb",
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if res == nil || res.Bytes == 0 {
		t.Fatalf("Stream result = %+v", res)
	}
	if err := createFullBackup(context.Background(), cfg, t.TempDir(), "20260101_000000", "", CreateOptions{NoEncrypt: true}); err != nil {
		t.Fatalf("createFullBackup: %v", err)
	}

	argv := readLogFile(t, argvLog)
	for _, name := range []string{"pg_dump", "psql", "rclone", "age", "docker"} {
		if !strings.Contains(argv, name+" ") {
			t.Errorf("shim %s never ran; argv log:\n%s", name, argv)
		}
	}
	if hits := argvLinesWithSecret(argv, argvTestPassword); len(hits) != 0 {
		t.Errorf("the password reached an argv: %q", hits)
	}
	// The password must still reach pg_dump and psql, through the environment.
	env := readLogFile(t, envLog)
	for _, name := range []string{"pg_dump", "psql"} {
		if !strings.Contains(env, name+" PGPASSWORD="+argvTestPassword+"\n") {
			t.Errorf("%s did not receive PGPASSWORD; env log:\n%s", name, env)
		}
	}
}

// TestArgvScanFlagsASecret is the positive control for the scan above: a line
// that does carry the password must be flagged, and clean lines must not be.
func TestArgvScanFlagsASecret(t *testing.T) {
	leaky := "pg_dump --format=custom postgresql://u:" + argvTestPassword + "@localhost:5432/db"
	clean := "pg_dump --format=custom postgresql://u@localhost:5432/db"
	hits := argvLinesWithSecret(clean+"\n"+leaky+"\n", argvTestPassword)
	if len(hits) != 1 || hits[0] != leaky {
		t.Fatalf("scan hits = %q, want only the leaky line", hits)
	}
}

// TestPgDumpCmd checks how pgDumpCmd splits the password off the DSN.
func TestPgDumpCmd(t *testing.T) {
	cases := []struct {
		name       string
		url        string
		wantErr    bool
		wantPW     string
		wantEnvVar bool
	}{
		{"url with password", "postgresql://u:" + argvTestPassword + "@localhost:5432/db?sslmode=disable", false, argvTestPassword, true},
		{"url without password", "postgresql://u@localhost:5432/db?sslmode=disable", false, "", false},
		{"password in query param", "postgresql://u@localhost:5432/db?password=" + argvTestPassword, true, "", false},
		{"keyword dsn", "host=localhost user=u password=" + argvTestPassword + " dbname=db", true, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, pw, err := pgDumpCmd(context.Background(), tc.url)
			if tc.wantErr {
				if err == nil || cmd != nil {
					t.Fatalf("want error and nil cmd, got cmd=%v err=%v", cmd, err)
				}
				if strings.Contains(err.Error(), argvTestPassword) {
					t.Errorf("error leaks the password: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if pw != tc.wantPW {
				t.Errorf("password = %q, want %q", pw, tc.wantPW)
			}
			for _, a := range cmd.Args {
				if strings.Contains(a, argvTestPassword) {
					t.Errorf("argv carries the password: %q", cmd.Args)
				}
			}
			hasEnv := false
			for _, e := range cmd.Env {
				if e == "PGPASSWORD="+argvTestPassword {
					hasEnv = true
				}
			}
			if hasEnv != tc.wantEnvVar {
				t.Errorf("PGPASSWORD in Env = %v, want %v", hasEnv, tc.wantEnvVar)
			}
		})
	}
}

// TestPgDumpCmdHostilePasswords builds the DSN the way production does
// (buildPgURL) for passwords full of URL metacharacters. PGPASSWORD must be
// the exact raw password, and no argv element may hold the raw password or any
// escaped form of it.
func TestPgDumpCmdHostilePasswords(t *testing.T) {
	for _, pw := range []string{"p@ss", "a:b", "100%", "a/b", "%41", "p\u00e4\U0001F4A5ss"} {
		t.Run(pw, func(t *testing.T) {
			cfg := minimalConfig()
			cfg.Postgres.User = "pguser"
			cfg.Postgres.Password = pw
			cmd, got, err := pgDumpCmd(context.Background(), buildPgURL(cfg))
			if err != nil {
				t.Fatal(err)
			}
			if got != pw {
				t.Errorf("returned password = %q, want %q", got, pw)
			}
			hasEnv := false
			for _, e := range cmd.Env {
				if e == "PGPASSWORD="+pw {
					hasEnv = true
				}
			}
			if !hasEnv {
				t.Errorf("PGPASSWORD must equal the raw password %q", pw)
			}
			forms := []string{pw, url.QueryEscape(pw), url.PathEscape(pw)}
			for _, a := range cmd.Args {
				for _, f := range forms {
					if strings.Contains(a, f) {
						t.Errorf("argv element %q holds the password form %q", a, f)
					}
				}
			}
		})
	}
}
