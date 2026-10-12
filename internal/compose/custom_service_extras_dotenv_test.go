package compose

// custom_service_extras_dotenv_test.go — D-0444/D-0558: a malformed
// CS_N_ENV_FILE (on disk or in a pending overlay) is reported by name and line,
// never by its text.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

func TestLoadCustomServiceEnvFileSyntaxErrorDoesNotLeak(t *testing.T) {
	const secret = "fake-planted-s3cret-0444"
	cases := map[string]string{
		"unterminated quote":   "GOOD=1\nSECRET_TOKEN=\"" + secret + "\n",
		"unexpected character": "GOOD=1\nbad key " + secret + "=1\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := godotenv.UnmarshalBytes([]byte(content)); err == nil || !strings.Contains(err.Error(), secret) {
				t.Fatalf("fixture does not leak through godotenv: %v", err)
			}
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "svc.env"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, onDisk := loadCustomServiceEnvFile(dir, "svc.env", nil)
			_, overlay := loadCustomServiceEnvFile(dir, "svc.env", map[string][]byte{"svc.env": []byte(content)})
			for label, err := range map[string]error{"disk": onDisk, "overlay": overlay} {
				if err == nil {
					t.Fatalf("%s: want a syntax error", label)
				}
				want := "invalid dotenv syntax in " + filepath.Join(dir, "svc.env") + " (line 2)"
				if err.Error() != want {
					t.Errorf("%s: %q, want %q", label, err.Error(), want)
				}
			}
		})
	}
}

func TestLoadCustomServiceEnvFileMissingKeepsWrapper(t *testing.T) {
	dir := t.TempDir()
	_, err := loadCustomServiceEnvFile(dir, "absent.env", nil)
	if err == nil || !os.IsNotExist(unwrapPath(err)) || !strings.HasPrefix(err.Error(), "reading "+filepath.Join(dir, "absent.env")+": ") {
		t.Fatalf("got %v", err)
	}
}

func unwrapPath(err error) error {
	for {
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return err
		}
		err = u.Unwrap()
	}
}
