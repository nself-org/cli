package commands

// config_show_get_dotenv_test.go — D-0444/D-0558: a malformed env file must not
// put its text (a secret) into the error of config show/get/list/export/import.
// The fixtures and assertDotenvNoLeak are shared by the other *_dotenv_test.go
// files in this package. Fixtures use obviously fake values only.

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

const dotenvLeakSecret = "fake-planted-s3cret-0444"

// dotenvLeakCases are the two godotenv failure shapes; the bad line is line 2.
var dotenvLeakCases = []struct {
	name    string
	content string
}{
	{"unterminated quote", "GOOD=1\nSECRET_TOKEN=\"" + dotenvLeakSecret + "\n"},
	{"unexpected character", "GOOD=1\nbad key " + dotenvLeakSecret + "=1\n"},
}

// dotenvPlantFile writes content to a new temp file and proves, as a positive
// control, that godotenv itself would put the secret in its error.
func dotenvPlantFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := godotenv.Read(path); err == nil || !strings.Contains(err.Error(), dotenvLeakSecret) {
		t.Fatalf("fixture does not leak through godotenv, so it proves nothing: %v", err)
	}
	return path
}

// assertDotenvNoLeak checks msg names the syntax problem and the line, and
// holds none of the file's text.
func assertDotenvNoLeak(t *testing.T, msg, file string, line int) {
	t.Helper()
	if strings.Contains(msg, dotenvLeakSecret) {
		t.Errorf("message leaks the file text: %q", msg)
	}
	want := "invalid dotenv syntax in " + file + " (line " + strconv.Itoa(line) + ")"
	if !strings.Contains(msg, want) {
		t.Errorf("message %q lacks %q", msg, want)
	}
}

func TestConfigCommandsDotenvSyntaxErrorDoesNotLeak(t *testing.T) {
	for _, tc := range dotenvLeakCases {
		for _, sub := range [][]string{{"show"}, {"get", "GOOD"}, {"list"}, {"export", "out.env"}} {
			t.Run(tc.name+"/"+sub[0], func(t *testing.T) {
				dir := makeNSelfProject(t, tc.content)
				dotenvPlantFile(t, tc.content) // positive control on a twin file
				root := newConfigCmd()
				root.SetErr(new(bytes.Buffer))
				root.SetArgs(append([]string{"config"}, sub...))
				if sub[0] == "export" {
					root.SetArgs([]string{"config", "export", filepath.Join(t.TempDir(), "out.env")})
				}
				out, err := captureStdout(t, func() error { return root.Execute() })
				if err == nil {
					t.Fatalf("want a syntax error, got output %q", out)
				}
				assertDotenvNoLeak(t, err.Error(), filepath.Join(dir, ".env"), 2)
				if strings.Contains(out, dotenvLeakSecret) {
					t.Errorf("stdout leaks the file text: %q", out)
				}
			})
		}
	}
}

func TestConfigImportDotenvSyntaxErrorDoesNotLeak(t *testing.T) {
	for _, tc := range dotenvLeakCases {
		t.Run(tc.name+"/import file", func(t *testing.T) {
			makeNSelfProject(t, "A=1\n")
			src := dotenvPlantFile(t, tc.content)
			root := newConfigCmd()
			root.SetErr(new(bytes.Buffer))
			root.SetArgs([]string{"config", "import", src})
			_, err := captureStdout(t, func() error { return root.Execute() })
			if err == nil {
				t.Fatal("want a syntax error")
			}
			assertDotenvNoLeak(t, err.Error(), src, 2)
		})
		t.Run(tc.name+"/current env file", func(t *testing.T) {
			dir := makeNSelfProject(t, tc.content)
			src := filepath.Join(t.TempDir(), "in.env")
			if err := os.WriteFile(src, []byte("NEW_KEY=1\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			root := newConfigCmd()
			root.SetErr(new(bytes.Buffer))
			root.SetArgs([]string{"config", "import", src})
			_, err := captureStdout(t, func() error { return root.Execute() })
			if err == nil {
				t.Fatal("want a syntax error")
			}
			assertDotenvNoLeak(t, err.Error(), filepath.Join(dir, ".env"), 2)
		})
	}
}

// A missing env file keeps its own message (the not-found branch is unchanged).
func TestConfigShowMissingEnvFileUnchanged(t *testing.T) {
	dir := makeNSelfProject(t, "A=1\n")
	other := filepath.Join(dir, ".env.nope")
	root := newConfigCmd()
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"config", "show", "--env", "nope"})
	_, err := captureStdout(t, func() error { return root.Execute() })
	if err == nil || !strings.Contains(err.Error(), "env file not found: "+other) {
		t.Fatalf("got %v", err)
	}
}
