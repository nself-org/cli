package migrate

// env_order_dotenv_test.go — D-0444/D-0558: DetectEnvOrder reports a malformed
// cascade file by name and line, never by its text.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

func TestDetectEnvOrderSyntaxErrorDoesNotLeak(t *testing.T) {
	const secret = "fake-planted-s3cret-0444"
	cases := map[string]string{
		"unterminated quote":   "GOOD=1\nSECRET_TOKEN=\"" + secret + "\n",
		"unexpected character": "GOOD=1\nbad key " + secret + "=1\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".env.secrets")
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := godotenv.Read(path); err == nil || !strings.Contains(err.Error(), secret) {
				t.Fatalf("fixture does not leak through godotenv: %v", err)
			}
			_, err := DetectEnvOrder(dir)
			if err == nil {
				t.Fatal("want a syntax error")
			}
			want := "invalid dotenv syntax in " + path + " (line 2)"
			if err.Error() != want {
				t.Errorf("%q, want %q", err.Error(), want)
			}
		})
	}
}
