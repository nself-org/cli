//go:build unix

package compose

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCachePasswordConfig(t *testing.T) {
	for _, tc := range []struct{ name, password, encoded string }{
		{"simple", "fake-password", "fake-password"},
		{"escapes", "fake\"\\\n\r password", "fake\\\"\\\\\\n\\r password"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "redis.conf")
			script := strings.ReplaceAll(cachePasswordConfig, "/run/nself-cache/redis.conf", path) + "true"
			cmd := exec.Command("sh", "-c", script)
			cmd.Env = append(os.Environ(), "REDISCLI_AUTH="+tc.password)
			if err := cmd.Run(); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != "requirepass \""+tc.encoded+"\"\n" {
				t.Fatal("password config escaping failed")
			}
			fi, err := os.Stat(path)
			if err != nil || fi.Mode().Perm() != 0600 {
				t.Fatal("config permissions", err)
			}
		})
	}
}

// TestCacheServerArgv executes the generated startup through a server stub,
// checking the argv after shell expansion rather than only compose text.
func TestCacheServerArgv(t *testing.T) {
	for _, engine := range []string{"redis", "valkey"} {
		t.Run(engine, func(t *testing.T) {
			dir := t.TempDir()
			const password = "fake-password-argv-check"
			svc := NewGenerator(cacheCfg(engine, "", password)).buildRedisService()
			argvPath := filepath.Join(dir, "argv")
			stub := filepath.Join(dir, engine+"-server")
			if err := os.WriteFile(stub, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$ARGV_OUT\"\n"), 0700); err != nil {
				t.Fatal(err)
			}
			command, ok := svc.Command.([]string)
			if !ok || len(command) != 3 {
				t.Fatal("missing startup shell")
			}
			script := strings.ReplaceAll(command[2], "/run/nself-cache/redis.conf", filepath.Join(dir, "redis.conf"))
			script = strings.ReplaceAll(script, "$$", "$")
			cmd := exec.Command(command[0], command[1], script)
			cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "REDISCLI_AUTH="+password, "ARGV_OUT="+argvPath)
			if err := cmd.Run(); err != nil {
				t.Fatal(err)
			}
			argv, err := os.ReadFile(argvPath)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(argv), password) || strings.Contains(string(argv), "--requirepass") {
				t.Fatal("server argv contains password option")
			}
			if !strings.Contains(string(argv), filepath.Join(dir, "redis.conf")) {
				t.Fatal("server did not receive private config")
			}
		})
	}
}
