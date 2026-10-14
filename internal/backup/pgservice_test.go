package backup

// Purpose: verify URI credentials win over inherited libpq service credentials.
// Inputs: a service file with a stale password and subprocess shims modeling
// libpq's service-password precedence over PGPASSWORD.
// Outputs: failures if either backup subprocess inherits service settings.
// Constraints: shell shims require Unix; no live database or real credentials.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBackupURIOverridesInheritedPGService(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell shims need /bin/sh")
	}
	dir := t.TempDir()
	serviceFile := filepath.Join(dir, "pg_service.conf")
	if err := os.WriteFile(serviceFile, []byte("[stale]\npassword=stale-service-password\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// An explicitly supplied service password takes precedence over the
	// PGPASSWORD default in libpq. Model that resolution in each subprocess.
	shim := `#!/bin/sh
password=$PGPASSWORD
if [ "$PGSERVICE" = stale ]; then
  while IFS= read -r line; do
    case "$line" in password=*) password=${line#password=} ;; esac
  done < "$PGSERVICEFILE"
fi
printf '%s\n' "$password" "service=${PGSERVICE-unset}" "file=${PGSERVICEFILE-unset}"
`
	for _, name := range []string{"pg_dump", "psql"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(shim), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PGSERVICE", "stale")
	t.Setenv("PGSERVICEFILE", serviceFile)
	t.Setenv("PGPASSWORD", "stale-environment-password")
	for _, withPassword := range []bool{true, false} {
		name, pgURL, want := "URI password", "postgresql://user:uri-password@localhost/db", "uri-password\nservice=unset\nfile=unset\n"
		if !withPassword {
			name, pgURL, want = "passwordless URI", "postgresql://user@localhost/db", "stale-service-password\nservice=stale\nfile="+serviceFile+"\n"
		}
		t.Run(name, func(t *testing.T) {
			t.Run("pg_dump", func(t *testing.T) {
				cmd, _, err := pgDumpCmd(context.Background(), pgURL)
				if err != nil {
					t.Fatal(err)
				}
				out, err := cmd.Output()
				if err != nil {
					t.Fatal(err)
				}
				if string(out) != want {
					t.Errorf("resolved credentials and service environment = %q, want %q", out, want)
				}
				if withPassword && strings.Contains(strings.Join(cmd.Args, " "), "uri-password") {
					t.Error("URI password leaked into argv")
				}
			})
			t.Run("psql", func(t *testing.T) {
				out, err := approxQuery(context.Background(), pgURL)
				if err != nil {
					t.Fatal(err)
				}
				if out != want {
					t.Errorf("resolved credentials and service environment = %q, want %q", out, want)
				}
			})
		})
	}
}
