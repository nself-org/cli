package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeEnvFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExplainKey(t *testing.T) {
	t.Setenv(LegacyEnvOrderVar, "")
	tests := []struct {
		name       string
		files      map[string]string
		process    string
		wantSource string
		wantValue  string
		wantFiles  []string
	}{
		{"base only", map[string]string{".env": "BASE_DOMAIN=a.test\n"}, "", ".env", "a.test", []string{".env"}},
		{"env file beats base", map[string]string{".env": "BASE_DOMAIN=a.test\n", ".env.dev": "BASE_DOMAIN=b.test\n"}, "", ".env.dev", "b.test", []string{".env", ".env.dev"}},
		{"local beats all", map[string]string{".env": "BASE_DOMAIN=a.test\n", ".env.dev": "BASE_DOMAIN=b.test\n", ".env.local": "BASE_DOMAIN=c.test\n"}, "", ".env.local", "c.test", []string{".env", ".env.dev", ".env.local"}},
		{"file replaces process env (Load uses Overload)", map[string]string{".env": "BASE_DOMAIN=a.test\n"}, "p.test", ".env", "a.test", []string{".env"}},
		{"process env when no file sets it", map[string]string{".env": "OTHER=1\n"}, "p.test", "process environment", "p.test", nil},
		{"default when nothing sets it", map[string]string{}, "", "default", "local.nself.org", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeEnvFiles(t, dir, tt.files)
			t.Setenv("BASE_DOMAIN", tt.process) // restores the caller's value on cleanup
			if tt.process == "" {
				os.Unsetenv("BASE_DOMAIN")
			}
			exp, err := ExplainKey(dir, "dev", "BASE_DOMAIN")
			if err != nil {
				t.Fatal(err)
			}
			src, val := exp.Effective()
			if src != tt.wantSource || val != tt.wantValue {
				t.Errorf("Effective() = %q, %q; want %q, %q", src, val, tt.wantSource, tt.wantValue)
			}
			var got []string
			for _, s := range exp.Setters {
				got = append(got, s.File)
			}
			if len(got) != len(tt.wantFiles) {
				t.Fatalf("setters = %v, want %v", got, tt.wantFiles)
			}
			for i := range got {
				if got[i] != tt.wantFiles[i] {
					t.Errorf("setters = %v, want %v", got, tt.wantFiles)
				}
			}
			if !exp.Known || exp.Default != "local.nself.org" {
				t.Errorf("Known=%v Default=%q", exp.Known, exp.Default)
			}
		})
	}
}

func TestExplainKeyUnknownAndUnreadable(t *testing.T) {
	dir := t.TempDir()
	exp, err := ExplainKey(dir, "dev", "NOT_A_REAL_KEY_XYZ")
	if err != nil {
		t.Fatal(err)
	}
	if exp.Known {
		t.Error("unknown key reported as known")
	}
	if src, _ := exp.Effective(); src != "unset" {
		t.Errorf("Effective source = %q, want unset", src)
	}
	if err := os.Mkdir(filepath.Join(dir, ".env"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ExplainKey(dir, "dev", "BASE_DOMAIN"); err == nil {
		t.Error("expected an error for an unreadable cascade file")
	}
}
