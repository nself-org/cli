package config

// envfile_test.go — ReadEnvFile / ParseEnvBytes never put file text in an error.
//
// Every fixture plants a fake secret in a malformed file. A positive control
// proves godotenv itself would leak it, so the "does not contain" check can fail.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/joho/godotenv"
)

const plantedSecret = "fake-planted-s3cret-0444"

// malformedEnvCases are the two godotenv failure shapes plus line-number edges.
var malformedEnvCases = []struct {
	name     string
	content  string
	wantLine int
}{
	{"unterminated quote", "GOOD=1\nSECRET_TOKEN=\"" + plantedSecret + "\n", 2},
	{"unterminated quote at EOF without newline", "T=\"" + plantedSecret, 1},
	{"unterminated single quote", "GOOD=1\nT='" + plantedSecret + "\n", 2},
	{"unexpected character", "GOOD=1\nbad key " + plantedSecret + "=1\n", 2},
	{"unexpected character, fragment holds the marker", "GOOD=1\nbad-key in variable name near " + plantedSecret + "=1\n", 2},
	{"unexpected character on line 1", "bad-key=" + plantedSecret + "\n", 1},
	{"CRLF unterminated", "GOOD=1\r\nOTHER=2\r\nT=\"" + plantedSecret + "\r\n", 3},
	{"CRLF unexpected character", "GOOD=1\r\nbad key " + plantedSecret + "=1\r\n", 2},
	{"multi-line valid value before the error", "A=\"l1\nl2\nl3\"\nB=\"" + plantedSecret + "\n", 4},
	{"escaped quote earlier in the file", "A=\"x\\\"y\"\nB='" + plantedSecret + "\n", 2},
	{"export prefix", "export bad-key=" + plantedSecret + "\n", 1},
}

func TestReadEnvFileSyntaxErrors(t *testing.T) {
	for _, tc := range malformedEnvCases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			// Positive control: the raw parser really does quote the secret.
			if _, perr := godotenv.Read(path); perr == nil || !strings.Contains(perr.Error(), plantedSecret) {
				t.Fatalf("fixture does not leak through godotenv, so it proves nothing: %v", perr)
			}
			_, err := ReadEnvFile(path)
			var se *EnvSyntaxError
			if !errors.As(err, &se) {
				t.Fatalf("want *EnvSyntaxError, got %T %v", err, err)
			}
			if strings.Contains(err.Error(), plantedSecret) {
				t.Fatalf("error leaks the file text: %v", err)
			}
			if se.Line != tc.wantLine || se.File != path {
				t.Errorf("got file=%q line=%d, want %q line %d", se.File, se.Line, path, tc.wantLine)
			}
			want := "invalid dotenv syntax in " + path + " (line " + strconv.Itoa(tc.wantLine) + ")"
			if err.Error() != want {
				t.Errorf("message = %q, want %q", err.Error(), want)
			}
			// Same result from bytes, labelled with the given name.
			_, err = ParseEnvBytes("the pending file", []byte(tc.content))
			if err == nil || strings.Contains(err.Error(), plantedSecret) || !strings.Contains(err.Error(), "the pending file (line "+strconv.Itoa(tc.wantLine)+")") {
				t.Errorf("ParseEnvBytes error = %v", err)
			}
		})
	}
}

func TestReadEnvFileOK(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("A=1\nB=\"two words\"\n# c\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := ReadEnvFile(path)
	if err != nil || m["A"] != "1" || m["B"] != "two words" {
		t.Fatalf("got %v %v", m, err)
	}
}

func TestReadEnvFileMissing(t *testing.T) {
	_, err := ReadEnvFile(filepath.Join(t.TempDir(), "nope.env"))
	if err == nil || !os.IsNotExist(err) || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a missing file must stay an fs not-exist error, got %v", err)
	}
}

func TestEnvSyntaxErrorWithoutLine(t *testing.T) {
	if got := (&EnvSyntaxError{File: "f.env"}).Error(); got != "invalid dotenv syntax in f.env" {
		t.Errorf("got %q", got)
	}
	// A message shape the line finder does not know degrades to line 0.
	if n := envSyntaxLine([]byte("A=1\n"), errors.New("zero length string")); n != 0 {
		t.Errorf("unknown shape: line = %d, want 0", n)
	}
	if n := envSyntaxLine([]byte("A=1\n"), errors.New(`unexpected character 'x' in variable name near "not in the data"`)); n != 0 {
		t.Errorf("fragment not a suffix of the data: line = %d, want 0", n)
	}
}

func TestEnvReadError(t *testing.T) {
	se := &EnvSyntaxError{File: "/p/.env", Line: 3}
	if got := EnvReadError("/p/.env", se); got != error(se) {
		t.Errorf("a syntax error must pass through unchanged, got %v", got)
	}
	fsErr := &fs.PathError{Op: "open", Path: "/p/.env", Err: fs.ErrPermission}
	got := EnvReadError("/p/.env", fsErr)
	if !strings.HasPrefix(got.Error(), "reading /p/.env: ") || !errors.Is(got, fs.ErrPermission) {
		t.Errorf("fs error wrapper = %v", got)
	}
}

func TestExplainSyntaxErrorDoesNotLeak(t *testing.T) {
	t.Setenv(LegacyEnvOrderVar, "")
	t.Setenv("ENV", "")
	dir := t.TempDir()
	writeEnvFiles(t, dir, map[string]string{".env": "GOOD=1\nSECRET_TOKEN=\"" + plantedSecret + "\n"})
	if _, perr := godotenv.Read(filepath.Join(dir, ".env")); perr == nil || !strings.Contains(perr.Error(), plantedSecret) {
		t.Fatalf("fixture does not leak through godotenv: %v", perr)
	}
	for name, run := range map[string]func() error{
		"ExplainKey":    func() error { _, err := ExplainKey(dir, "dev", "GOOD"); return err },
		"ExplainActive": func() error { _, err := ExplainActive(dir, "GOOD"); return err },
	} {
		err := run()
		if err == nil {
			t.Fatalf("%s: want an error", name)
		}
		if strings.Contains(err.Error(), plantedSecret) {
			t.Errorf("%s leaks: %v", name, err)
		}
		want := "invalid dotenv syntax in " + filepath.Join(dir, ".env") + " (line 2)"
		if err.Error() != want {
			t.Errorf("%s: %q, want %q", name, err.Error(), want)
		}
	}
	// ResolveEnv (readEnvKeyFromFile) falls back to dev on a malformed .env.
	if env, src := ResolveEnv(dir); env != "dev" || src != "default" {
		t.Errorf("ResolveEnv = %q %q", env, src)
	}
}
