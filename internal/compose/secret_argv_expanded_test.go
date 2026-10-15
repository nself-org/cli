package compose_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
)

// TestCacheExpandedArgv exercises the generated shell with a server stand-in.
// It inspects actual exec arguments and the config, without needing Docker.
func TestCacheExpandedArgv(t *testing.T) {
	for _, engine := range []string{"redis", "valkey"} {
		for _, password := range []string{"fakeRedisPw0123", "quotes\" slash\\ space\tline\n\r世界"} {
			t.Run(engine+"/"+fmt.Sprint(len(password)), func(t *testing.T) {
				dir := t.TempDir()
				server := filepath.Join(dir, engine+"-server")
				stub := "#!/bin/sh\nprintf '%s\\n' \"$@\"\nif [ -f \"$1\" ]; then cp \"$1\" \"$CAPTURE\"; fi\n"
				if err := os.WriteFile(server, []byte(stub), 0700); err != nil {
					t.Fatal(err)
				}
				cfg := optionalSecretsConfig("meilisearch", setSecrets)
				cfg.Redis.Engine = engine
				out, err := compose.NewGenerator(cfg).Generate()
				if err != nil {
					t.Fatal(err)
				}
				args := argvStrings(t, out)["redis command"]
				if len(args) != 3 || args[0] != "sh" || args[1] != "-c" {
					t.Fatalf("unexpected shell command: %q", args)
				}
				// Compose unescapes $$; redirect the container's tmpfs into test scratch.
				script := strings.ReplaceAll(strings.ReplaceAll(args[2], "$$", "$"), "/tmp/", dir+"/")
				cmd := exec.Command("sh", "-c", script)
				capture := filepath.Join(dir, "captured.conf")
				cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "REDISCLI_AUTH="+password, "CAPTURE="+capture)
				argv, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("startup failed: %v: %s", err, argv)
				}
				if strings.Contains(string(argv), password) || strings.Contains(string(argv), "--requirepass") {
					t.Fatal("expanded server argv carries the password or --requirepass")
				}
				assertCacheConfig(t, argv, capture, password)
			})
		}
	}
}

func assertCacheConfig(t *testing.T, argv []byte, capture, password string) {
	t.Helper()
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	want := `requirepass "`
	for _, b := range []byte(password) {
		want += fmt.Sprintf(`\x%02x`, b)
	}
	want += "\"\n"
	if string(data) != want {
		t.Fatalf("config = %q, want %q", data, want)
	}
	encoded := strings.TrimSuffix(strings.TrimPrefix(string(data), "requirepass "), "\n")
	if decoded, err := strconv.Unquote(encoded); err != nil || decoded != password {
		t.Fatalf("password round trip failed: %q, %v", decoded, err)
	}
	path := strings.SplitN(string(argv), "\n", 2)[0]
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
	}
}
